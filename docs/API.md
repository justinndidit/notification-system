# API Reference

All client traffic enters through the API Gateway on `:8000`. Service ports are
listed for direct access during development; in a deployment only the gateway is
exposed.

Related: [Architecture](./ARCHITECTURE.md) · [Operations](./OPERATIONS.md)

---

## Conventions

### Response envelope

Every JSON response uses the same shape:

```json
{
  "success": true,
  "data": {},
  "message": "Request successful",
  "meta": {}
}
```

On failure, `success` is `false`, `data` is `null`, and `error` carries the
detail.

### Authentication

Send the JWT from `POST /user/signin` as a bearer token:

```
Authorization: Bearer <token>
```

Everything except `signup`, `signin` and the health endpoints requires one.

### Status codes

| Code | Meaning |
|---|---|
| `200` | Success, or a recognised duplicate submission |
| `202` | Accepted for asynchronous processing |
| `400` | Malformed request — a bad UUID, a missing required field |
| `401` | Missing, expired or invalid token |
| `403` | Authenticated, but not permitted |
| `404` | No such resource |
| `409` | Valid request, but the resource is not in a state that allows it |
| `429` | Rate limit exceeded |
| `502` | An upstream service is unreachable |

### Rate limiting

Default 100 requests per 60 seconds per client, configurable through
`THROTTLE_LIMIT` and `THROTTLE_TTL`. Counters live in Redis and are shared across
gateway replicas. `/health` is exempt so probes do not consume the budget.

---

## Notifications

### Submit a notification

```http
POST /notifications
Authorization: Bearer <token>
X-Idempotency-Key: <caller-supplied key>
Content-Type: application/json
```

