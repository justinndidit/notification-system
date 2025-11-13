# How Your Email Service Works - Complete Flow Guide

## 🎯 Overview: The Big Picture

Your Email Service is **ONE PART** of a larger distributed system. Think of it like an assembly line:

```
API Gateway (Entry Point)
    ↓
    Sends notification request to your Email Service
    ↓
Email Service (You are here)
    ↓
    Processes and sends email
    ↓
Reports back to API Gateway
```

---

## 📨 1. What the API Gateway Sends

### The Request Format

The API Gateway will send a POST request to your Email Service like this:

```http
POST http://your-email-service-url/api/v1/notifications/
Content-Type: application/json

{
  "notification_type": "email",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "template_code": "welcome_email",
  "variables": {
    "name": "John Doe",
    "email": "john@example.com",
    "subject": "Welcome to Our Platform!",
    "link": "https://yourapp.com/verify?token=abc123"
  },
  "request_id": "req-20251113-001",
  "priority": 10,
  "metadata": {
    "correlation_id": "corr-12345",
    "campaign_id": "campaign-nov-2025",
    "source": "signup"
  }
}
```

### What Each Field Means

| Field | Purpose | Example |
|-------|---------|---------|
| `notification_type` | Type of notification (email/push) | `"email"` |
| `user_id` | Unique user identifier | `"550e8400-e29b..."` |
| `template_code` | Which template to use | `"welcome_email"` |
| `variables` | Data to fill into template | `{"name": "John", ...}` |
| `request_id` | Unique ID for this request (prevents duplicates) | `"req-20251113-001"` |
| `priority` | How urgent (1-100) | `10` |
| `metadata` | Extra info for tracking | `{"correlation_id": "..."}` |

---

## ⚙️ 2. What Your Email Service Does

### Step-by-Step Process

```
API Gateway sends request
    ↓
[1] Your Email Service receives request
    └─ Pydantic validates the format
    └─ Returns 202 Accepted immediately
    ↓
[2] Service queues the task in RabbitMQ
    └─ Message sits in email.queue
    ↓
[3] Celery Worker picks up the task
    └─ Worker pool has 4 workers (configurable)
    ↓
[4] Worker processes the email
    ├─ Check: Is request_id already processed? (Idempotency)
    ├─ Fetch: Template from Template Service
    ├─ Fill: Template with variables
    ├─ Send: Email via SMTP
    └─ Log: Everything with correlation_id
    ↓
[5] Update database with result
    └─ Status: "delivered" or "failed"
    ↓
[6] Report back to API Gateway
    └─ POST to STATUS_CALLBACK_URL
    ↓
[7] Client can check status anytime
    └─ GET /api/v1/notifications/{request_id}/
```

### Visual Timeline

```
Time    Event                           Who
────────────────────────────────────────────────────────
T0      API Gateway sends request       API Gateway → Email Service
        ↓
T0+10ms Your service validates          Email Service (Pydantic)
        ↓
T0+20ms Service returns 202             Email Service → API Gateway
        (Don't wait for email!)
        ↓
T0+50ms Task queued in RabbitMQ         Email Service → RabbitMQ
        ↓
T0+100ms Celery Worker picks up task    RabbitMQ → Celery Worker
        ↓
T0+2s   Fetches template                Celery Worker → Template Service
        ↓
T0+2.5s Fills template with data        Celery Worker (in-memory)
        ↓
T0+3s   Connects to SMTP server         Celery Worker → Gmail/SendGrid
        ↓
T0+5s   Email sent!                     SMTP Server → User's Email
        ↓
T0+5.1s Updates database                Celery Worker → PostgreSQL
        ↓
T0+5.2s Reports status callback         Celery Worker → API Gateway
        ↓
T0+5.3s Complete!                       Status logged
```

---

## 📊 3. The Data Flow Diagram

### Simple Version

