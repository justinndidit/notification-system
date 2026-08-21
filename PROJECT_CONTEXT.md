# Project Context — Notification System

> Working context document. Written from a full read of the codebase on 2026-08-20 (branch `chore/readme`, HEAD `e00b765`).
> This describes **what is actually in the code**, not what the README claims. Where the two disagree, this file wins.
>
> **Phase 0 is complete** (committed `9a099c2`). **Phase 1 is largely complete** and uncommitted.
> Items marked ✅ below are fixed. Everything else stands as written.

---

## 1. What this repo is

A polyglot microservices monorepo for a multi-channel (email/push) notification platform.

| Path | Language | Role | Port (container) |
|---|---|---|---|
| `api-gateway/` | NestJS 11 | Edge: JWT, proxying, (intended) rate limiting | 8000 |
| `services/orchestrator/` | Go 1.25 + chi | Ingests requests, enriches, persists, publishes to MQ | 8080 |
| `services/user-service/` | NestJS + Prisma | Users, auth, notification preferences | 3007 |
| `services/template-service/` | NestJS + Prisma | Template CRUD + Handlebars rendering | 3003 |
| `services/push-service/` | Go | RabbitMQ consumer → FCM | 8080 |
| `services/email-service/` | Django 5 + Celery | Email delivery worker | 8000 |
| `packages/common/` | TypeScript | 2 files (`logger.ts`, `validation.ts`) — essentially empty | — |
| `infra/` | Docker Compose | Postgres 16, Redis 7, RabbitMQ 3.13, pgAdmin | — |

Tooling: pnpm workspaces + Nx (TS), Go workspace (`go.work`), Taskfile (mostly commented out).

### Build/run status (verified, not assumed)

Originally audited, then re-verified after Phase 0:

| Check | Before Phase 0 | After Phase 0 |
|---|---|---|
| `go build` + `go vet` + `gofmt` (orchestrator) | ✅ passes | ✅ passes |
| `go build` + `go vet` + `gofmt` (push-service) | ✅ passes | ✅ passes |
| `nest build` (api-gateway, template, user) | ✅ passes | ✅ passes |
| `tsc --noEmit -p tsconfig.json` (template) | ❌ 5 errors | ✅ passes |
| `tsc --noEmit -p tsconfig.json` (user) | ❌ 3 errors | ✅ passes |
| `npx jest` (template-service) | ❌ 2 of 4 suites fail | ✅ 4/4 pass |
| `npx jest` (user-service) | ❌ suites fail | ✅ 4/4 pass |
| `docker compose config` | ✅ valid | ✅ valid |

The `tsc`/jest failures were a **config split**: `tsconfig.build.json` defined `baseUrl` + `paths` for `src/*`, but the base `tsconfig.json` had `baseUrl` commented out and jest had no matching `moduleNameMapper`. So `nest build` worked while `npm test` and editor typechecking were red. Both now carry the mapping.

Note the suites pass but assert only that classes instantiate — see §6.

---

## 2. Corrections to the README

The README's status table is materially wrong in both directions. Actual state:

| Component | README says | Reality |
|---|---|---|
| Template Service | Skeleton, 10% | **~80%** — 475-line service, full CRUD, pagination, filtering, Handlebars rendering, versioning, Redis cache |
| User Service | Skeleton, 10% | **~75%** — signup/signin, bcrypt, JWT, preferences, push tokens, pagination, Redis cache |
| Push Service | Empty, 0% | **~65%** — ~870 lines: AMQP consumer, FCM v1 client, Redis status store, health server. iOS/APNS is a stub |
| Email Service | Ready, 100%, "production-ready" | **Standalone-complete, integrated 0%.** Not in docker-compose. Not connected to the orchestrator's queue. `DEBUG=True`, hardcoded `SECRET_KEY` |
| Orchestrator | Partial, 70% | Data layer ~90%, **API surface ~20%** — only `POST /notification` and `GET /health` exist |
| Integration tests | Missing, 0% | Correct. Also: **unit tests are 0%** — all 8 spec files are `it('should be defined')` stubs |
| API Gateway | Partial, 60% | Proxying works; **auth and throttling are effectively bypassed** (see §4) |

