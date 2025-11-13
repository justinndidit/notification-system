"""
URL configuration for email_service project.

The `urlpatterns` list routes URLs to views. For more information please see:
    https://docs.djangoproject.com/en/5.2/topics/http/urls/
"""
from django.contrib import admin
from django.urls import path
from notifications.views import (
    health,
    send_notification,
    get_notification_status,
    list_notifications,
)

urlpatterns = [
    path("admin/", admin.site.urls),
    path("health/", health, name="health"),
    path("api/v1/notifications/", send_notification, name="send_notification"),
    path(
        "api/v1/notifications/<str:request_id>/",
        get_notification_status,
        name="get_notification_status",
    ),
    path("api/v1/notifications/list/", list_notifications, name="list_notifications"),
]
