# Development

Working on the codebase: local setup, testing, conventions, and how to extend it.

Related: [Architecture](./ARCHITECTURE.md) · [API reference](./API.md) ·
[Operations](./OPERATIONS.md)

---

## Prerequisites

- Docker and Docker Compose
- Node.js 20+ and pnpm 10+
- Go 1.25+
- Python 3.11+

The repository is a polyglot monorepo: pnpm workspaces with Nx for TypeScript, a
Go workspace (`go.work`) for the Go services, and a standalone Django project.

---

## Layout

```
api-gateway/              NestJS edge service
services/
  orchestrator/           Go — enrichment, outbox, status API
    cmd/                  Entrypoint
    internal/
      handlers/           HTTP handlers
      services/           Business logic
      repositories/       Data access
      models/ dtos/       Types
      database/           Migrations, partitions
      auth/               Service tokens
      metrics/            Prometheus collectors
      tracing/            OpenTelemetry setup and AMQP propagation
  user-service/           NestJS + Prisma
  template-service/       NestJS + Prisma
  push-service/           Go — FCM consumer
  email-service/          Django + Celery
packages/common/          Shared TypeScript utilities
infra/                    Compose stack, Prometheus, Grafana, Jaeger
scripts/                  Database bootstrap, smoke test
docs/                     This documentation
```

---

## Running

### Everything

```bash
docker compose -f infra/docker-compose.local.yaml up -d --build
```

Source directories are bind-mounted, so the NestJS services and Go services
hot-reload on save.

### One service against the rest

```bash
docker compose -f infra/docker-compose.local.yaml up -d          # the stack
docker compose -f infra/docker-compose.local.yaml stop orchestrator

cd services/orchestrator && go run cmd/orchestrator/main.go
```

Point its `.env` at `localhost` and the published ports.

---

## Testing

```bash
# Go
cd services/orchestrator && go test -race ./...
cd services/push-service && go test -race ./...

# TypeScript
cd api-gateway && pnpm test
cd services/user-service && pnpm test
cd services/template-service && pnpm test

# Python
cd services/email-service && python manage.py test notifications

# End to end, against a running stack
./scripts/smoke-test.sh
```

### What is worth testing here

The failures this system is prone to live **between** components rather than
inside them: a field renamed on one side of a queue, a queue argument that must
match in three places, a client that resolves differently in a container than on
a laptop. Unit tests catch none of that.

That is why CI builds every image and runs a real notification through the whole
stack. When adding behaviour that crosses a service boundary, extend
`scripts/smoke-test.sh` alongside the unit tests.

Prefer table-driven tests in Go and explicit provider mocks in NestJS specs —
`{ provide: PrismaService, useValue: … }` rather than a partially real module.

---

## CI

`.github/workflows/ci.yml` runs on every push and pull request:

| Job | Covers |
|---|---|
| `go` | `gofmt`, `go vet`, `go build`, `go test -race` for both services |
| `typescript` | `tsc --noEmit`, `nest build`, `jest` for all three services |
| `python` | Django system check and tests |
| `end-to-end` | Builds every image, starts the stack, seeds an admin, runs the smoke test |

Typechecking targets `tsconfig.json`, not `tsconfig.build.json`. The two can
diverge, and `nest build` will happily pass over a failure the base config
catches.

---

## Conventions

**Go.** Standard layout under `internal/`. Errors wrap with `%w`. Sentinel
errors (`ErrNotFound`, `ErrNotRetryable`) let callers use `errors.Is` rather than
matching on strings. `gofmt` and `go vet` are enforced in CI.

**TypeScript.** NestJS modules per domain. DTOs validated with `class-validator`.
Guards on handlers, not paths. Prisma clients are generated per service into
`node_modules/@notification/{service}-prisma`.

**Python.** Django app layout. Celery tasks in `tasks.py`, the queue bridge as a
management command.

**Comments** explain *why*, not *what*. A comment earns its place when it records
a constraint or a decision that the code alone cannot convey.

---

## Common changes

### Adding a notification channel

The topology is designed for this: routing keys are `notification.{channel}`, so
nothing existing needs editing.

1. Add the channel to `dtos.NotificationType` in the orchestrator.
2. Declare a queue and binding in `SetupRabbitMQ`, carrying the same
   `x-dead-letter-exchange` argument as the others.
3. Handle it in `channelAllowed` and `resolveRecipient` — consent and how to
   address a recipient on that channel.
4. Add the channel to the template service's `NotificationChannel` enum and emit
   its fields in `render`.
5. Write the consumer, reading
   [the message contract](./contracts/enriched-notification.md). Everything it
   needs already arrives resolved and rendered.
6. Extend the smoke test.

### Changing the message contract

The producer and consumers are in three languages with no shared type, so
nothing catches drift at compile time — a renamed field simply arrives as a zero
value and the worker does the wrong thing quietly.

Change [the contract document](./contracts/enriched-notification.md) first, then
the producer, then every consumer, in one change. Adding an optional field is
safe; renaming or removing one is not.

### Database migrations

**Orchestrator** — add a numbered SQL file under
`internal/database/migrations/`. Applied with tern on boot.

**NestJS services** — `pnpm prisma migrate dev --name <change>`, then regenerate
the client. Each service owns only its own models; the two schemas are
independent.

### Adding a span

Use the service's tracer and record the identifiers that make a span findable:

```go
ctx, span := tracing.Tracer().Start(ctx, "notification.something")
defer span.End()
tracing.CorrelationID(span, correlationStr)
```

Two boundaries need explicit handling — instrumentation libraries cover neither:

**Crossing the queue.** Inject on publish with `tracing.InjectAMQP(ctx, headers)`
and extract on consume with `tracing.ExtractAMQP(ctx, delivery.Headers)`.
Producer and consumer share no connection, so without this a trace ends at
publish and an unrelated one begins at consume.

**Detaching from a request.** Work that outlives its request cannot use the
request context — that is cancelled when the response is written. Carry the span
context onto a background one instead:

```go
ctx := trace.ContextWithSpanContext(
    context.Background(),
    trace.SpanContextFromContext(r.Context()),
)
```

### Adding a metric

Declare it in `internal/metrics/metrics.go` and increment it at the point the
event happens. Prefer a label that answers *which one* — the `stage` label on
enrichment failures is what turns "something broke" into "the template fetch
broke".

---

## Troubleshooting the setup

| Symptom | Fix |
|---|---|
| `Cannot find module '@notification/…-prisma'` | `pnpm prisma generate` in that service |
| Jest cannot resolve `src/…` | The base `tsconfig.json` needs `baseUrl` and `paths`, and Jest needs a matching `moduleNameMapper` |
| A container starts, then exits | Configuration validation. The log names the variable |
| Ports already in use | `POSTGRES_HOST_PORT` and friends are configurable in `infra/.env` |
| A rebuild does not pick up changes | Source is bind-mounted, but dependency changes need `--build` |