```
┌──────────────────┐
│  API Gateway     │
│  (Frontend/App)  │
└────────┬─────────┘
         │ POST /api/v1/notifications/
         ▼
┌──────────────────────────────────┐
│  Your Email Service              │
│  (Django REST API)               │
│  ✓ Validate request              │
│  ✓ Return 202 Accepted           │
└────────┬─────────────────────────┘
         │ Queue task
         ▼
    ┌─────────────┐
    │  RabbitMQ   │  (Message Queue)
    │  email.queue│
    └──────┬──────┘
           │ Pick up task
           ▼
┌──────────────────────────────────┐
│  Celery Worker Pool              │
│  (Background Processing)         │
│  ✓ 4 workers running             │
│  ✓ Each processes 1 email at a   │
│    time                          │
└────────┬─────────────────────────┘
         │
    ┌────┴────┬──────────┬───────────┐
    ▼         ▼          ▼           ▼
 ┌─────┐  ┌────────┐  ┌────────┐  ┌──────────┐
 │Fetch│  │Render  │  │ SMTP   │  │ Report   │
 │Templ│  │ vars   │  │ Send   │  │ Status   │
 │ate  │  │        │  │        │  │ Callback │
 └─────┘  └────────┘  └────────┘  └──────────┘
    │         │          │          │
    └────┬────┴──────────┴──────────┘
         ▼
    ┌─────────────┐
    │ PostgreSQL  │  (Database)
    │ EmailLog    │
    │ table       │
    └─────────────┘
```

### Detailed Flow with Components

```
External Systems          Your Email Service               External Services
───────────────────      ──────────────────               ──────────────────

                         ┌──────────────────┐
                         │  Django API      │
API Gateway ────────────→│  Validates       │
                         │  Returns 202     │
                         └────────┬─────────┘
                                  │
                                  ▼
                         ┌──────────────────┐
                         │  RabbitMQ Queue  │
                         │  (AMQP Protocol) │
                         └────────┬─────────┘
                                  │
                    ┌─────────────┼─────────────┐
                    ▼             ▼             ▼
                ┌────────┐   ┌────────┐   ┌────────┐
                │Worker 1│   │Worker 2│   │Worker 3│
                └────┬───┘   └────┬───┘   └────┬───┘
                     │            │            │
              ┌──────┴────────────┴────────────┴──────┐
              │                                       │
              ▼                                       │
    Template Service ←─ Fetch Template               │
         (REST API)                                  │
                                                     │
    SMTP Server ←─────────────────────── Send Email │
    (Gmail, SendGrid, AWS SES, etc.)                 │
                                                     │
                                                     ▼
                                     ┌───────────────────────┐
                                     │ PostgreSQL Database   │
                                     │ - EmailLog table      │
                                     │ - Store status        │
                                     │ - Track attempts      │
                                     │ - Store errors        │
                                     └───────────┬───────────┘
                                                 │
                                                 ▼
                                    Status Callback
                                    POST to API Gateway
                                    /{notification_id}/status
```

---

## 🚀 4. Deploying on Railway - Step by Step

### Phase 1: Prepare Your Code (What You Have Now)

✅ **Already Done:**
- Email Service code (tasks.py, views.py, etc.)
- Pydantic schemas (validation)
- Celery configuration
- Docker setup
- CI/CD pipeline (GitHub Actions)

### Phase 2: Deploy to Railway

#### Step 1: Connect Your GitHub Repository

```
1. Go to railway.app
2. Click "New Project"
3. Select "Deploy from GitHub"
4. Connect your GitHub account
5. Select your EmailMicroService repository
```

#### Step 2: Set Up Environment Variables

Railway will ask you to set environment variables. Configure these:

```env
# Django
DJANGO_SETTINGS_MODULE=email_service.settings
DEBUG=False
SECRET_KEY=your-super-secret-key-change-this

# Database
DATABASE_URL=postgresql://user:password@your-db-host:5432/email_service

# Email Configuration
EMAIL_HOST=smtp.gmail.com
EMAIL_PORT=587
EMAIL_USE_TLS=True
EMAIL_HOST_USER=your-email@gmail.com
EMAIL_HOST_PASSWORD=your-app-password
EMAIL_FROM=noreply@yourapp.com

# RabbitMQ
RABBITMQ_HOST=your-rabbitmq-host
RABBITMQ_USER=guest
RABBITMQ_PASSWORD=guest

# External Services
TEMPLATE_SERVICE_URL=http://template-service.railway.app/api/v1/templates/
STATUS_CALLBACK_URL=http://api-gateway.railway.app/api/v1/notifications/status/
```

#### Step 3: Create a Dockerfile

Railway will automatically detect your Dockerfile. Make sure it's configured:

