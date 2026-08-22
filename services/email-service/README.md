# Email Service

Delivers email over SMTP, with retries, circuit breaking and dead-lettering.

Part of the [Notification System](../../README.md).

| | |
|---|---|
| **Stack** | Django 5.2, Celery, pybreaker, pika |
| **Port** | 8000 (published on 8003) |
| **Owns** | `email_service_db` — `EmailLog`, one row per `request_id` |
| **Consumes** | `email_queue`, bound to `notification.email` |

---

## Three processes, one image

| Process | Command | Role |
|---|---|---|
| API | `runserver` | Direct HTTP submission, status queries |
| Bridge | `manage.py consume_notifications` | Reads the queue, dispatches to Celery |
| Worker | `celery -A email_service worker` | Performs the SMTP send |

**The bridge is not optional.** The orchestrator publishes plain JSON, while
Celery expects its own wire protocol — a raw message on the queue can never
dispatch as a task. The bridge reads
[the contract](../../docs/contracts/enriched-notification.md), translates it into
the task's payload, and hands it over. Without it the queue simply fills up.

---

## Delivery guarantees

`send_email_task` runs with `acks_late=True` and checks `EmailLog` for an
already-delivered `request_id` before doing any work, so a redelivered message
does not send twice.

Both the SMTP send and the template fetch sit behind separate circuit breakers
(5 failures / 60s reset for SMTP; 3 / 30s for templates), so a sustained outage
fails fast instead of queueing thousands of doomed calls.

Retries use exponential backoff to a 600-second cap, up to five attempts, after
which the payload is published to a durable `failed.queue` for inspection rather
than dropped.

Every outcome is reported back to the orchestrator — including the deduplicated
path. Returning silently there left the notification sitting at `queued` forever,
waiting for a result that was never coming.

---

## API

| Method | Route | |
|---|---|---|
| `POST` | `/api/v1/notifications/` | Direct submission, bypassing the queue |
| `GET` | `/api/v1/notifications/{request_id}/` | Status |
| `GET` | `/api/v1/notifications/list/` | Recent deliveries |
| `GET` | `/health/` | Liveness |

The direct path has no orchestrator record behind it, so it renders from a
fetched template rather than using pre-rendered content, and reports no status.

---

## Running it

```bash
docker compose -f ../../infra/docker-compose.local.yaml up \
  email-service email-worker email-bridge
```

Locally this points at **MailHog** (`http://localhost:8025`), so no real mail is
ever sent.

| Variable | Purpose |
|---|---|
| `SECRET_KEY` | Required when `DEBUG` is off — the service refuses to start otherwise |
| `DEBUG`, `ALLOWED_HOSTS` | Both must be set explicitly in production |
| `DATABASE_URL`, `DB_SSL_REQUIRE` | Postgres; SSL off for a local container, on for managed providers |
| `EMAIL_HOST`, `EMAIL_PORT`, `EMAIL_USE_TLS`, `EMAIL_HOST_USER`, `EMAIL_HOST_PASSWORD`, `EMAIL_FROM` | SMTP |
| `RABBITMQ_*`, `CELERY_BROKER_URL` | Broker |
| `JWT_SECRET` | Signs the short-lived service token used for status callbacks |

---

## Tests

```bash
python manage.py test notifications
```

Nine tests on the bridge's payload translation. That translation is the only
thing keeping a Go producer and a Python consumer in step — every integration
bug this service has had lived exactly there, including reporting the caller's
idempotency key where the callback expected a notification UUID.
