"""
Celery tasks for email processing with circuit breaker and comprehensive logging.
"""

import os
import time
import json
from celery import shared_task, Task
from django.core.mail import EmailMessage
from django.db import transaction
from django.conf import settings
from pybreaker import CircuitBreaker

from .models import EmailLog
from .utils import fetch_email_template, report_status, publish_to_failed_queue
from .logging_config import celery_logger

MAX_RETRIES = 5

# Circuit breaker for SMTP
smtp_circuit_breaker = CircuitBreaker(
    fail_max=5,
    reset_timeout=60,
    exclude=[ValueError],
    listeners=[],
)

# Circuit breaker for external template service
template_circuit_breaker = CircuitBreaker(
    fail_max=3,
    reset_timeout=30,
    exclude=[ValueError],
    listeners=[],
)


class BaseEmailTask(Task):
    """
    Base task with retry configuration and error handling.
    """

    autoretry_for = (Exception,)
    retry_backoff = True  # built-in exponential backoff
    retry_backoff_max = 600  # max backoff in seconds
    retry_jitter = True
    default_retry_delay = 5


@shared_task(
    bind=True,
    base=BaseEmailTask,
    acks_late=True,
    max_retries=MAX_RETRIES,
    name="send_email_task",
)
def send_email_task(self, payload: dict):
    """
    Send email via SMTP with idempotency and circuit breaker protection.

    Args:
        payload: Notification payload containing:
            - notification_type: "email"
            - user_id: User UUID
            - template_code: Template identifier
            - variables: Template variables
            - request_id: Unique request ID
            - priority: Priority level (1-100)
            - metadata: Additional metadata

    Returns:
        dict: Status response

    Raises:
        Task.retry: On transient errors
    """
    request_id = payload.get("request_id")
    if not request_id:
        request_id = f"generated-{int(time.time() * 1000)}"

    correlation_id = payload.get("metadata", {}).get("correlation_id", request_id)

    celery_logger.info(
        f"Processing email task",
        extra={
            "request_id": request_id,
            "user_id": payload.get("user_id"),
            "template_code": payload.get("template_code"),
        },
    )

    # Idempotency: Check if already delivered
    try:
        existing = EmailLog.objects.filter(request_id=request_id).first()
        if existing and existing.status == "delivered":
            celery_logger.info(
                f"Email already delivered (idempotency)",
                extra={"request_id": request_id},
            )
            return {"status": "already_delivered", "request_id": request_id}
    except Exception as exc:
        celery_logger.error(
            f"Error checking existing record: {str(exc)}",
            extra={"request_id": request_id},
        )

    # Create or update log record atomically
    try:
        with transaction.atomic():
            log, created = EmailLog.objects.get_or_create(
                request_id=request_id,
                defaults={
                    "user_id": payload.get("user_id"),
                    "to_email": payload.get("variables", {}).get("email"),
                    "template_code": payload.get("template_code"),
                    "variables": payload.get("variables", {}),
                    "status": "processing",
                },
            )
        celery_logger.debug(
            f"Log record {'created' if created else 'updated'}",
            extra={"request_id": request_id},
        )
    except Exception as exc:
        celery_logger.error(
            f"Failed to create/update log: {str(exc)}",
            extra={"request_id": request_id},
        )
        raise self.retry(exc=exc, countdown=5)

    try:
        # Fetch template with circuit breaker
        template_code = payload.get("template_code")
        variables = payload.get("variables", {})

        template = _fetch_template_with_breaker(template_code, request_id)

        # Render template with variable substitution
        try:
            message_body = template.format(**variables)
        except (KeyError, ValueError) as e:
            celery_logger.warning(
                f"Template variable substitution failed: {str(e)}",
                extra={"request_id": request_id, "template_code": template_code},
            )
            # Use template as-is if substitution fails
            message_body = template

        to_email = variables.get("email")
        if not to_email:
            raise ValueError("Email address not provided in variables")

        subject = variables.get("subject") or f"Notification: {template_code}"

        # Send email with circuit breaker protection
        _send_email_with_breaker(
            subject=subject,
            message_body=message_body,
            to_email=to_email,
            request_id=request_id,
        )

        # Mark as delivered
        log.status = "delivered"
        log.error = ""
        log.attempts = (log.attempts or 0) + 1
        log.save()

        celery_logger.info(
            f"Email sent successfully",
            extra={"request_id": request_id, "to_email": to_email},
        )

        # Report status
        report_status(request_id, "delivered")

        return {"status": "delivered", "request_id": request_id}

    except Exception as exc:
        # Update log with error
        log.attempts = (log.attempts or 0) + 1
        log.error = str(exc)
        log.status = "failed" if log.attempts >= MAX_RETRIES else "pending"
        log.save()

        celery_logger.error(
            f"Email processing failed (attempt {log.attempts}/{MAX_RETRIES}): {str(exc)}",
            extra={"request_id": request_id},
        )

        # If max retries exceeded, move to dead-letter queue
        if log.attempts >= MAX_RETRIES:
            celery_logger.error(
                f"Max retries exceeded, sending to dead-letter queue",
                extra={"request_id": request_id},
            )
            try:
                publish_to_failed_queue(payload)
            except Exception as dq_exc:
                celery_logger.error(
                    f"Failed to publish to dead-letter queue: {str(dq_exc)}",
                    extra={"request_id": request_id},
                )

            try:
                report_status(request_id, "failed", error=str(exc))
            except Exception as status_exc:
                celery_logger.error(
                    f"Failed to report status: {str(status_exc)}",
                    extra={"request_id": request_id},
                )

            # Return failure response instead of raising
            return {"status": "failed", "request_id": request_id, "error": str(exc)}

        # Retry with exponential backoff
        retry_delay = min(2 ** log.attempts, 600)
        celery_logger.info(
            f"Retrying in {retry_delay} seconds",
            extra={"request_id": request_id},
        )
        raise self.retry(exc=exc, countdown=retry_delay)


