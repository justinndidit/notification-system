# Push Service

Delivers push notifications through Firebase Cloud Messaging.

Part of the [Notification System](../../README.md).

| | |
|---|---|
| **Stack** | Go 1.25, amqp091-go, go-redis, oauth2, zerolog |
| **Port** | 8080 (published on 8081) |
| **Owns** | Redis delivery-status records. No relational database |
| **Consumes** | `push_notifications`, bound to `notification.push` |

---

## What it does

Consumes with manual ack and a configurable prefetch, splits tokens by platform,
and sends per token through the FCM HTTP v1 API using OAuth2 service-account
credentials. Results are aggregated into a `DeliveryStatus` and written to Redis
with a seven-day TTL.

Messages arrive already complete — resolved device tokens, rendered title and
body — so this service never looks anything up or compiles a template. See
[the message contract](../../docs/contracts/enriched-notification.md).

| Method | Route | |
|---|---|---|
| `GET` | `/health` · `/ready` | Liveness · readiness |
| `GET` | `/status/{notification_id}` | Per-token delivery results |

---

## Failure handling

A message that fails is retried up to five delivery attempts and then
dead-lettered to `notifications.dlx`, so a message that can never succeed is set
aside for inspection rather than occupying a consumer indefinitely.

Attempt counting reads `x-delivery-count` where the broker provides it (quorum
queues) and falls back to the `redelivered` flag on classic queues.

---

## Running it

```bash
docker compose -f ../../infra/docker-compose.local.yaml up push-service

# or directly
go run cmd/push-service/main.go
```

| Variable | Purpose |
|---|---|
| `PORT` | Listen port (8080) |
| `RABBITMQ_URL`, `RABBITMQ_QUEUE`, `RABBITMQ_EXCHANGE` | Broker |
| `FCM_ENABLED` | **Off by default.** Enabling it without credentials is a startup failure |
| `FCM_PROJECT_ID`, `FCM_SERVICE_ACCOUNT_PATH` | Required when FCM is enabled |
| `REDIS_URL`, `REDIS_PASSWORD` | Delivery-status store |

To send real notifications, mount a service-account JSON, set `FCM_PROJECT_ID`
and `FCM_SERVICE_ACCOUNT_PATH`, and set `FCM_ENABLED=true`.

---

## Tests

```bash
go test -race ./...
```

Covers delivery-attempt counting across both queue types, which is what bounds
the retry loop, and per-token success and failure aggregation.

APNS is not implemented; iOS tokens produce a placeholder result.
