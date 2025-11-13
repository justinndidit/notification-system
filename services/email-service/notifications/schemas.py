"""
Pydantic schemas for request/response validation.
Follows snake_case naming convention.
"""

from pydantic import BaseModel, EmailStr, HttpUrl, Field
from typing import Optional, Dict, Any
from enum import Enum
from datetime import datetime


class NotificationType(str, Enum):
    email = "email"
    push = "push"


class NotificationStatus(str, Enum):
    pending = "pending"
    processing = "processing"
    delivered = "delivered"
    failed = "failed"
    bounced = "bounced"


class NotificationRequest(BaseModel):
    """
    Request schema for sending notifications.
    """
    notification_type: NotificationType
    user_id: str = Field(..., description="UUID of the user")
    template_code: str = Field(..., description="Template identifier")
    variables: Dict[str, Any] = Field(default_factory=dict)
    request_id: str = Field(..., description="Unique request ID for idempotency")
    priority: Optional[int] = Field(default=10, ge=1, le=100)
    metadata: Optional[Dict[str, Any]] = Field(default_factory=dict)

    class Config:
        json_schema_extra = {
            "example": {
                "notification_type": "email",
                "user_id": "550e8400-e29b-41d4-a716-446655440000",
                "template_code": "welcome_email",
                "variables": {
                    "name": "Joe",
                    "email": "joe@example.com",
                    "link": "https://example.com/verify",
                    "subject": "Welcome to Our Platform"
                },
                "request_id": "req-12345-67890",
                "priority": 10,
                "metadata": {}
            }
        }


class PaginationMeta(BaseModel):
    """
    Standard pagination metadata.
    """
    total: int
    limit: int
    page: int
    total_pages: int
    has_next: bool
    has_previous: bool


class ApiResponse(BaseModel):
    """
    Standard API response format.
    """
    success: bool
    data: Optional[Dict[str, Any]] = None
    error: Optional[str] = None
    message: str
    meta: Optional[PaginationMeta] = None

    class Config:
        json_schema_extra = {
            "example": {
                "success": True,
                "data": {
                    "request_id": "req-12345-67890",
                    "status": "delivered",
                    "created_at": "2025-11-13T10:30:00Z"
                },
                "error": None,
                "message": "Email sent successfully",
                "meta": None
            }
        }


class NotificationStatusUpdate(BaseModel):
    """
    Status update payload for status callback.
    """
    notification_id: str = Field(..., alias="request_id")
    status: NotificationStatus
    timestamp: Optional[datetime] = None
    error: Optional[str] = None

    class Config:
        populate_by_name = True


class EmailLogResponse(BaseModel):
    """
    Response model for email log retrieval.
    """
    request_id: str
    user_id: str
    to_email: str
    template_code: str
    status: NotificationStatus
    attempts: int
    error: Optional[str] = None
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True


class HealthCheckResponse(BaseModel):
    """
    Response for health check endpoint.
    """
    status: str
    service: str = "email-service"
    version: str = "1.0.0"
    timestamp: datetime = Field(default_factory=datetime.utcnow)
