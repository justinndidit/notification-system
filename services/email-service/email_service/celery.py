import os
from celery import Celery
import dotenv

dotenv.load_dotenv()

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'email_service.settings')

app = Celery('email_service')

# ✅ Correct property is app.conf, not app.config
app.conf.broker_url = f'amqp://{os.getenv("RABBITMQ_USER")}:{os.getenv("RABBITMQ_PASSWORD")}@{os.getenv("RABBITMQ_HOST")}:5672//'
app.conf.task_default_queue = 'email.queue'
app.conf.task_acks_late = True
app.conf.worker_prefetch_multiplier = 1
app.conf.task_serializer = 'json'
app.conf.result_serializer = 'json'
app.conf.accept_content = ['json']

app.autodiscover_tasks()
