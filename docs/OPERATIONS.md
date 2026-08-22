# Operations

Running, configuring and troubleshooting the system.

Related: [Architecture](./ARCHITECTURE.md) · [API reference](./API.md) ·
[Development](./DEVELOPMENT.md)

---

## Configuration

### Shared secrets

| Variable | Used by | Notes |
|---|---|---|
| `JWT_SECRET` | every service | Signs user tokens **and** the short-lived service tokens the orchestrator and email worker mint. Must be identical everywhere |
| `DB_PASSWORD` | Postgres and all consumers | |
| `RABBITMQ_USER` / `RABBITMQ_PASSWORD` | broker and all clients | |
| `REDIS_PASSWORD` | Redis and all clients | Optional locally; set it anywhere else |
| `DJANGO_SECRET_KEY` | email service | Required whenever `DEBUG` is off |

### Per-service

**Orchestrator** — all configuration is `ORCHESTRATOR_`-prefixed, loaded into a
typed struct and validated at startup, so a missing or malformed value fails at
boot with a clear message rather than as a nil dereference under load.

| Variable | Purpose |
|---|---|
| `ORCHESTRATOR_DATABASE.*` | Host, port, credentials, pool sizing |
| `ORCHESTRATOR_REDIS.*` | Address, password, database index |
| `ORCHESTRATOR_RABBITMQ.URL` | Broker connection |
| `ORCHESTRATOR_RABBITMQ.EXCHANGE_NAME` | `notifications` |
| `ORCHESTRATOR_RABBITMQ.QUEUE_NAME` / `.ROUTING_KEY` | The queue this instance declares |
| `ORCHESTRATOR_SERVER.*` | Port and timeouts, e.g. `15s` |
| `ORCHESTRATOR_EXTERNAL.USER_SERVICE_ADDRESS` | Enrichment target |
| `ORCHESTRATOR_EXTERNAL.TEMPLATE_SERVICE_ADDRESS` | Enrichment target |
| `ORCHESTRATOR_EXTERNAL.JWT_SECRET` | Signs service tokens |

**Gateway** — `PORT`, `JWT_SECRET`, `REDIS_URL`, the three upstream URLs,
`CORS_ORIGIN`, and `THROTTLE_TTL` / `THROTTLE_LIMIT` (default 60 seconds / 100
requests).

**Push service** — `FCM_ENABLED` is **off by default**. Enabling it without a
service-account file is a startup failure, which is deliberate: silently running
with push disabled would be worse. To send real notifications, mount the
credentials and set `FCM_ENABLED=true`, `FCM_PROJECT_ID` and
`FCM_SERVICE_ACCOUNT_PATH`.

**Email service** — `DEBUG` and `ALLOWED_HOSTS` must be set explicitly outside
development; the service refuses to start with `DEBUG` off and no `SECRET_KEY`.
`DB_SSL_REQUIRE` is off for a local container and on for managed providers.

---

## Bringing the stack up

```bash
cp infra/.env.example infra/.env      # then fill in the secrets above
docker compose -f infra/docker-compose.local.yaml up -d --build
```

Starts Postgres (with the four databases from `scripts/init-databases.sql`),
Redis, RabbitMQ, the five services, MailHog, Prometheus, Grafana and pgAdmin.

Migrations run on boot: the orchestrator applies its own with tern, and the
NestJS services run `prisma migrate deploy`.

### Creating the first admin

Templates require an admin, and `role` is not settable at signup. Bootstrap one:

```bash
docker compose -f infra/docker-compose.local.yaml exec \
  -e ADMIN_EMAIL=admin@example.com -e ADMIN_PASSWORD='choose-a-strong-password' \
  user-service sh -c 'cd /usr/src/app/services/user-service && pnpm seed:admin'
```

Idempotent — re-running promotes an existing account rather than duplicating it.

### Verifying

```bash
./scripts/smoke-test.sh
```

Signs up a user, creates a template, submits a notification, asserts the email
arrived, and re-submits to confirm deduplication. This is what CI runs.

---

## Consoles

| Service | URL | Credentials |
|---|---|---|
| Grafana | http://localhost:3001 | `GRAFANA_USER` / `GRAFANA_PASSWORD` (default admin/admin) |
| Prometheus | http://localhost:9090 | — |
| RabbitMQ | http://localhost:15672 | `RABBITMQ_USER` / `RABBITMQ_PASSWORD` |
| MailHog | http://localhost:8025 | — |
| pgAdmin | http://localhost:5050 | `PGADMIN_DEFAULT_EMAIL` / `PGADMIN_DEFAULT_PASSWORD` |

---

## Monitoring

The orchestrator exposes Prometheus metrics on `/metrics`.

