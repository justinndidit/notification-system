"""
Views for Email Service API endpoints.
"""

from django.http import JsonResponse
from django.views.decorators.http import require_http_methods
from django.views.decorators.csrf import csrf_exempt
from django.db.models import Q
from datetime import datetime
import json

from .models import EmailLog
from .tasks import send_email_task
from .schemas import (
    NotificationRequest,
    ApiResponse,
    HealthCheckResponse,
    EmailLogResponse,
    NotificationStatus,
)
from .logging_config import logger

import logging

# Get logger
django_logger = logging.getLogger("email_service")


def _wrap_response(
    success: bool,
    message: str,
    data=None,
    error=None,
    status_code=200,
):
    """
    Wrap response in standard API format.

    Args:
        success: Whether the operation was successful
        message: Human-readable message
        data: Response data
        error: Error message if applicable
        status_code: HTTP status code

    Returns:
        JsonResponse with wrapped data
    """
    response = {
        "success": success,
        "message": message,
        "data": data,
        "error": error,
        "meta": None,
    }
    return JsonResponse(response, status=status_code)


@require_http_methods(["GET"])
def health(request):
    """
    Health check endpoint.

    Returns:
        200: Service is healthy
    """
    health_check = {
        "status": "healthy",
        "service": "email-service",
        "version": "1.0.0",
        "timestamp": datetime.utcnow().isoformat(),
    }
    return _wrap_response(
        success=True,
        message="Service is healthy",
        data=health_check,
        status_code=200,
    )


@csrf_exempt
@require_http_methods(["POST"])
def send_notification(request):
    """
    Send email notification.

    Expected payload:
    {
        "notification_type": "email",
        "user_id": "uuid",
        "template_code": "welcome_email",
        "variables": {"name": "Joe", "email": "joe@example.com", "subject": "Welcome"},
        "request_id": "unique-id",
        "priority": 10,
        "metadata": {}
    }

    Returns:
        202: Notification queued for processing
        400: Invalid request
        409: Request already processed (idempotency)
    """
    try:
        payload = json.loads(request.body)
    except json.JSONDecodeError:
        return _wrap_response(
            success=False,
            message="Invalid JSON payload",
            error="Request body must be valid JSON",
            status_code=400,
        )

    # Validate request
    try:
        notification_request = NotificationRequest(**payload)
    except Exception as validation_error:
        return _wrap_response(
            success=False,
            message="Validation error",
            error=str(validation_error),
            status_code=400,
        )

    # Check idempotency - if request already processed, return 409
    request_id = notification_request.request_id
    existing_log = EmailLog.objects.filter(request_id=request_id).first()
    if existing_log:
        if existing_log.status == "delivered":
            return _wrap_response(
                success=True,
                message="Notification already delivered (idempotency)",
                data={
                    "request_id": request_id,
                    "status": "delivered",
                    "delivered_at": existing_log.updated_at.isoformat(),
                },
                status_code=409,
            )
        elif existing_log.status in ["processing", "pending"]:
            return _wrap_response(
                success=True,
                message="Notification is being processed",
                data={
                    "request_id": request_id,
                    "status": existing_log.status,
                },
                status_code=202,
            )

    # Queue the task
    try:
        task = send_email_task.delay(payload)
        django_logger.info(
            f"Email task queued",
            extra={
                "request_id": request_id,
                "task_id": task.id,
                "user_id": notification_request.user_id,
            },
        )
        return _wrap_response(
            success=True,
            message="Notification queued for processing",
            data={
                "request_id": request_id,
                "task_id": task.id,
                "status": "queued",
            },
            status_code=202,
        )
    except Exception as exc:
        django_logger.error(
            f"Failed to queue email task: {str(exc)}",
            extra={"request_id": request_id},
        )
        return _wrap_response(
            success=False,
            message="Failed to queue notification",
            error=str(exc),
            status_code=500,
        )


@require_http_methods(["GET"])
def get_notification_status(request, request_id):
    """
    Get notification status by request ID.

    Args:
        request_id: Unique notification request ID

    Returns:
        200: Status found
        404: Status not found
    """
    try:
        log = EmailLog.objects.get(request_id=request_id)
        response_data = {
            "request_id": log.request_id,
            "user_id": log.user_id,
            "to_email": log.to_email,
            "template_code": log.template_code,
            "status": log.status,
            "attempts": log.attempts,
            "error": log.error,
            "created_at": log.created_at.isoformat(),
            "updated_at": log.updated_at.isoformat(),
        }
        return _wrap_response(
            success=True,
            message="Notification status retrieved",
            data=response_data,
            status_code=200,
        )
    except EmailLog.DoesNotExist:
        return _wrap_response(
            success=False,
            message="Notification not found",
            error=f"No notification with request_id={request_id}",
            status_code=404,
        )
    except Exception as exc:
        django_logger.error(
            f"Error retrieving notification status: {str(exc)}",
            extra={"request_id": request_id},
        )
        return _wrap_response(
            success=False,
            message="Error retrieving status",
            error=str(exc),
            status_code=500,
        )


@require_http_methods(["GET"])
def list_notifications(request):
    """
    List notifications with filtering and pagination.

    Query parameters:
        - user_id: Filter by user ID
        - status: Filter by status (pending, processing, delivered, failed)
        - limit: Results per page (default 20)
        - page: Page number (default 1)

    Returns:
        200: List of notifications
    """
    try:
        # Get filters
        user_id = request.GET.get("user_id")
        status = request.GET.get("status")
        limit = int(request.GET.get("limit", 20))
        page = int(request.GET.get("page", 1))

        # Validate limit
        limit = min(limit, 100)

        # Build query
        query = EmailLog.objects.all()
        if user_id:
            query = query.filter(user_id=user_id)
        if status:
            query = query.filter(status=status)

        # Calculate pagination
        total = query.count()
        total_pages = (total + limit - 1) // limit
        offset = (page - 1) * limit

        # Get paginated results
        logs = query.order_by("-created_at")[offset : offset + limit]

        response_data = [
            {
                "request_id": log.request_id,
                "user_id": log.user_id,
                "to_email": log.to_email,
                "template_code": log.template_code,
                "status": log.status,
                "attempts": log.attempts,
                "created_at": log.created_at.isoformat(),
                "updated_at": log.updated_at.isoformat(),
            }
            for log in logs
        ]

        meta = {
            "total": total,
            "limit": limit,
            "page": page,
            "total_pages": total_pages,
            "has_next": page < total_pages,
            "has_previous": page > 1,
        }

        return JsonResponse(
            {
                "success": True,
                "message": f"Retrieved {len(response_data)} notifications",
                "data": response_data,
                "error": None,
                "meta": meta,
            },
            status=200,
        )
    except Exception as exc:
        django_logger.error(f"Error listing notifications: {str(exc)}")
        return _wrap_response(
            success=False,
            message="Error retrieving notifications",
            error=str(exc),
            status_code=500,
        )