```dockerfile
FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
CMD ["python", "manage.py", "runserver", "0.0.0.0:8000"]
```

#### Step 4: Set Up Services on Railway

Create multiple services:

```
Service 1: Email Service API (Web Server)
├─ Command: python manage.py runserver 0.0.0.0:8000
├─ Port: 8000
└─ Expose: Yes

Service 2: Celery Worker (Background Jobs)
├─ Command: celery -A email_service worker --loglevel=info
├─ Port: None
└─ Expose: No

Service 3: PostgreSQL Database
├─ Type: Postgres
├─ Create new database
└─ Connect to Email Service

Service 4: RabbitMQ (Message Queue)
├─ Type: Container
├─ Use official RabbitMQ image
└─ Connect to both services
```

---

## 📋 5. What Happens After Deployment

### Your System Architecture on Railway

```
┌─────────────────────────────────────────────────────────┐
│                      Railway.app                        │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │             Email Service (Your App)             │ │
│  │                                                   │ │
│  │  API Container (8000)         Celery Container  │ │
│  │  ├─ receive requests           ├─ process emails│ │
│  │  ├─ validate data              ├─ retry logic   │ │
│  │  ├─ queue tasks                ├─ error handling│ │
│  │  └─ return 202                 └─ log events    │ │
│  │                                                   │ │
│  │  Database (PostgreSQL)    Broker (RabbitMQ)     │ │
│  │  ├─ EmailLog table        ├─ email.queue       │ │
│  │  ├─ Track status          ├─ push.queue        │ │
│  │  └─ Store history         └─ failed.queue      │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  Connected to External Services:                       │
│  ├─ API Gateway (incoming requests)                    │
│  ├─ Template Service (fetch templates)                 │
│  ├─ SMTP Server (send emails)                          │
│  └─ User Service (get user data)                       │
└─────────────────────────────────────────────────────────┘
```

### Request Flow After Deployment

```
User Signs Up
    ↓
API Gateway receives signup
    ↓
API Gateway sends to your Email Service (Railway)
    POST http://email-service.railway.app/api/v1/notifications/
    ↓
Your API Container (Django)
    ├─ Validates request (Pydantic)
    ├─ Returns 202 Accepted immediately
    └─ Queues task in RabbitMQ
    ↓
RabbitMQ (Message Queue on Railway)
    └─ Stores task in email.queue
    ↓
Celery Worker Container (on Railway)
    ├─ Picks up task from queue
    ├─ Fetches template from Template Service
    ├─ Fills template: "Welcome John!"
    ├─ Sends via SMTP (Gmail/SendGrid)
    ├─ Saves to PostgreSQL database
    └─ Reports status back to API Gateway
    ↓
User receives email! ✅
```

---

## 🔄 6. API Gateway Integration Points

### 1. Initial Request (Synchronous)

**API Gateway sends:**
```http
POST /api/v1/notifications/
{
  "notification_type": "email",
  "user_id": "user-123",
  "template_code": "welcome_email",
  "variables": {...},
  "request_id": "req-001"
}
```

**Your service returns immediately:**
```json
{
  "success": true,
  "message": "Notification queued for processing",
  "data": {
    "request_id": "req-001",
    "task_id": "celery-task-id",
    "status": "queued"
  }
}
```

**Key Point:** This response is FAST (<100ms) because we don't wait for email to send!

### 2. Status Callback (Asynchronous)

After email is sent, your service calls API Gateway:

```http
POST http://api-gateway.railway.app/api/v1/notifications/status/
{
  "notification_id": "req-001",
  "status": "delivered",
  "timestamp": "2025-11-13T10:30:05Z",
  "error": null
}
```

### 3. Status Query (API Gateway Pulls)

API Gateway can check status anytime:

```http
GET /api/v1/notifications/req-001/
```

**Response:**
```json
{
  "success": true,
  "data": {
    "request_id": "req-001",
    "status": "delivered",
    "attempts": 1,
    "created_at": "2025-11-13T10:30:00Z",
    "updated_at": "2025-11-13T10:30:05Z"
  }
}
```

---

## 📱 7. Example Real-World Scenario

### Scenario: User Signs Up on Your App

#### Timeline

