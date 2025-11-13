# Email Microservice

A scalable, resilient email notification microservice built with Django, Celery, and RabbitMQ. Part of a distributed notification system.

## 📋 Features

- **Asynchronous Processing**: Uses Celery for background email processing
- **Message Queue Integration**: RabbitMQ for reliable message delivery
- **Circuit Breaker Pattern**: Prevents cascading failures with automatic recovery
- **Idempotency**: Prevents duplicate emails using request IDs
- **Comprehensive Logging**: Structured JSON logging with correlation IDs
- **Retry Logic**: Exponential backoff with configurable max retries
- **Dead-Letter Queue**: Failed messages are persisted for manual inspection
- **Template Support**: Dynamic template rendering with variable substitution
- **REST API**: Standard request/response format with proper HTTP status codes
- **Health Checks**: Built-in health endpoint for monitoring
- **Docker Support**: Containerized for easy deployment
- **CI/CD Pipeline**: GitHub Actions workflow for automated testing and deployment

## 🏗️ Architecture

```
┌─────────────────────┐
│  API Gateway        │
│ (sends requests)    │
└──────────┬──────────┘
           │
    POST /api/v1/notifications/
           │
           ▼
┌─────────────────────┐
│  Email Service API  │
│  (validates)        │
└──────────┬──────────┘
           │
           ▼
    ┌─────────────┐
    │  RabbitMQ   │
    │ (queue)     │
    └──────┬──────┘
           │
           ▼
┌─────────────────────┐
│  Celery Worker      │
│  (processes)        │
└──────────┬──────────┘
           │
    ┌──────┴──────┬──────────┐
    │             │          │
    ▼             ▼          ▼
┌────────┐  ┌────────┐  ┌──────────┐
│ SMTP   │  │Template│  │ Status   │
│        │  │Service │  │ Callback │
└────────┘  └────────┘  └──────────┘
```

## 🚀 Quick Start

### Prerequisites

- Python 3.11+
- Docker & Docker Compose
- PostgreSQL (for production)
- RabbitMQ (message broker)

