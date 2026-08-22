from django.apps import AppConfig


class NotificationsConfig(AppConfig):
    default_auto_field = 'django.db.models.BigAutoField'
    name = 'notifications'

    def ready(self):
        # Applies to all three processes started from this image: the HTTP API,
        # the Celery worker and the queue bridge.
        from notifications.tracing import configure

        configure()
