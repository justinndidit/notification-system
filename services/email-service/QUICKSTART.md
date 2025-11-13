# Quick Start Guide - Email Service

## ⚡ Get Started in 5 Minutes

### Prerequisites
- Python 3.11+
- Docker & Docker Compose
- Git

### Step 1: Clone & Setup
```bash
cd EmailMicroService
cp .env.example .env
```

### Step 2: Edit `.env`
Update these critical variables:
```bash
# Gmail example (requires app password, not regular password)
EMAIL_HOST_USER=your-email@gmail.com
EMAIL_HOST_PASSWORD=your-16-character-app-password
EMAIL_FROM=noreply@yourapp.com
```

### Step 3: Install & Migrate
```bash
pip install -r requirements.txt
python manage.py migrate
```

### Step 4: Start Services
```bash
docker-compose up
```

In another terminal:
```bash
celery -A email_service worker --loglevel=info
```

### Step 5: Test the API
```bash
# Send notification
curl -X POST http://localhost:8000/api/v1/notifications/ \
  -H "Content-Type: application/json" \
  -d '{
    "notification_type": "email",
    "user_id": "user-123",
    "template_code": "welcome",
    "variables": {
      "name": "John Doe",
      "email": "john@example.com",
      "subject": "Welcome!",
      "link": "https://example.com/verify"
    },
    "request_id": "req-001",
    "priority": 10
  }'

# Check status
curl http://localhost:8000/api/v1/notifications/req-001/

# Health check
curl http://localhost:8000/health/
```

---

## 📊 What You Get

| Feature | Status |
|---------|--------|
| Email Sending | ✅ |
| Async Processing | ✅ |
| Retry Logic | ✅ |
| Circuit Breaker | ✅ |
| Idempotency | ✅ |
| Status Tracking | ✅ |
| Logging | ✅ |
| API Endpoints | ✅ |
| CI/CD Pipeline | ✅ |

---

## 🔧 Configuration

### SMTP Providers

**Gmail:**
```env
EMAIL_HOST=smtp.gmail.com
EMAIL_PORT=587
EMAIL_USE_TLS=True
EMAIL_HOST_USER=your-email@gmail.com
EMAIL_HOST_PASSWORD=your-app-password  # Not your regular password
```

**SendGrid:**
```env
EMAIL_HOST=smtp.sendgrid.net
EMAIL_PORT=587
EMAIL_USE_TLS=True
EMAIL_HOST_USER=apikey
EMAIL_HOST_PASSWORD=SG.your-sendgrid-key
```

**AWS SES:**
```env
EMAIL_HOST=email-smtp.region.amazonaws.com
EMAIL_PORT=587
EMAIL_USE_TLS=True
EMAIL_HOST_USER=your-ses-username
EMAIL_HOST_PASSWORD=your-ses-password
```

---

## 📖 API Endpoints

### Send Email
```
POST /api/v1/notifications/
Content-Type: application/json

{
  "notification_type": "email",
  "user_id": "uuid-string",
  "template_code": "template-name",
  "variables": {
    "email": "recipient@example.com",
    "name": "John Doe",
    "subject": "Email Subject",
    "link": "https://example.com"
  },
  "request_id": "unique-id-for-idempotency",
  "priority": 10
}

Response: 202 Accepted
{
  "success": true,
  "message": "Notification queued for processing",
  "data": {
    "request_id": "unique-id",
    "task_id": "celery-task-id",
    "status": "queued"
  }
}
```

### Get Status
```
GET /api/v1/notifications/{request_id}/

Response: 200 OK
{
  "success": true,
  "message": "Notification status retrieved",
  "data": {
    "request_id": "unique-id",
    "status": "delivered",
    "attempts": 1,
    "error": null,
    "created_at": "2025-11-13T10:30:00Z",
    "updated_at": "2025-11-13T10:30:05Z"
  }
}
```

### List Notifications
```
GET /api/v1/notifications/list/?user_id=uuid&status=delivered&limit=20&page=1

Response: 200 OK
{
  "success": true,
  "message": "Retrieved 20 notifications",
  "data": [...],
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
```
GET /health/

Response: 200 OK
{
  "success": true,
  "message": "Service is healthy",
  "data": {
    "status": "healthy",
    "service": "email-service",
    "version": "1.0.0"
  }
}
```

---

## 🔍 Monitoring

### Check Active Tasks
```bash
celery -A email_service inspect active
```

### View RabbitMQ UI
```
http://localhost:15672
Username: guest
Password: guest
```

### View Flower (Celery Monitoring)
```bash
celery -A email_service flower
# Open http://localhost:5555
```

### View Logs
```bash
docker-compose logs -f email_service
```

---

## 🐛 Troubleshooting

### Celery worker not processing tasks
```bash
# Check if RabbitMQ is running
docker-compose ps

# Check celery worker
ps aux | grep celery

# Inspect active tasks
celery -A email_service inspect active

# Clear queue if needed
celery -A email_service purge
```

### SMTP connection error
```bash
# Test SMTP in Python shell
python manage.py shell

>>> from django.core.mail import send_mail
>>> from django.conf import settings
>>> send_mail(
...     'Test Subject',
...     'Test body',
...     settings.EMAIL_FROM,
...     ['test@example.com'],
... )
```

### Database error
```bash
# Run migrations
python manage.py migrate

# Check database connectivity
python manage.py dbshell
```

---

## 📚 Learn More

- **Full API Docs**: See `README.md`
- **Architecture**: See `SYSTEM_DESIGN.md`
- **Requirements Analysis**: See `ANALYSIS.md`
- **Implementation**: See `IMPLEMENTATION_SUMMARY.md`

---

## ✅ Common Tasks

### Deploy to Production
```bash
# Set GitHub secrets for deployment
# Then push to main branch
git push origin main
```

### Scale Workers
Edit `docker-compose.yml`:
```yaml
email_service:
  command: ["sh", "-c", "celery -A email_service worker --loglevel=info --concurrency=8"]
```

### Change Database
Update `DATABASE_URL` in `.env`:
```env
# PostgreSQL
DATABASE_URL=postgresql://user:pass@localhost:5432/email_service

# Then migrate
python manage.py migrate
```

---

## 🎓 What You Need to Know

1. **Async Processing**: Emails are sent in background via Celery
2. **Idempotency**: Use same `request_id` = same email (no duplicates)
3. **Retry Logic**: Automatic retries with exponential backoff
4. **Circuit Breaker**: If SMTP fails too much, service falls back gracefully
5. **Dead Letter Queue**: Permanently failed messages stored for review
6. **Correlation IDs**: Track emails across services in logs

---

## 🚀 You're Ready!

Your Email Service is configured and ready to use. 

Start sending emails! 📧

---

**Need Help?**
- Check `README.md` for detailed API docs
- Review `SYSTEM_DESIGN.md` for architecture
- Look at logs: `docker-compose logs -f email_service`
- Test with curl examples above

Happy sending! 🎉