### Local Development

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd EmailMicroService
   ```

2. **Create environment file**
   ```bash
   cp .env.example .env
   ```

3. **Install dependencies**
   ```bash
   pip install -r requirements.txt
   ```

4. **Run database migrations**
   ```bash
   python manage.py migrate
   ```

5. **Start services with Docker Compose**
   ```bash
   docker-compose up
   ```

6. **In another terminal, run Celery worker**
   ```bash
   celery -A email_service worker --loglevel=info
   ```

7. **Test the API**
   ```bash
   curl -X POST http://localhost:8000/api/v1/notifications/ \
     -H "Content-Type: application/json" \
     -d '{
       "notification_type": "email",
       "user_id": "550e8400-e29b-41d4-a716-446655440000",
       "template_code": "welcome_email",
       "variables": {
         "name": "John Doe",
         "email": "john@example.com",
         "link": "https://example.com/verify",
         "subject": "Welcome to Our Platform"
       },
       "request_id": "req-12345-67890",
       "priority": 10,
       "metadata": {}
     }'
   ```

## 📖 API Documentation

### Send Notification

**Endpoint**: `POST /api/v1/notifications/`

**Request Body**:
```json
{
  "notification_type": "email",
  "user_id": "uuid-string",
  "template_code": "welcome_email",
  "variables": {
    "name": "John",
    "email": "john@example.com",
    "subject": "Welcome!",
    "link": "https://example.com/verify"
  },
  "request_id": "unique-request-id",
  "priority": 10,
  "metadata": {
    "campaign_id": "campaign-123"
  }
}
```

**Response** (202 - Accepted):
```json
{
  "success": true,
  "message": "Notification queued for processing",
  "data": {
    "request_id": "unique-request-id",
    "task_id": "celery-task-id",
    "status": "queued"
  },
  "error": null,
  "meta": null
}
```

### Get Notification Status

**Endpoint**: `GET /api/v1/notifications/{request_id}/`

**Response** (200 - OK):
```json
{
  "success": true,
  "message": "Notification status retrieved",
  "data": {
    "request_id": "unique-request-id",
    "user_id": "user-uuid",
    "to_email": "john@example.com",
    "template_code": "welcome_email",
    "status": "delivered",
    "attempts": 1,
    "error": null,
    "created_at": "2025-11-13T10:30:00",
    "updated_at": "2025-11-13T10:30:05"
  },
  "error": null,
  "meta": null
}
```

### List Notifications

**Endpoint**: `GET /api/v1/notifications/list/?user_id=uuid&status=delivered&limit=20&page=1`

**Query Parameters**:
- `user_id` (optional): Filter by user ID
- `status` (optional): Filter by status (pending, processing, delivered, failed)
- `limit` (optional): Results per page (default: 20, max: 100)
- `page` (optional): Page number (default: 1)

**Response** (200 - OK):
```json
{
  "success": true,
  "message": "Retrieved 20 notifications",
  "data": [
    {
      "request_id": "req-1",
      "user_id": "user-uuid",
      "to_email": "john@example.com",
      "template_code": "welcome_email",
      "status": "delivered",
      "attempts": 1,
      "created_at": "2025-11-13T10:30:00",
      "updated_at": "2025-11-13T10:30:05"
    }
  ],
  "error": null,
  "meta": {
    "total": 100,
    "limit": 20,
    "page": 1,
    "total_pages": 5,
    "has_next": true,
    "has_previous": false
  }
}
```

### Health Check

**Endpoint**: `GET /health/`

**Response** (200 - OK):
```json
{
  "success": true,
  "message": "Service is healthy",
  "data": {
    "status": "healthy",
    "service": "email-service",
    "version": "1.0.0",
    "timestamp": "2025-11-13T10:30:00"
  },
  "error": null,
  "meta": null
}
```

## 🔧 Configuration

### Environment Variables

Create a `.env` file with the following variables:

```bash
# Django
DJANGO_SETTINGS_MODULE=email_service.settings
DEBUG=False
SECRET_KEY=your-secret-key

# Database (SQLite for dev, PostgreSQL for prod)
DATABASE_URL=postgresql://user:password@localhost:5432/email_service

# Email Configuration
EMAIL_HOST=smtp.gmail.com
EMAIL_PORT=587
EMAIL_USE_TLS=True
EMAIL_HOST_USER=your-email@gmail.com
EMAIL_HOST_PASSWORD=your-app-password
EMAIL_FROM=noreply@example.com

# RabbitMQ Configuration
RABBITMQ_HOST=rabbitmq
RABBITMQ_USER=guest
RABBITMQ_PASSWORD=guest

# Template Service
TEMPLATE_SERVICE_URL=http://template-service:8000/templates/

# Status Callback
STATUS_CALLBACK_URL=http://api-gateway:8000/api/v1/notifications/status/

# Logging
LOG_LEVEL=INFO

# Redis (optional, for caching)
REDIS_URL=redis://localhost:6379/0
```

## 📊 Key Features Explained

### Circuit Breaker Pattern

Protects against cascading failures when external services (SMTP, Template Service) are unavailable:

```python
smtp_circuit_breaker = CircuitBreaker(
    fail_max=5,              # Open after 5 failures
    reset_timeout=60,        # Try again after 60 seconds
    exclude=[ValueError],    # Don't count validation errors
)
```

**States**:
- **Closed**: Normal operation, requests pass through
- **Open**: Service unavailable, requests fail fast
- **Half-Open**: Testing if service recovered

### Idempotency

Prevents duplicate emails by tracking `request_id`:

```python
existing = EmailLog.objects.filter(request_id=request_id).first()
if existing and existing.status == "delivered":
    return {"status": "already_delivered"}
```

### Retry Logic

Exponential backoff with jitter for transient failures:

```python
# Retry with exponential backoff: 2^attempt seconds
retry_delay = min(2 ** attempts, 600)  # Max 10 minutes
raise self.retry(exc=exc, countdown=retry_delay)
```

### Dead-Letter Queue

Permanently failed messages are stored for manual inspection:

```python
if attempts >= MAX_RETRIES:
    publish_to_failed_queue(payload)  # Store in failed.queue
    report_status(request_id, "failed")