```json
{
  "notification_type": "email",
  "user_id": "9c8b7a65-4321-4f0e-9d8c-7b6a5f4e3d2c",
  "template_code": "3e2d1c0b-9a87-4655-b4a3-2c1d0e9f8a7b",
  "variables": { "name": "Ada", "link": "https://example.com/start" },
  "request_id": "order-4821-confirmation",
  "priority": 2
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `notification_type` | `email` \| `push` | yes | Determines the routing key |
| `user_id` | UUID | yes | Recipient |
| `template_code` | UUID | yes | Template to render |
| `variables` | object | yes | Handlebars context |
| `request_id` | string | yes | Caller's reference |
| `priority` | 1–4 | yes | low, normal, high, urgent |
| `metadata` | object | no | Passthrough, stored and forwarded |

`X-Idempotency-Key` is required. The gateway generates one if the client omits
it, but supplying your own is what makes a retry safe.

**`202 Accepted`**

```json
{
  "success": true,
  "message": "Notification accepted and being processed",
  "data": {
    "correlation_id": "b2a1f0c9-1234-4d5e-8f90-1a2b3c4d5e6f",
    "idempotency_key": "order-4821-confirmation",
    "status": "processing"
  }
}
```

Enrichment continues after the response. Poll the correlation endpoint below
rather than blocking.

**`200 OK` — duplicate.** A repeat with the same idempotency key returns the
original outcome, including `notification_id` once processing has completed.

---

### Poll by correlation ID

```http
GET /notifications/correlation/{correlation_id}
```

The polling path: the caller receives a correlation ID at submission time,
before a notification row exists. Redis is consulted first, with Postgres as the
source of truth.

```json
{
  "success": true,
  "data": {
    "correlation_id": "b2a1f0c9-…",
    "status": "sent",
    "notification": { "notification_id": "0f7c2c1e-…", "…": "…" },
    "cached_status": { "status": "sent", "updated_at": 1787406502 }
  }
}
```

`cached_status` is surfaced separately so a caller can tell a cached transition
from the persisted record when the two have not converged yet.

---

### Fetch one notification

```http
GET /notifications/{id}
```

```json
{
  "success": true,
  "data": {
    "notification_id": "0f7c2c1e-…",
    "correlation_id": "b2a1f0c9-…",
    "user_id": "9c8b7a65-…",
    "template_id": "3e2d1c0b-…",
    "channel": "email",
    "status": "sent",
    "priority": "normal",
    "recipient": "ada@example.com",
    "retry_count": 0,
    "max_retries": 3,
    "queued_at": "2026-08-22T09:14:08.685Z",
    "sent_at": "2026-08-22T09:14:08.729Z",
    "created_at": "2026-08-22T09:14:08.593Z"
  }
}
```

`enriched_payload` is deliberately omitted — it holds the recipient's contact
details and the fully rendered body.

**Status values:** `pending`, `enriching`, `queued`, `processing`, `sent`,
`failed`, `cancelled`. `cancelled` means the recipient has opted out of that
channel.

---

### List a user's notifications

```http
GET /notifications?user_id={uuid}&limit=20&cursor=2026-08-22T09:14:08.593Z
```

| Parameter | Default | Notes |
|---|---|---|
| `user_id` | — | Required, UUID |
| `limit` | 20 | Capped at 100 |
| `cursor` | — | RFC 3339 timestamp from `page.next_cursor` |

Cursor pagination keyed on `created_at` rather than an offset, so paging stays
correct while new notifications arrive.

```json
{
  "data": {
    "notifications": [ "…" ],
    "page": { "next_cursor": "2026-08-22T08:02:11.004Z", "has_more": true, "limit": 20 }
  }
}
```

---

### Audit timeline

```http
GET /notifications/{id}/events
```

```json
{
  "data": {
    "notification_id": "0f7c2c1e-…",
    "events": [
      { "event_type": "created",   "event_at": "2026-08-22T09:14:08.593Z" },
      { "event_type": "enriched",  "event_at": "2026-08-22T09:14:08.681Z" },
      { "event_type": "queued",    "event_at": "2026-08-22T09:14:08.688Z" },
      { "event_type": "delivered", "event_at": "2026-08-22T09:14:08.733Z" }
    ]
  }
}
```

Where a `status` answers *where is it now*, the timeline answers *what happened
and when*.

---

### Retry a failed notification

```http
POST /notifications/{id}/retry
```

Republishes the message stored at enrichment time, verbatim. Re-deriving it
would silently pick up a template or preference that changed since, which is not
what a retry should mean.

- **`202`** — requeued
- **`409`** — not retryable: the status is not `failed`, retries are exhausted, or no stored payload exists
- **`404`** — no such notification

---

### Delivery status callback

```http
POST /notifications/status
Authorization: Bearer <service token>
```

```json
{
  "notification_id": "0f7c2c1e-…",
  "status": "delivered",
  "provider": "smtp",
  "provider_message_id": "…",
  "error": ""
}
```

Called by channel workers, not clients. Accepts `sent`, `delivered`, `failed`
and `bounced`. Requires a token carrying `role: "service"` — a user token, an
admin's included, is rejected.

---

## Users

Base path `/user`, service port `:3007`.

| Method | Route | Auth | Purpose |
|---|---|---|---|
| `POST` | `/user/signup` | Public | Register. `role` is not accepted |
| `POST` | `/user/signin` | Public | Returns a JWT |
| `GET` | `/user` | Admin | Paginated list |
| `GET` | `/user/preference` | Admin | Paginated preferences |
| `GET` | `/user/:id` | JWT | Single user |
| `GET` | `/user/preference/:id` | JWT | Notification preferences |
| `GET` | `/user/:id/delivery-profile` | JWT | Contact details, tokens and consent in one call |
| `PATCH` | `/user/:id/preference` | JWT (self) | Update consent, language, daily limit |
| `PATCH` | `/user/:id/device-tokens` | JWT (self) | Replace the FCM token set |
| `PATCH` | `/user/:id/role` | Admin | Grant or revoke admin |
| `GET` | `/user-service/health` | Public | Liveness |

### Signup

```json
{ "name": "Ada Lovelace", "email": "ada@example.com", "password": "at-least-6-chars" }
```

Returns the created user without the password hash. `role` is always `user`;
see [Operations](./OPERATIONS.md#creating-the-first-admin) for granting admin.

### Device tokens

```json
{
  "device_tokens": [
    { "token": "fcm-registration-token", "platform": "android" },
    { "token": "another-token",          "platform": "ios" }
  ]
}
```

Replaces the whole set — FCM registration tokens rotate, so the client owns the
list. Duplicates are removed on write.

### Delivery profile

```json
{
  "user_id": "9c8b7a65-…",
  "name": "Ada Lovelace",
  "email": "ada@example.com",
  "device_tokens": [ { "token": "…", "platform": "android" } ],
  "email_opt_in": true,
  "push_opt_in": true,
  "daily_limit": 100,
  "language": "en"
}
```

---

## Templates

Base path `/template`, service port `:3003`.

| Method | Route | Auth | Purpose |
|---|---|---|---|
| `POST` | `/template` | Admin | Create |
| `GET` | `/template` | JWT | Paginated; filter by `name`, `language`, `event`, `channel` |
| `GET` | `/template/:id` | JWT | One template; `?history=true` returns every version |
| `POST` | `/template/:id/render` | JWT | Compile against a supplied context |
| `GET` | `/template/event/:event/channel/:channel` | JWT | Look up by event |
| `PATCH` | `/template/:id` | JWT | Creates a new version |
| `DELETE` | `/template/:id` | JWT | Remove |
| `GET` | `/template-service/health` | Public | Liveness |

### Create

```json
{
  "name": "Order Confirmation",
  "event": "ORDER_CONFIRMED",
  "channel": ["EMAIL", "PUSH"],
  "language": "en",
  "subject": "Your order {{order.id}} is confirmed",
  "title": "Order confirmed",
  "body": "Hi {{user.name}}, order {{order.id}} is on its way."
}
```

`subject` applies to email, `title` to push, and `body` to both. Unique on
`(event, language)`.

### Render

```json
{ "data": { "user": { "name": "Ada" }, "order": { "id": "4821" } } }
```

Returns one message per declared channel:

```json
{
  "data": [
    { "channel": "EMAIL", "subject": "Your order 4821 is confirmed",
      "html": "Hi Ada, order 4821 is on its way.",
      "metadata": { "templateId": "…", "templateVersion": 3 } },
    { "channel": "PUSH", "title": "Order confirmed",
      "body": "Hi Ada, order 4821 is on its way.",
      "metadata": { "templateId": "…", "templateVersion": 3 } }
  ]
}
```

The caller supplies everything the template references; this endpoint performs no
lookups.

---

## Email service

Direct submission, bypassing the orchestrator. Service port `:8003`.

| Method | Route | Purpose |
|---|---|---|
| `POST` | `/api/v1/notifications/` | Submit directly to the worker |
| `GET` | `/api/v1/notifications/{request_id}/` | Delivery status |
| `GET` | `/api/v1/notifications/list/` | Recent deliveries |
| `GET` | `/health/` | Liveness |

This path has no orchestrator record behind it, so it renders from a fetched
template rather than pre-rendered content and reports no status. Use
`POST /notifications` for anything that should be tracked.

---

## Push service

Operational only. Service port `:8081`.

| Method | Route | Purpose |
|---|---|---|
| `GET` | `/health` · `/ready` | Liveness · readiness |
| `GET` | `/status/{notification_id}` | Per-token delivery results |

---

## Health and metrics

| Endpoint | Purpose |
|---|---|
| `GET :8000/health` | Gateway; public and exempt from rate limiting |
| `GET :3002/health` | Orchestrator, including database and Redis checks |
| `GET :3002/metrics` | Prometheus scrape target |
| `GET :3007/user-service/health` | User service |
| `GET :3003/template-service/health` | Template service |
| `GET :8081/health` · `/ready` | Push service |
| `GET :8003/health/` | Email service |
