import os
import requests
import pika
import json

from .logging_config import celery_logger

# The template service listens on 3003 and serves /template/{id}.
TEMPLATE_SERVICE_URL = os.getenv('TEMPLATE_SERVICE_URL', 'http://template-service:3003')
# The orchestrator owns notification status.
STATUS_CALLBACK_URL = os.getenv('STATUS_CALLBACK_URL', 'http://orchestrator:8080/notifications/status')
INTERNAL_SERVICE_TOKEN = os.getenv('INTERNAL_SERVICE_TOKEN', '')
RABBITMQ_HOST = os.getenv('RABBITMQ_HOST', 'localhost')
RABBITMQ_USER = os.getenv('RABBITMQ_USER', 'guest')
RABBITMQ_PASSWORD = os.getenv('RABBITMQ_PASSWORD', 'guest')


def fetch_email_template(template_code):
    """
    Fallback path only. Notifications from the orchestrator arrive pre-rendered;
    this is used by the direct HTTP API, which has no rendering step of its own.
    """
    try:
        resp = requests.get(
            f'{TEMPLATE_SERVICE_URL}/template/{template_code}',
            headers={'X-Service-Token': INTERNAL_SERVICE_TOKEN},
            timeout=5,
        )
        resp.raise_for_status()
        payload = resp.json()
        data = payload.get('data') or {}
        versions = data.get('versions') or []
        if versions:
            return versions[0].get('body', '')
        return ''
    except Exception:
        return "Hello {name}, \n\n(Template not available) \n\n{link}"
    
def report_status(notification_id, status, error=None):
    if not notification_id:
        # Requests submitted straight to the HTTP API have no orchestrator
        # record to update; there is nothing to report against.
        return

    payload = {
        'notification_id': notification_id,
        'status': status,
    }
    if error:
        payload['error'] = str(error)
    try:
        requests.post(
            STATUS_CALLBACK_URL,
            json=payload,
            headers={'X-Service-Token': INTERNAL_SERVICE_TOKEN},
            timeout=5,
        )
    except Exception as exc:
        # The orchestrator's status endpoint lands in Phase 2. Until then this
        # call is expected to fail; log it rather than swallowing it silently.
        celery_logger.warning(
            f"Status callback failed for {notification_id}: {exc}"
        )


def publish_to_failed_queue(message_body: dict):
    """
    On permanent failure, push message to a failed (dead-letter) queue for manual inspection.
    """
    credentials = pika.PlainCredentials(RABBITMQ_USER, RABBITMQ_PASSWORD)
    params = pika.ConnectionParameters(host=RABBITMQ_HOST, credentials=credentials)
    try:
        conn = pika.BlockingConnection(params)
        ch = conn.channel()
        
        # ensure exchange/queue exist (direct)
        ch.exchange_declare(exchange="notifications.direct", exchange_type="direct", durable=True)
        ch.queue_declare(queue="failed.queue", durable=True)
        ch.queue_bind(queue="failed.queue", exchange="notifications.direct", routing_key="failed")
        ch.basic_publish(
            exchange="notifications.direct",
            routing_key="failed",
            body=json.dumps(message_body),
            properties=pika.BasicProperties(delivery_mode=2),
        )
    except Exception:
        # log in production
        pass
    finally:
        try:
            conn.close()
        except Exception:
            pass