Also inaccurate in the README: "unit tests exist per service" (they don't), "Idempotency keys to reduce duplicate processing" (implemented incorrectly, see §5.3), `Authorization: Bearer <jwt>` on `POST /notifications` (that route is unauthenticated).

---

## 3. The headline problem: the pipeline is not connected

Every service works in isolation. **No end-to-end flow completes.** The breaks below are independent of one another; §3.1 and §3.2 are the two that remain after Phase 0.

### 3.1 Email is fully disconnected (three separate breaks) ✅ FIXED

**a) Queue name mismatch.** The orchestrator declares and binds `email_queue` → routing key `notification.email` (`internal/config/config.go:276`). Celery consumes `email.queue` (`email_service/celery.py`, `task_default_queue`). Different queues. `email_queue` accumulates messages forever with no consumer.

**b) Wire-protocol mismatch.** Even with matching names, the orchestrator publishes a raw JSON `EnrichedNotification` body. Celery expects its own message protocol (task name, `args`/`kwargs`, protocol headers). A raw JSON body will not dispatch as a Celery task. **This needs a bridge**: either a small AMQP consumer in the Django service that reads `email_queue` and calls `send_email_task.delay(...)`, or have the orchestrator publish Celery-protocol messages via `kombu`.

**c) Not deployed.** `services/email-service` has no `Dockerfile.dev` and appears in **neither** compose file. `docker compose up` starts the entire system *except* the one service the README calls production-ready.

Additional email-side breakage, independent of the above:
- `notifications/utils.py:14` hardcodes `http://template-service:8000/templates/{code}`. The template service listens on **3003** and its route is `GET /template/:id`, returning `{success, data}` — not `{template_content}`. Template fetch **always fails**, and the circuit breaker silently substitutes a hardcoded fallback string. Every email would send the fallback body.
- `report_status()` POSTs to `http://notification_service/api/notifications/status/` — a host that exists nowhere in this repo, and the orchestrator has no status-callback route. Every callback fails silently (`except: pass`).
- The DLQ publishes to exchange `notifications.direct` — a different exchange from the main `notifications` topic exchange. Isolated from the rest of the topology.

### 3.2 Push receives messages it cannot use ✅ FIXED

The orchestrator publishes `dtos.EnrichedNotification` (user_id, template, user_preferences, variables). The push service unmarshals into `PushNotificationMessage`, which requires `Tokens []DeviceToken`. **The orchestrator never populates device tokens** — it doesn't fetch them, and `EnrichedNotification` has no such field.

Result: `len(msg.Tokens) == 0` → `service.go` logs a warning, returns `nil`, and the message is **ACKed and silently dropped**. Push notifications fail in a way that looks like success in the logs.

The user service *does* store `push_token` (Prisma `User.push_token Json?`) and exposes `PATCH /user/:id/push-token` — so the data exists; the orchestrator just never reads it into the payload.

Also: the orchestrator never renders the template. It forwards the raw template + variables and expects the worker to render. The push service has no rendering logic at all — no `Title`/`Body` would ever be populated even with tokens present.

### 3.3 Duplicate push queue ✅ FIXED

The orchestrator binds **both** `push_queue` (hardcoded, `config.go:280`) and `${QUEUE_NAME}` — which compose sets to `push_notifications` — to the same routing key `notification.push`. On a topic exchange, both queues receive every message. The push service consumes only `push_notifications`; **`push_queue` grows unboundedly forever.**

### 3.4 The gateway cannot reach the orchestrator at all ✅ FIXED

`proxy.middleware.ts` sets `proxyReqPathResolver: (req) => req.originalUrl` — the **full** original path, including the mount prefix. Verified against Express 5: a request to `/notifications/notification` mounted at `app.use('/notifications', …)` yields `originalUrl = '/notifications/notification'`, and that whole string is forwarded.

The orchestrator's chi router only registers `POST /notification` and `GET /health`. So:

| Client calls | Gateway forwards | Orchestrator has | Result |
|---|---|---|---|
| `/notifications/notification` | `/notifications/notification` | `/notification` | **404** |
| `/notifications` | `/notifications` | `/notification` | **404** |

**There is no path through the gateway that reaches the orchestrator.** The notification ingress — the system's entire reason for existing — is only reachable by calling the orchestrator directly on port 3002.

This works for the other two routes purely by coincidence: the gateway mounts `/user` and the user service's controller prefix is `user`; same for `/template`. The `/notifications` mount is plural while the route is singular, so the coincidence breaks.

Fix: settle on one path. The cleanest is `POST /notifications` + `GET /notifications/{id}` in the orchestrator, matching the gateway mount and the README's documented API — which also sets up Phase 2's status routes.

### 3.5 The gateway's own health endpoint returns 401 ✅ FIXED

`JwtAuthGuard` is registered as a global `APP_GUARD`, and its public-route list is `['/user/signup', '/user/signin', '/signup', '/signin']`. `/health` matches none of them, so `GET /health` falls through to `super.canActivate()` and **401s without a token**.

The guard injects a `Reflector` but never calls it, and there is no `@Public()` decorator anywhere in the repo — so there's no escape hatch. The Compose healthcheck (`curl -f http://localhost:8000/health`) therefore always fails and the gateway container is permanently reported unhealthy.

### 3.6 Template enrichment has always returned 401 ✅ FIXED

Found while wiring the service token. `TemplateController` carried a class-level `@UseGuards(JwtAuthGuard)`, and the orchestrator's `TemplateClient` sent no credentials at all. Every enrichment call to `GET /template/{id}` was rejected with `401`, which `BaseHTTPClient` classifies as permanent (no retry) — so enrichment failed at the template step on every single request, before the queue was ever reached.

This was not visible in the original audit because the *user* preference endpoint had its guard commented out, so only the template half was broken.

Guards are now declared per route rather than on the class: mutating routes keep `JwtAuthGuard`, while the three read endpoints the orchestrator uses (`GET /template/:id`, `POST /template/:id/render`, `GET /template/event/...`) accept either a user JWT or the internal service token.

### 3.7 Compose healthchecks probe the wrong paths ✅ FIXED

The NestJS health routes are namespaced by controller prefix — `/user-service/health` and `/template-service/health` — but Compose probed bare `/health` on both. Both containers were therefore permanently reported unhealthy.

### 3.8 Template rendering could never succeed ✅ FIXED

Found while wiring the orchestrator to `POST /template/{id}/render`. The render
endpoint resolved its own recipients from **template-service's** `User` table —
a byte-identical copy of the user-service schema that nothing ever writes to.
With no `userId` it queried an empty table and threw `404 No eligible recipients
found`; with one it threw `404 User not found`. The endpoint could not return a
successful response under any input.

It also duplicated the consent logic the orchestrator now owns.

Rendering is now pure: template plus supplied context in, compiled content out.
`resolveTargetUsers`, `getEligibleChannelsForUser` and `mapRecipient` were
unreachable once recipient selection moved out, and were removed along with
`RenderTemplateDto.userId`.

### 3.9 The email service could not be containerised ✅ FIXED

Two things blocked the image build, both invisible until it was attempted:

- `requirements.txt` was UTF-16LE with a BOM and CRLF line endings. `pip install -r`
  cannot parse that.
- `settings.py` imports `dj_database_url`, which was not in `requirements.txt` at all.

### 3.10 Both Go images failed to build on an unpinned tool ✅ FIXED

Found on the first real `docker compose up --build`. Both Go `Dockerfile.dev`s ran
`go install github.com/air-verse/air@latest`. `air` has since released v1.67.4,
which requires Go >= 1.26, while the base image is `golang:1.25-alpine`:

```
go: github.com/air-verse/air@latest: github.com/air-verse/air@v1.67.4
    requires go >= 1.26.0 (running go 1.25.14; GOTOOLCHAIN=local)
```