```

## 🧪 Testing

### Run Tests

```bash
pytest notifications/tests.py -v
```

### Run with Coverage

```bash
pytest notifications/tests.py --cov=notifications --cov-report=html
```

### Integration Test

```bash
# Start Docker Compose
docker-compose up

# Send test notification
curl -X POST http://localhost:8000/api/v1/notifications/ \
  -H "Content-Type: application/json" \
  -d '{...}'

# Check status
curl http://localhost:8000/api/v1/notifications/req-12345-67890/
```

## 📦 Docker Deployment

### Build Image

```bash
docker build -t email-service:latest .
```

### Run with Docker Compose

```bash
docker-compose up -d
```

### View Logs

```bash
docker-compose logs -f email_service
```

## 🚀 Production Deployment

### Prerequisites

1. PostgreSQL database
2. RabbitMQ broker
3. SMTP credentials (Gmail, SendGrid, etc.)
4. Server for deployment

### Environment Setup

```bash
# Set production variables
export DEBUG=False
export SECRET_KEY=$(python -c 'from django.core.management.utils import get_random_secret_key; print(get_random_secret_key())')
export DATABASE_URL=postgresql://user:password@db-host:5432/email_service
export EMAIL_HOST_PASSWORD=your-app-password
```

### Database Migration

```bash
python manage.py migrate
```

### Run Celery Worker

```bash
celery -A email_service worker \
  --loglevel=info \
  --concurrency=4 \
  --max-tasks-per-child=1000
```

### Monitor with Flower

```bash
celery -A email_service flower
# Access at http://localhost:5555
```

## 📊 Performance Targets

- **Throughput**: 1,000+ notifications per minute
- **API Response Time**: <100ms (p99)
- **Delivery Success Rate**: 99.5%
- **Retry Handling**: Exponential backoff with max 5 retries

## 🔍 Monitoring

### Health Check

```bash
curl http://localhost:8000/health/
```

### Flower Dashboard

```bash
celery -A email_service flower
# Open http://localhost:5555
```

### RabbitMQ Management

```bash
# Access at http://localhost:15672
# Default credentials: guest/guest
```

## 🛠️ Troubleshooting

### Workers not processing tasks

```bash
# Check Celery worker is running
ps aux | grep celery

# Check RabbitMQ connection
docker logs email_service

# Inspect tasks
celery -A email_service inspect active
```

### SMTP Connection Issues

```bash
# Test SMTP credentials
python manage.py shell
>>> from django.core.mail import send_mail
>>> send_mail('Test', 'Test body', 'from@example.com', ['to@example.com'])
```

### Database Connection Issues

```bash
# Check database connectivity
python manage.py dbshell

# Run migrations
python manage.py migrate --verbosity 2
```

## 📝 Logging

All events are logged with correlation IDs for tracing:

```json
{
  "timestamp": "2025-11-13T10:30:00.123456",
  "level": "INFO",
  "logger": "email_service.celery",
  "message": "Email sent successfully",
  "correlation_id": "req-12345-67890",
  "module": "tasks",
  "function": "send_email_task",
  "line": 142
}
```

## 📚 Additional Resources

- [Django Documentation](https://docs.djangoproject.com/)
- [Celery Documentation](https://docs.celeryproject.org/)
- [RabbitMQ Documentation](https://www.rabbitmq.com/documentation.html)
- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)

## 📄 License

This project is part of a distributed notification system. See LICENSE file for details.

## 👥 Team

- Email Service: Responsible for email delivery
- API Gateway: Entry point and request routing
- User Service: User data and preferences
- Template Service: Template management
- Push Service: Push notification delivery

## 🎯 Roadmap

- [ ] Add support for email attachments
- [ ] Implement bounce handling
- [ ] Add A/B testing for subject lines
- [ ] Webhook system for custom integrations
- [ ] Advanced analytics dashboard
- [ ] Rate limiting per user