```
T0:00    User enters email and clicks "Sign Up"
         └─ API Gateway receives request

T0:05    API Gateway sends to Email Service
         POST /api/v1/notifications/
         {
           "user_id": "new-user-456",
           "template_code": "welcome",
           "variables": {
             "email": "newuser@example.com",
             "name": "Jane Smith"
           },
           "request_id": "req-20251113-001"
         }

T0:15    Your Email Service validates and returns 202
         └─ User sees "Confirmation email sent!"

T0:20    Task queued in RabbitMQ

T0:50    Celery Worker picks it up

T1:00    Fetches welcome template from Template Service
         Template: "Hello {name}, Click here to verify: {link}"

T1:50    Fills in variables:
         "Hello Jane Smith, Click here to verify: https://..."

T2:00    Connects to SMTP (Gmail)

T3:00    Email sent! ✅
         User receives: "Welcome Jane Smith"

T3:10    Saves to PostgreSQL:
         INSERT INTO notifications_emaillog
         (request_id, status, attempts, sent_at)
         VALUES ('req-20251113-001', 'delivered', 1, NOW())

T3:20    Reports back to API Gateway:
         POST /api/notifications/status/
         {
           "notification_id": "req-20251113-001",
           "status": "delivered"
         }

T3:25    Done! Total time: 3.25 seconds
```

---

## 🔧 8. Monitoring & Troubleshooting on Railway

### Check Logs

```bash
# View API logs
railway logs --service email-service-api

# View Celery logs
railway logs --service email-service-worker

# View database logs
railway logs --service postgres
```

### Common Issues & Solutions

| Issue | Cause | Solution |
|-------|-------|----------|
| No emails sending | RabbitMQ not running | Check RabbitMQ service on Railway |
| Emails stuck in queue | Celery worker crashed | Restart Celery service |
| Database connection error | DATABASE_URL not set | Check environment variables |
| SMTP errors | Wrong email credentials | Update EMAIL_HOST_USER/PASSWORD |

---

## 📊 9. What Your Team Needs to Do

### API Gateway Service
- [ ] Call `POST /api/v1/notifications/` with correct format
- [ ] Handle 202 Accepted response
- [ ] Poll `GET /api/v1/notifications/{request_id}/` for status OR
- [ ] Listen for status callback at `STATUS_CALLBACK_URL`

### Template Service
- [ ] Expose `GET /api/v1/templates/{template_code}/`
- [ ] Return template with placeholders: `{name}`, `{email}`, `{link}`
- [ ] Return JSON: `{"template_content": "Hello {name}..."}`

### User Service
- [ ] Provide user email addresses to Email Service
- [ ] Store notification preferences
- [ ] Expose user lookup APIs if needed

### Push Service
- [ ] Use same architecture as Email Service
- [ ] Use FCM/OneSignal instead of SMTP
- [ ] Same circuit breaker + retry patterns

---

## 🎯 10. Summary: What to Tell Your Team

**When API Gateway sends a notification:**

1. **Your Email Service receives it** (Django REST API)
2. **Returns 202 immediately** (Don't wait for email)
3. **Queues in RabbitMQ** (Message stored safely)
4. **Celery Worker processes** (Async, in background)
5. **Email sent to user** (Via SMTP)
6. **Status reported back** (To API Gateway)
7. **Stored in database** (For audit trail)

**Key Benefits:**
- ✅ Fast API response (<100ms)
- ✅ Reliable delivery (retry logic)
- ✅ Scalable (multiple workers)
- ✅ Auditable (full history)
- ✅ Resilient (circuit breakers)

---

## 🚀 Next Steps After Deploying to Railway

1. **Get your Email Service URL from Railway**
   - Example: `https://email-service.railway.app`

2. **Share with API Gateway team**
   - They'll use: `POST https://email-service.railway.app/api/v1/notifications/`

3. **Configure your Status Callback URL**
   - Set: `STATUS_CALLBACK_URL=https://api-gateway.railway.app/api/v1/notifications/status/`

4. **Configure Template Service URL**
   - Set: `TEMPLATE_SERVICE_URL=https://template-service.railway.app/api/v1/templates/`

5. **Test end-to-end**
   - Have API Gateway send a test notification
   - Verify email is received
   - Check database logs

6. **Monitor in production**
   - Set up Datadog/NewRelic monitoring
   - Configure alerts
   - Review logs regularly

---

**That's it! Your Email Service is a complete, production-ready microservice. 🎉**