Neither Go service could be containerised, with no change to the repository —
an unpinned `@latest` broke the build on someone else's release schedule. Now
pinned to `air@v1.61.7`.

Worth noting this was invisible to every static check: both services pass
`go build`, `go vet` and `gofmt`. Only actually building the image surfaced it.

### 3.11 Push service port mismatch ✅ FIXED

`PORT: "8080"` in compose, but the port mapping is `8081:8081` and the gateway is configured with `PUSH_SERVICE_URL: http://push-service:8081`. The container listens on 8080. The healthcheck (`curl localhost:8080`) passes internally, but nothing external can reach it.

### Actual current topology

```
Client → api-gateway :8000 → orchestrator :8080
                                   ├→ user-service :3007      ✅ works
                                   ├→ template-service :3003   ✅ works
                                   ├→ Postgres (garbage IDs, §5.1)
                                   └→ RabbitMQ "notifications" (topic)
                                        ├ notification.email → email_queue ──────✗ no consumer
                                        ├ notification.push  → push_queue ───────✗ no consumer (leak)
                                        └ notification.push  → push_notifications → push-service ✗ drops (no tokens)

email-service (Django+Celery)  ← not deployed, listens on unrelated queue "email.queue"
```

---

## 4. Security issues

Ordered by severity.

**4.1 Privilege escalation via signup (critical). ✅ FIXED**
`RegisterDto.role?: string` (`services/user-service/src/user/dto/user.dto.ts:30`) is accepted from the request body and passed straight into `prisma.user.create({ data: { ..., role } })`. `/user/signup` is a public route. **Anyone can register themselves as `admin`** and then reach every admin-gated endpoint (list all users, list all preferences, create templates).

**4.2 `POST /notifications` is unauthenticated (critical). ✅ FIXED**
`api-gateway/src/main.ts:79` — `requireAuth: () => false`, with a comment that says the opposite ("all orchestrator routes require authentication"). The orchestrator has no auth of its own. Anyone who can reach the gateway can submit unlimited notifications to arbitrary user IDs.

**4.3 Nest guards never run on proxied routes (critical).**
Proxy routes are registered with raw `app.use()` in `main.ts` *before* Nest's router. `JwtAuthGuard` (`APP_GUARD`) and the interceptors only apply to Nest controller routes — which is just `GET /health`. All real traffic bypasses them. Auth is re-implemented ad hoc inside the `app.use` closure.

**4.4 Rate limiting does not exist.**
`ThrottlerModule` is configured with a custom Redis storage backend (77 lines in `throttler/redis-storage.service.ts`), but `ThrottlerGuard` is **never registered** as an `APP_GUARD` — and even if it were, §4.3 means it wouldn't apply to proxied routes. The entire throttling subsystem is dead code.

**4.5 Auth bypass via substring matching.**
Both `main.ts:60` and `jwt-auth.guard.ts:59` decide a route is public with `requestPath.includes('signin') || requestPath.includes('signup')`. Any path containing those substrings — `/template/signin-banner`, `/user/x?q=signin` — is treated as public. The public-route list is also duplicated in three places with three slightly different implementations.

**4.6 Unauthenticated preference endpoint. ✅ FIXED**
`services/user-service/src/user/user.controller.ts:76` — `// @UseGuards(JwtAuthGaurd)` is commented out on `GET /user/preference/:id`. This is the endpoint the orchestrator calls. Since the service also publishes port 3007, user preference data is directly readable by anyone on the network.

**4.7 Django settings are development-only.**
`DEBUG = True`, `SECRET_KEY = 'django-insecure-...'` hardcoded in source, `ALLOWED_HOSTS = ['*']`. Incompatible with the "production-ready" claim.

**4.8 Compose credential drift. ✅ FIXED**
The gateway gets `REDIS_PASSWORD: redis_password` and `REDIS_URL: redis://:password@redis:6379/0` (two different passwords), while the Redis container uses `${REDIS_PASSWORD:-}` (empty by default). Three inconsistent values; gateway Redis auth will fail against a default-config Redis.