| Metric | Type | Reading it |
|---|---|---|
| `notification_received_total{channel}` | counter | Ingress rate |
| `notification_enrichment_total{channel,result,stage}` | counter | `stage` names the downstream call that failed |
| `notification_enrichment_duration_seconds` | histogram | Dominated by downstream HTTP |
| `notification_publish_total{channel,result}` | counter | Publish outcomes |
| `notification_publish_duration_seconds` | histogram | Includes broker confirmation |
| `notification_outbox_entries{status}` | gauge | **The leading indicator** |
| `notification_status_transitions_total{status}` | counter | Lifecycle movement |
| `notification_retries_total{origin}` | counter | `api` or `recovery` |
| `notification_recovered_total` | counter | Should be flat at zero |
| `notification_dependency_up{dependency}` | gauge | 1 healthy, 0 unreachable |
| `notification_duplicates_suppressed_total` | counter | Idempotency hits |

Per-queue depth comes from RabbitMQ's `/metrics/detailed` endpoint as
`rabbitmq_detailed_queue_messages`. The default `/metrics` aggregates across all
queues and drops the queue label, which makes it useless for alerting on a
specific queue or on the DLQ.

---

## Alerts

Seven rules in `infra/observability/alerts.yml`.

### `OutboxBacklogGrowing` — warning

More than 100 pending entries for 5 minutes. Messages are being accepted but not
published.

1. Is RabbitMQ up and reachable? `notification_dependency_up` and the broker's
   own health.
2. Check orchestrator logs for `Failed to publish outbox entry` — the recorded
   `last_error` on the rows says why.
3. The backlog drains on its own once publishing recovers; entries are retried
   with exponential backoff.

### `OutboxEntriesFailed` — critical

Entries have exhausted their retries and will not be published without
intervention.

```sql
SELECT id, notification_id, attempts, last_error, created_at
FROM notification_outbox WHERE status = 'failed' ORDER BY created_at DESC;
```

Fix the cause, then requeue with `POST /notifications/{id}/retry`, or reset the
row directly:

```sql
UPDATE notification_outbox
SET status = 'pending', attempts = 0, available_at = NOW()
WHERE id = '…';
```

### `DeadLetterQueueGrowing` — warning

Messages exhausted their retries at the worker. Inspect before replaying —
something made them undeliverable, and replaying blindly repeats it. Use the
RabbitMQ console to view `notifications.dlq`.

### `ChannelQueueBacklog` — warning

More than 500 unconsumed messages for 10 minutes. Either workers are down or
arrival outpaces them. Check consumer counts in the RabbitMQ console, then scale
the worker.

### `EnrichmentFailureRate` — warning

Over 10% of enrichments failing for 5 minutes. The `stage` label identifies the
failing call: `user_fetch`, `template_fetch`, `render`, `recipient_resolution`
or `outbox_enqueue`.

### `DependencyDown` — critical

The orchestrator cannot reach Postgres or Redis. Ingress fails outright while
this holds.

### `NotificationsBeingRecovered` — warning

The sweeper is finding abandoned notifications, which means processes are dying
mid-enrichment. Look for restarts, OOM kills, or panics in the logs.

---

## Common tasks

### Inspecting a notification's history

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8000/notifications/$ID/events"
```

Or from the correlation ID the caller holds:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8000/notifications/correlation/$CORRELATION_ID"
```

### Replaying dead-lettered messages

There is no automated replay. Inspect the message in the RabbitMQ console,
resolve the cause, then either retry through the API — which republishes the
stored message — or shovel it back onto its channel queue.

### Adding partitions manually

The maintainer runs at boot and every 12 hours. To extend the window by hand:

```sql
SELECT * FROM create_notification_partitions(6);
```

Safe to run repeatedly; it creates only what is missing.

### Rotating `JWT_SECRET`

Rotating invalidates every issued user token, and service tokens live 5 minutes.
Deploy the new secret to all services together — a partial rollout leaves
services minting tokens their peers cannot verify.

### Changing queue arguments

RabbitMQ rejects a redeclare with different arguments, and three services declare
these queues. Drain and delete the affected queues before deploying, then let the
services recreate them. A rolling restart alone fails with `PRECONDITION_FAILED`.

---

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Notifications stay `queued` | Workers are down, or the status callback is failing. Check `notifications.dlq` depth and worker logs |
| `401` on the status callback | The caller's token is not `role: "service"`, or `JWT_SECRET` differs between services |
| Everything returns `429` | Rate limit exhausted. Raise `THROTTLE_LIMIT` or wait out the window |
| A service will not start | Configuration validation. The log names the missing or malformed variable |
| `PRECONDITION_FAILED` on boot | Queue arguments changed; see above |
| Push service exits at start | `FCM_ENABLED=true` without credentials |
| Email is accepted but never arrives | Check the bridge is running — it is a separate process from the worker, and without it the queue simply fills |

### Reading the logs

Structured JSON with a `correlation_id` on every line, so one request can be
followed across services:

```bash
docker compose -f infra/docker-compose.local.yaml logs orchestrator \
  | grep "$CORRELATION_ID"
```

---

## Backup and retention

`notifications` is partitioned monthly, so retention is a `DROP TABLE` rather
than a mass `DELETE`:

```sql
DROP TABLE notifications_2026_01;
```

`notification_events` is not partitioned and grows without bound; it needs a
retention policy before it becomes large.

Prometheus is configured for 7 days on a local volume — appropriate for
development, and the first setting to revisit for anything longer-lived.
