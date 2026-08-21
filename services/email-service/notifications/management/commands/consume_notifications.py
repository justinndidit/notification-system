"""
Bridges the orchestrator's RabbitMQ queue to Celery.

The orchestrator publishes a plain JSON message (see
docs/contracts/enriched-notification.md). Celery expects its own wire protocol,
so a raw message on the queue can never dispatch as a task. This consumer sits
between the two: it reads the contract, translates it into the task's payload
shape, and hands it to Celery.

Run alongside the Celery worker:

    python manage.py consume_notifications
"""

import json
import os
import signal
import time

import pika
from django.core.management.base import BaseCommand

from notifications.logging_config import celery_logger
from notifications.tasks import send_email_task

RABBITMQ_HOST = os.getenv("RABBITMQ_HOST", "rabbitmq")
RABBITMQ_PORT = int(os.getenv("RABBITMQ_PORT", "5672"))
RABBITMQ_USER = os.getenv("RABBITMQ_USER", "guest")
RABBITMQ_PASSWORD = os.getenv("RABBITMQ_PASSWORD", "guest")

EXCHANGE = os.getenv("NOTIFICATIONS_EXCHANGE", "notifications")
QUEUE = os.getenv("EMAIL_QUEUE", "email_queue")
ROUTING_KEY = os.getenv("EMAIL_ROUTING_KEY", "notification.email")

RECONNECT_MAX_ATTEMPTS = 10


def to_task_payload(message: dict) -> dict:
    """
    Translate an enriched notification into the shape send_email_task expects.

    The contract carries a resolved recipient and rendered content, so nothing
    here needs to look anything up. `idempotency_key` becomes `request_id`
    because that is the key the task deduplicates on.
    """
    variables = dict(message.get("variables") or {})

    # The task reads the address and subject out of `variables`; the contract
    # carries them as top-level resolved fields.
    recipient = message.get("recipient")
    if recipient:
        variables["email"] = recipient
    if message.get("subject"):
        variables["subject"] = message["subject"]

    metadata = dict(message.get("metadata") or {})
    metadata["correlation_id"] = message.get("correlation_id")
    metadata["notification_id"] = message.get("notification_id")

    return {
        "notification_type": message.get("channel", "email"),
        "user_id": message.get("user_id"),
        "template_code": message.get("template_code"),
        "variables": variables,
        "request_id": message.get("idempotency_key") or message.get("notification_id"),
        "priority": message.get("priority", "normal"),
        "metadata": metadata,
        # Pre-rendered by the orchestrator. When present the task sends this
        # verbatim instead of fetching and compiling a template.
        "subject": message.get("subject"),
        "body": message.get("body"),
    }


class Command(BaseCommand):
    help = "Consume enriched notifications from RabbitMQ and dispatch them to Celery"

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._should_stop = False

    def handle(self, *args, **options):
        signal.signal(signal.SIGTERM, self._request_stop)
        signal.signal(signal.SIGINT, self._request_stop)

        attempt = 0
        while not self._should_stop:
            try:
                self._consume()
                attempt = 0
            except pika.exceptions.AMQPError as exc:
                if self._should_stop:
                    break
                attempt += 1
                if attempt > RECONNECT_MAX_ATTEMPTS:
                    celery_logger.error(
                        f"Giving up after {RECONNECT_MAX_ATTEMPTS} reconnection attempts: {exc}"
                    )
                    raise
                delay = min(2 ** attempt, 30)
                celery_logger.warning(
                    f"RabbitMQ connection lost ({exc}); reconnecting in {delay}s "
                    f"[attempt {attempt}/{RECONNECT_MAX_ATTEMPTS}]"
                )
                time.sleep(delay)

        self.stdout.write(self.style.SUCCESS("Consumer stopped"))

    def _request_stop(self, _signum, _frame):
        self._should_stop = True

    def _consume(self):
        credentials = pika.PlainCredentials(RABBITMQ_USER, RABBITMQ_PASSWORD)
        params = pika.ConnectionParameters(
            host=RABBITMQ_HOST,
            port=RABBITMQ_PORT,
            credentials=credentials,
            heartbeat=60,
        )

        connection = pika.BlockingConnection(params)
        try:
            channel = connection.channel()

            # Declared idempotently and identically to the orchestrator, so
            # whichever service starts first establishes the topology.
            channel.exchange_declare(
                exchange=EXCHANGE, exchange_type="topic", durable=True
            )
            channel.queue_declare(queue=QUEUE, durable=True)
            channel.queue_bind(
                queue=QUEUE, exchange=EXCHANGE, routing_key=ROUTING_KEY
            )
            channel.basic_qos(prefetch_count=10)

            channel.basic_consume(
                queue=QUEUE, on_message_callback=self._on_message, auto_ack=False
            )

            self.stdout.write(
                self.style.SUCCESS(
                    f"Listening on {QUEUE} (bound to {EXCHANGE}/{ROUTING_KEY})"
                )
            )

            while not self._should_stop:
                connection.process_data_events(time_limit=1)

            channel.stop_consuming()
        finally:
            if connection.is_open:
                connection.close()

    def _on_message(self, channel, method, properties, body):
        correlation_id = getattr(properties, "correlation_id", None)

        try:
            message = json.loads(body)
        except json.JSONDecodeError as exc:
            celery_logger.error(
                f"Discarding malformed message: {exc}",
                extra={"correlation_id": correlation_id},
            )
            # Malformed messages will never parse, so requeueing would loop.
            channel.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
            return

        try:
            payload = to_task_payload(message)
            send_email_task.delay(payload)
        except Exception as exc:
            celery_logger.error(
                f"Failed to enqueue email task: {exc}",
                extra={"correlation_id": correlation_id},
            )
            # Enqueueing failed for an environmental reason (broker or backend);
            # requeue so another attempt can pick it up.
            channel.basic_nack(delivery_tag=method.delivery_tag, requeue=True)
            return

        celery_logger.info(
            "Dispatched email task",
            extra={
                "correlation_id": message.get("correlation_id"),
                "request_id": payload["request_id"],
            },
        )
        channel.basic_ack(delivery_tag=method.delivery_tag)