Note: `.env` files are correctly gitignored — only `.env.example`/`.env.sample` are tracked. No committed secrets found.

---

## 5. Correctness bugs

**5.1 Notifications are persisted with random UUIDs (critical).**
`services/orchestrator/internal/services/orchestrator.go:61-67`:
```go
//TODO:convert strings to dto
UserID:        uuid.New(), //req.UserID,
TemplateID:    uuid.New(), //req.TemplateCode,
CorrelationID: uuid.New(), //correlationID,
```
Every notification row is written with three freshly-generated random UUIDs instead of the real user, template, and correlation IDs. Consequences:
- No notification can ever be looked up by user, template, or correlation ID.
- `GetByCorrelationID`, `GetUserNotifications`, `GetStatsByDateRange` can never return correct data.
- The `notification_events` rows are keyed to a `correlationUUID` parsed from the *real* correlation ID, which does not match the `correlation_id` stored on the notification row. The audit trail is internally inconsistent.

The helper that does this correctly (`utils.ToNotificationModel`) exists and is **never called**. This one bug invalidates the entire persistence and status-tracking story.

**5.2 Fire-and-forget goroutine loses work.**
`handlers/notification.go` — `go h.orchestrator.EnrichAndPublish(context.Background(), ...)`. Detached from the request context, unbounded (no worker pool or semaphore), and not durable: any process restart loses every in-flight notification. There is no outbox and no reconciliation job, so nothing recovers them.

**5.3 Idempotency is implemented backwards.**
The key is written to Redis with a 24h TTL *before* any work happens. If enrichment then fails, the key remains — so the client's retry is answered with "duplicate request detected, status: processing" for the next 24 hours, and the notification is never sent. Idempotency should be claimed atomically (`SET NX`) and either finalized on success or released on failure.

**5.4 Server timeouts are effectively infinite. ✅ FIXED**
`internal/server/server.go:31` — `time.Duration(cfg.Server.ReadTimeout) * time.Second`. `ReadTimeout` is already a `time.Duration`, and compose sets it to `15s`. So this computes `15e9 * 1e9` nanoseconds ≈ **475 years**. Same bug for write and idle timeouts. The server has no effective timeouts.

**5.5 Dead duplicate-detection branch.**
`repositories/notification.go` `CreateNotification` checks `result.RowsAffected() == 0` to detect duplicates, but the `INSERT` has no `ON CONFLICT DO NOTHING` — a real conflict raises a unique-violation error instead, and `RowsAffected()` is never 0. The branch is unreachable, and if it were reached it would nil-deref `*notif.IdempotencyKey`.

**5.6 Potential runtime panic on struct comparison.**
`orchestrator.go` compares `result.user == dtos.HTTPResponse{}`. `HTTPResponse.Data` is `interface{}`, and after JSON decoding it holds a `map[string]interface{}` — an uncomparable type. When the first field (`Success`) doesn't short-circuit the comparison (i.e. an error response where `Success` is false but `Data` is populated), this **panics**. The panic is inside a goroutine, so it takes down the whole process.

**5.7 Push retry loop with no ceiling.**
`push-service/internal/rabbitmq.go` — on handler error, `delivery.Nack(false, true)` requeues indefinitely. No retry counter, no backoff, no dead-letter exchange. A poison message pins a consumer forever. None of the orchestrator's queue declarations set `x-dead-letter-exchange` either, so the README's DLQ story only exists inside the Celery task.

**5.8 No publisher confirms.**
`publishToQueue` uses `mandatory: false` and no confirm mode. An unroutable message is discarded silently and the orchestrator still marks the notification `queued`.

**5.9 Partitioning will break.**
The `notifications` table is `PARTITION BY RANGE (created_at)`, and `create_notification_partitions()` creates only the current and next month. It's called once, at migration time. **Inserts will start failing ~2 months after deployment** with no partition for the range. Nothing schedules it to run again.

**5.10 Queries ignore the partition key.**
`UpdateStatus`, `GetByID`, etc. filter on `id` alone while the primary key is `(id, created_at)`. Every one of these fans out across all partitions.

