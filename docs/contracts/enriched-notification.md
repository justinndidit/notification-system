# Contract: enriched notification

The message the orchestrator publishes to RabbitMQ, and the only thing a channel worker needs in order to deliver.

**Producer:** orchestrator — `dtos.EnrichedNotification` (`internal/dtos/dtos.go`)
**Consumers:** push service — `internal.PushNotificationMessage` (`internal/models.go`); email service — the `payload` argument to `send_email_task`
**Exchange:** `notifications` (topic) · **Routing key:** `notification.{channel}`

---

## Principle

**The message is self-contained.** A worker never calls back into the system to find out who the recipient is or what the message says. The orchestrator resolves both during enrichment, so adding a channel means writing a sender, not re-implementing recipient lookup and template rendering.

Two consequences worth stating plainly:

- `recipient` and `tokens` are already resolved. Workers do not query the user service.
- `subject`, `title` and `body` are already rendered. Workers do not compile templates.

---

## Schema

| Field | Type | Always present | Notes |
|---|---|---|---|
| `notification_id` | UUID string | yes | Primary key of the `notifications` row |
| `correlation_id` | UUID string | yes | Threads one request across every service and log line |
| `idempotency_key` | string | yes | Caller-supplied; workers may use it to deduplicate |
| `user_id` | UUID string | yes | Recipient's ID in the user service |
| `template_code` | UUID string | yes | Template ID that produced the content |
| `channel` | `email` \| `push` | yes | Matches the routing key suffix |
| `priority` | `low` \| `normal` \| `high` \| `urgent` | yes | |
| `recipient` | string | yes | Email address for `email`; first device token for `push` (informational — `tokens` carries delivery) |
| `tokens` | `DeviceToken[]` | push only | Empty for email |
| `subject` | string | email only | Rendered |
| `title` | string | push only | Rendered |
| `body` | string | yes | Rendered. HTML for email, plain text for push |
| `user_preferences` | object | yes | Consent snapshot at enrichment time |
| `template` | object | yes | Template metadata, for auditing |
| `variables` | object | yes | The raw variables, retained for debugging |
| `metadata` | object | no | Caller-supplied passthrough |
| `created_at` | RFC 3339 | yes | When the message was enriched |

### DeviceToken

| Field | Type | Notes |
|---|---|---|
| `token` | string | FCM registration token |
| `platform` | `android` \| `ios` | iOS is accepted but not yet delivered |

---

## Example — email

```json
{
  "notification_id": "0f7c2c1e-6d1a-4a6f-9a3a-2b1f4c0d9e88",
  "correlation_id": "b2a1f0c9-1234-4d5e-8f90-1a2b3c4d5e6f",
  "idempotency_key": "req-123",
  "user_id": "9c8b7a65-4321-4f0e-9d8c-7b6a5f4e3d2c",
  "template_code": "3e2d1c0b-9a87-4655-b4a3-2c1d0e9f8a7b",
  "channel": "email",
  "priority": "normal",
  "recipient": "ada@example.com",
  "tokens": [],
  "subject": "Welcome, Ada",
  "body": "<p>Hello Ada, your account is ready.</p>",
  "user_preferences": {
    "user_id": "9c8b7a65-4321-4f0e-9d8c-7b6a5f4e3d2c",
    "email_opt_in": true,
    "push_opt_in": true,
    "daily_limit": 100,
    "language": "en"
  },
  "variables": { "name": "Ada", "link": "https://example.com/start" },
  "metadata": {},
  "created_at": "2026-08-20T12:34:56Z"
}
```

## Example — push

```json
{
  "notification_id": "1a2b3c4d-5e6f-4708-9a0b-1c2d3e4f5a6b",
  "correlation_id": "b2a1f0c9-1234-4d5e-8f90-1a2b3c4d5e6f",
  "idempotency_key": "req-124",
  "user_id": "9c8b7a65-4321-4f0e-9d8c-7b6a5f4e3d2c",
  "template_code": "3e2d1c0b-9a87-4655-b4a3-2c1d0e9f8a7b",
  "channel": "push",
  "priority": "high",
  "recipient": "fcm-token-abc123",
  "tokens": [
    { "token": "fcm-token-abc123", "platform": "android" },
    { "token": "fcm-token-def456", "platform": "ios" }
  ],
  "title": "Your order shipped",
  "body": "Order #4821 is on its way.",
  "user_preferences": { "user_id": "9c8b…", "email_opt_in": true, "push_opt_in": true, "daily_limit": 100, "language": "en" },
  "variables": { "name": "Ada", "link": "https://example.com/orders/4821" },
  "metadata": {},
  "created_at": "2026-08-20T12:35:10Z"
}
```

---

## Message properties

Published with `delivery_mode=2` (persistent), plus:

| Property | Value |
|---|---|
| `message_id` | `notification_id` |
| `correlation_id` | `correlation_id` |
| `content_type` | `application/json` |
| `timestamp` | publish time |
| `headers.channel` | `email` \| `push` |
| `headers.priority` | priority string |

This is what lets a message in the RabbitMQ management UI be traced back to its database row without decoding the body.

---

## Changing this contract

The producer and both consumers are in different languages, so nothing catches a mismatch at compile time — a drifted field simply arrives as a zero value and the worker silently does the wrong thing. That has already happened once: the push worker expected `tokens`, the orchestrator never sent them, and every push notification was acknowledged and dropped while the logs read as success.

So: change this document first, then the producer, then every consumer. Adding an optional field is safe. Renaming or removing one is not, and needs the consumers updated in the same change.

Known gaps, tracked in `PROJECT_CONTEXT.md`:

- The email worker takes a differently-shaped `payload` (`request_id`, `variables.email`). The queue bridge is responsible for translating this message into that shape until the task is updated to consume the contract directly.
- `template` and `variables` are carried for auditing and debugging. Workers should not depend on them for delivery — `recipient`, `subject`/`title`, and `body` are the delivery surface.