def _fetch_template_with_breaker(template_code: str, request_id: str) -> str:
    """
    Fetch template with circuit breaker protection.

    Args:
        template_code: Template identifier
        request_id: Request ID for logging

    Returns:
        Template content string

    Raises:
        CircuitBreakerError: If circuit is open
    """
    try:
        return template_circuit_breaker.call(
            fetch_email_template, template_code=template_code
        )
    except Exception as exc:
        celery_logger.warning(
            f"Template fetch failed or circuit open: {str(exc)}",
            extra={"request_id": request_id, "template_code": template_code},
        )
        # Return default fallback template
        return "Hello {name},\n\nThis is an automated notification.\n\n{link}"


def _send_email_with_breaker(
    subject: str, message_body: str, to_email: str, request_id: str
) -> None:
    """
    Send email with circuit breaker protection.

    Args:
        subject: Email subject
        message_body: Email body
        to_email: Recipient email address
        request_id: Request ID for logging

    Raises:
        CircuitBreakerError: If circuit is open
        Exception: If email sending fails
    """
    try:
        smtp_circuit_breaker.call(
            _send_email_direct,
            subject=subject,
            message_body=message_body,
            to_email=to_email,
        )
    except Exception as exc:
        celery_logger.error(
            f"SMTP circuit breaker error: {str(exc)}",
            extra={"request_id": request_id},
        )
        raise


def _send_email_direct(subject: str, message_body: str, to_email: str) -> None:
    """
    Direct email sending via Django.

    Args:
        subject: Email subject
        message_body: Email body
        to_email: Recipient email address

    Raises:
        Exception: If email sending fails
    """
    email = EmailMessage(
        subject=subject,
        body=message_body,
        from_email=settings.EMAIL_FROM,
        to=[to_email],
    )
    email.content_subtype = "html" if "<html" in message_body else "plain"
    email.send(fail_silently=False)