**5.11 Double-close on shutdown.**
`main.go` `defer db.Close()` and `defer redisClient.Close()`, while `srv.Shutdown()` also closes the DB and `orchestrator.Shutdown()` also closes Redis and the AMQP channel.

**5.12 Dead code in the repository layer.**
14 of 19 repository methods are never called: `GetByID`, `GetByCorrelationID`, `GetByIdempotencyKey`, `GetUserNotifications`, `GetUserNotificationsWithCursor`, `GetFailedForRetry`, `GetStatsByDateRange`, `SoftDelete`, `CreateNotificationWithTransaction`, `BeginTx`, `UpdateStatusWithTimestamp`, `CreateEvent`, `GetEventsByNotificationID`, `GetEventsByCorrelationID`. The data layer for status queries, retry tooling, and analytics is **already written** — it just has no HTTP routes. This is the cheapest high-value work in the repo (see §7, Phase 2).

**5.13 Orchestrator ignores its own service-URL config. ✅ FIXED**
`main.go` hardcodes `http://template-service:3003` and `http://user-service:3007`. Compose sets `ORCHESTRATOR_EXTERNAL.*`, and the `ExternalServices` config struct exists but is **commented out**. The env vars are silently ignored.

**5.14 Duplicated Prisma schema.**
`services/user-service/prisma/schema.prisma` and `services/template-service/prisma/schema.prisma` are **byte-identical** — both define `User`, `Preference`, `Template`, `TemplateVersion` — but point at different databases. Each service materializes all four tables, half of which it never uses. Any model change must be hand-synced across both.

---

## 6. Quality / hygiene

- **Zero real test coverage.** All 8 `.spec.ts` files are 18–22-line "should be defined" stubs. `notifications/tests.py` is the untouched Django template. `npx jest` in template-service is red.
- Large commented-out blocks left in source: `orchestrator.go` (~25 lines of a superseded channel pattern), `config/config.go` (~100 lines of an old `parseMapString` loader), `Taskfile.yml` (almost entirely commented out), `app.module.ts` (`NotificationModule`).
- Commented-out `logger.Info()` calls in push-service using `string(int)` conversions — those were removed because they were bugs (`string(5)` yields `"\x05"`, not `"5"`), but the correct version was never written back.
- Six overlapping status docs in `services/email-service/` (`ANALYSIS.md`, `HOW_IT_WORKS.md`, `IMPLEMENTATION_SUMMARY.md`, `QUICKSTART.md`, `STATUS.md`, `SYSTEM_DESIGN.md`, `README.md`) — ~80KB of prose for one service.
- Two divergent compose files: `docker-compose.yaml` (has Consul, no push-service) and `docker-compose.local.yaml` (has push-service, no Consul). Neither has email-service.
- `packages/common/` is referenced as shared code but contains two trivial files that nothing imports.
- Typos in public API surface: `JwtAuthGaurd`, `"Invalid Request body"` / `"requets"`, `isSucessful`.
- Uncommitted working tree: `api-gateway/src/app.controller.spec.ts` deleted, `src/main.ts` modified.

---

## 7. Direction to completion

The distance to "working end to end" is much shorter than the distance to "production-ready." Phase 1 alone is roughly a week and converts a collection of services into a functioning system — that is the single highest-leverage block of work in this repo.

### Phase 0 — Stop the bleeding ✅ COMPLETE

Small, independent, no design decisions needed. All items applied and verified.

