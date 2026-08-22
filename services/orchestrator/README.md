# Orchestrator

The workflow engine. Turns a bare notification request into a self-contained,
deliverable message and gets it onto the queue durably.

Part of the [Notification System](../../README.md).

| | |
|---|---|
| **Stack** | Go 1.25, chi, pgx/v5, tern, koanf, zerolog, amqp |
| **Port** | 8080 (published on 3002) |
| **Owns** | `notification_db` — `notifications`, `notification_events`, `notification_outbox` |
| **Talks to** | User Service, Template Service, Postgres, Redis, RabbitMQ |

---

## The path a notification takes

1. **Claim** the idempotency key with `SETNX`. A duplicate short-circuits here.
2. **Persist** the notification as `pending` — the durable unit of work.
3. **Enrich** concurrently: the recipient's contact details and consent from the
   User Service, the template from the Template Service.
4. **Check consent.** An opt-out ends as `cancelled`, which is a settled
   outcome, not a failure.
5. **Render** through the Template Service, so workers receive finished content.
6. **Commit** the queued state *and* an outbox row in one transaction.
7. **Publish** from the outbox, separately, with broker confirmation.

---

## Why the outbox exists

Enrichment used to publish to RabbitMQ from a detached goroutine and then update
the row as a separate step. A crash between the two left them inconsistent; a
crash before them lost the work entirely, because nothing durable was holding it.

Now the state change and the intent to publish commit together or not at all. A
separate publisher drains the table with `FOR UPDATE SKIP LOCKED`, so several
replicas can share the work without coordinating. Backoff pushes `available_at`
forward rather than sleeping, so one stuck row never blocks the queue behind it.

A **recovery sweeper** covers the other window — a process that dies *during*
enrichment, before the outbox commit. It re-enriches anything stuck in `pending`
or `enriching` past five minutes. Re-enrichment is safe: the outbox is uniquely
keyed on `notification_id`, so a notification enriched twice still produces one
message.

Verified by stopping RabbitMQ mid-flight: the notification was accepted, held in
the outbox with the broker error recorded, and delivered on its own once the
broker returned.

---

## API

| Method | Route | Notes |
|---|---|---|
| `POST` | `/notifications` | Requires `X-Idempotency-Key` |
| `GET` | `/notifications?user_id=&limit=&cursor=` | Cursor-paginated |
| `GET` | `/notifications/{id}` | Lifecycle timestamps |
| `GET` | `/notifications/correlation/{id}` | Polling path; Redis-cached |
| `GET` | `/notifications/{id}/events` | Audit timeline |
| `POST` | `/notifications/{id}/retry` | Republishes the stored message verbatim |
| `POST` | `/notifications/status` | Worker callback; service token only |
| `GET` | `/health` · `/metrics` | Liveness · Prometheus |

---

## Running it

```bash
docker compose -f ../../infra/docker-compose.local.yaml up orchestrator

# or directly
go run cmd/orchestrator/main.go
```

Configuration is `ORCHESTRATOR_`-prefixed and loaded into a typed struct, then
validated at startup — a missing or malformed value fails at boot with a clear
message rather than as a nil dereference under load. See `.env.example`.

Migrations run automatically on boot, as does partition creation: the
`notifications` table is range-partitioned by month, and a background job keeps
a window open from one month back to three ahead.

---

## Tests

```bash
go test -race ./...
```

Covers the enrichment decisions (consent, recipient resolution, channel
matching), the JSONB scanners that once broke every read, outbox backoff bounds,
and the callback status mapping.
