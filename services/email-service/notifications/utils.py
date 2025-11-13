import os
import requests
import pika
import json

TEMPLATE_SERVICE_URL = os.getenv('TEMPLATE_SERVICE_URL', 'http://template-service:8000/templates/')
STATUS_CALLBACK_URL = os.getenv('STATUS_CALLBACK_URL', 'http://notification_service/api/notifications/status/')
RABBITMQ_HOST = os.getenv('RABBITMQ_HOST', 'localhost')
RABBITMQ_USER = os.getenv('RABBITMQ_USER', 'guest')
RABBITMQ_PASSWORD = os.getenv('RABBITMQ_PASSWORD', 'guest')


def fetch_email_template(template_code):
    try:
        resp = requests.get(f'http://template-service:8000/templates/{template_code}', timeout=5)
        resp.raise_for_status()
        data = resp.json()
        return data.get('template_content', '')
    except Exception:
        return "Hello {name}, \n\n(Template not available) \n\n{link}"
    
def report_status(notification_id, status, error=None):
    payload = {
        'notification_id': notification_id,
        'status': status,
    }
    if error:
        payload['error'] = str(error)
    try:
        requests.post(STATUS_CALLBACK_URL, json=payload, timeout=5)
    except Exception as e:
        pass


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