1. ✅ Rename the orchestrator route to `POST /notifications` so the gateway mount and the downstream route agree. *(§3.4 — a one-line change that unblocks the entire ingress path; do it first)*
2. ✅ Remove `role` from `RegisterDto` and hardcode `role: 'user'` in `signup()`. Add a separate admin-only `PATCH /user/:id/role`. *(§4.1)*
3. ✅ Restore `@UseGuards(JwtAuthGaurd)` on `GET /user/preference/:id` and pass a service-to-service token from the orchestrator. *(§4.6)*
4. ✅ Add `/health` to the gateway's public-route list, or introduce a `@Public()` decorator and have `JwtAuthGuard` consult its `Reflector`. *(§3.5)*
5. ✅ Fix the timeout multiplication in `server.go:29-33` — drop `* time.Second`. *(§5.4)*
6. ✅ Fix the push-service port: make compose map `8081:8080`, or set `PORT: 8081`. Align `PUSH_SERVICE_URL`. *(§3.11)*
7. ✅ Reconcile the three Redis passwords in compose to a single `${REDIS_PASSWORD}`. *(§4.8)*
8. ✅ Delete the `push_queue` binding from `config.go` — keep only the configured queue name. *(§3.3)*
9. ✅ Add `baseUrl`/`paths` to the base `tsconfig.json` and a jest `moduleNameMapper` so `npm test` and editors resolve `src/*`. *(§1)*

Two additional bugs surfaced while doing the above and were fixed with them: template enrichment 401'd on every request (§3.6), and both NestJS Compose healthchecks probed the wrong path (§3.7).

**Also fixed, out of the listed order:** `requireAuth` on the gateway's `/notifications` route was flipped to `true` (§4.2). This was Phase 2 work, but item 1 made the route reachable for the first time — leaving it open would have turned a 404 into a live unauthenticated notification-submission endpoint. The two changes had to land together.

**Introduced by Phase 0:** service-to-service calls now authenticate with a shared `INTERNAL_SERVICE_TOKEN` (`X-Service-Token` header), required by the orchestrator, user, and template services at startup. It is an interim measure — replace with mTLS or signed service JWTs in Phase 4.

### Phase 1 — Make one notification actually arrive (~1 week)

This is the critical path. Nothing else matters until a `POST /notifications` produces a delivered email.

1. **Fix the persistence bug** (§5.1). Parse `req.UserID` / `req.TemplateCode` / `correlationID` into real UUIDs and validate them at the handler boundary — reject with 400 rather than generating placeholders. Call the existing `utils.ToNotificationModel`. *Do this first; nothing downstream is verifiable until notification rows are real.*

2. **Render templates in the orchestrator, not the workers.** The template service already exposes `POST /template/:id/render` with Handlebars. Call it during enrichment and put the rendered `subject`/`title`/`body` on `EnrichedNotification`. This removes the need for rendering logic in every worker and fixes the push service's missing title/body.

3. **Resolve recipients during enrichment.** Fetch the user's email and `push_token` alongside preferences, and add `recipient` / `tokens` to `EnrichedNotification`. Honour `email_opt_in` / `push_opt_in` — short-circuit to a `cancelled` status if the user has opted out. *(This is what makes §3.2 work.)*

4. **Define one shared message contract.** Write the enriched-message schema down once (a `docs/contracts/` JSON Schema, or a shared Go package). Today the orchestrator's `EnrichedNotification`, the push service's `PushNotificationMessage`, and the Celery task's expected `payload` are three unrelated shapes. Make the push service's struct match what the orchestrator sends.

5. **Bridge email.** Add a small AMQP consumer to the Django service that binds `email_queue` → `notification.email`, and calls `send_email_task.delay(payload)`. Point `fetch_email_template` at the right host/port/path — or delete it entirely, since step 2 makes it unnecessary. Fix `STATUS_CALLBACK_URL`. *(§3.1)*

6. **Deploy email-service.** Add `Dockerfile.dev`, plus `email-service` and `celery-worker` services to `docker-compose.local.yaml`.

7. **Prove it.** A single script that POSTs one notification and asserts a delivered `EmailLog` row. Use MailHog or Django's `console` email backend so this runs without real SMTP.

**Exit criterion:** `docker compose up`, one `curl`, one email delivered, one `notifications` row in status `sent`.

### Phase 2 — Close the API surface (~3 days)

Mostly wiring; the hard part is already written (§5.12).

