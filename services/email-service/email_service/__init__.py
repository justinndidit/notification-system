# Ensure the Celery app is created and configured when Django starts, so that
# `celery -A email_service` and `shared_task` both use the configuration in
# celery.py rather than Celery's defaults (which point at localhost).
from .celery import app as celery_app

__all__ = ("celery_app",)
