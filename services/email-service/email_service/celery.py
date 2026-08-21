import os

import dotenv
from celery import Celery

# Only fills in variables that are not already set, so real container
# configuration always wins over a local .env file.
dotenv.load_dotenv()

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'email_service.settings')


def _broker_url() -> str:
    explicit = os.getenv('CELERY_BROKER_URL')
    if explicit:
        return explicit

    user = os.getenv('RABBITMQ_USER', 'guest')
    password = os.getenv('RABBITMQ_PASSWORD', 'guest')
    host = os.getenv('RABBITMQ_HOST', 'rabbitmq')
    port = os.getenv('RABBITMQ_PORT', '5672')
    return f'amqp://{user}:{password}@{host}:{port}//'


app = Celery('email_service')

app.conf.broker_url = _broker_url()

# Results are not consumed anywhere: the delivery outcome is reported through
# the status callback, not through Celery. Leaving a result backend configured
# only adds a dependency that can fail at publish time.
app.conf.result_backend = os.getenv('CELERY_RESULT_BACKEND') or None
app.conf.task_ignore_result = True

app.conf.task_default_queue = 'email.queue'
app.conf.task_acks_late = True
app.conf.worker_prefetch_multiplier = 1
app.conf.task_serializer = 'json'
app.conf.result_serializer = 'json'
app.conf.accept_content = ['json']

app.autodiscover_tasks()