1. `GET /notification/{id}` and `GET /notification/correlation/{id}` → `GetByID` / `GetByCorrelationID`, Redis-first with a Postgres fallback.
2. `GET /notifications?user_id=…` → `GetUserNotificationsWithCursor`.
3. `GET /notifications/{id}/events` → `GetEventsByNotificationID` (the audit timeline).
4. `POST /notifications/{id}/retry` → `GetFailedForRetry`.
5. A status-callback endpoint so workers can report delivery back — this is what `report_status` and the push service's Redis status store are both trying to reach.
6. Route `/notifications` and the new routes through the gateway with auth **on**. *(§4.2)*

### Phase 3 — Fix the reliability story (~1 week)

The README claims these patterns; the code doesn't implement them yet.

1. **Transactional outbox.** Replace the fire-and-forget goroutine (§5.2) with an outbox table written in the same transaction as the notification row, plus a publisher loop. This is the only way the "no silent drops" claim becomes true.
2. **Correct idempotency** (§5.3): `SET NX` to claim, release on failure, and back it with the existing unique index on `idempotency_key`.
3. **Real DLQs**: `x-dead-letter-exchange` on every queue declaration, a retry counter in message headers, and a bounded retry policy in the push consumer instead of infinite `Nack(requeue=true)` (§5.7).
4. **Publisher confirms** and `mandatory: true` with a return handler (§5.8).
5. **Partition maintenance**: a scheduled job (pg_cron, or a goroutine ticker) calling `create_notification_partitions()`, creating 3 months ahead (§5.9). Add `created_at` to the single-row query predicates (§5.10).
6. Fix the struct-comparison panic (§5.6) — use explicit error fields instead of zero-value comparison. Add `recover()` to the enrichment goroutine.
7. Fix the unreachable duplicate branch (§5.5) — add `ON CONFLICT (idempotency_key, created_at) DO NOTHING`.

### Phase 4 — Harden the edge (~3 days)

1. Convert the gateway's `app.use()` proxy registration into a proper Nest middleware/controller so guards and interceptors actually apply (§4.3). This is a real refactor, not a config change.
2. Register `ThrottlerGuard` as an `APP_GUARD` (§4.4).
3. Replace substring route matching with an exact allowlist, defined **once** and shared (§4.5).
4. Service-to-service auth: internal JWT or mTLS, so services aren't trusting bare `x-user-id` headers.
5. Django production settings: `SECRET_KEY` from env, `DEBUG` from env, real `ALLOWED_HOSTS` (§4.7).

### Phase 5 — Tests, then observability (~1 week)

Do these in this order — instrumenting untested code just produces confident dashboards over broken behaviour.

1. Replace the 8 stub specs with real unit tests. Priority: `user.service` (auth, role assignment), `template.service` (rendering, versioning), orchestrator enrichment (table-driven, mocked clients).
2. Integration test on the Phase 1 harness: gateway → orchestrator → queue → worker → status, with testcontainers or the compose stack.
3. Failure-path tests: user service down, template missing, SMTP failing, poison message → DLQ.
4. CI: `go vet` + `go test`, `nest build` + `jest`, `manage.py test`. There is a `.github/workflows/` dir under email-service only — lift it to the repo root.
5. *Then* OpenTelemetry (correlation IDs already exist — propagate them as trace context), Prometheus (queue depth, retry rate, delivery latency), Grafana.

### Phase 6 — Consolidation

1. Fix §5.14: give each service a schema containing only its own models, or extract a shared Prisma package.
2. Delete the ~130 lines of commented-out code in `orchestrator.go` and `config/config.go`; uncomment and wire `ExternalServices` (§5.13).
3. Collapse the six email-service status docs into one README.
4. Reconcile the two compose files, or delete `docker-compose.yaml`.
5. Rewrite the root README's status table against §2 — the current one undersells three services and oversells the one that isn't deployed.

---

## 8. Suggested ordering, in one line

**§7 Phase 0 → fix §5.1 → Phase 1 (contract + recipients + email bridge) → Phase 2 (status API) → Phase 4.1–4.2 (auth) → Phase 3 (outbox/DLQ) → Phase 5 (tests → observability).**

The project's real problem is not missing features — it is that six well-built services were never introduced to each other. Phase 1 is worth more than everything after it combined.
