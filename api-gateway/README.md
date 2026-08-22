# API Gateway

The single entry point. Authenticates callers, enforces rate limits, injects
tracing headers, and proxies to the service that owns the data.

Part of the [Notification System](../README.md).

| | |
|---|---|
| **Stack** | NestJS 11, Express, Passport JWT, ioredis |
| **Port** | 8000 |
| **Owns** | No persistent state. Redis holds throttle counters only |
| **Talks to** | User Service, Template Service, Orchestrator |

---

## What it does

| Route | Proxies to | Auth |
|---|---|---|
| `/user/**` | User Service `:3007` | JWT, except `signup` and `signin` |
| `/template/**` | Template Service `:3003` | JWT |
| `/notifications/**` | Orchestrator `:8080` | JWT |
| `/health` | — | Public, and exempt from rate limiting |

On the way through it attaches `X-Correlation-ID` and `X-Idempotency-Key`,
generating them when the caller supplied none, and forwards the authenticated
user as `x-user-id`.

---

## Two decisions worth knowing

**Proxying happens in controllers, not middleware.** Middleware registered with
`app.use()` runs *before* Nest's router, so guards and interceptors never see it
— proxied traffic would bypass authentication and rate limiting entirely.
Controllers place it inside the pipeline where the guards are.

**Public routes are declared, not pattern-matched.** Exemptions use `@Public()`
on the handler, so exemption is a property of the route rather than a pattern
matched against the URL. It cannot drift from the endpoint it protects, and
there is no second list to keep in sync.

The full path is forwarded unchanged, including the mount prefix, so a
downstream service's route prefix must match the gateway's. `/user/signup` at
the edge is `/user/signup` at the User Service.

---

## Running it

```bash
# With the rest of the stack
docker compose -f ../infra/docker-compose.local.yaml up api-gateway

# On its own, against services already running
pnpm install && pnpm start:dev
```

| Variable | Purpose |
|---|---|
| `PORT` | Listen port (default 3000; Compose sets 8000) |
| `JWT_SECRET` | Must match every other service |
| `USER_SERVICE_URL`, `TEMPLATE_SERVICE_URL`, `ORCHESTRATOR_SERVICE_URL` | Upstreams |
| `REDIS_URL` | Throttle counters |
| `THROTTLE_TTL`, `THROTTLE_LIMIT` | Window in seconds, requests per window (default 60 / 100) |
| `CORS_ORIGIN` | `*`, or a comma-separated list |

---

## Tests

```bash
pnpm test
```

19 tests covering the auth surface: that `@Public()` is read from handler
metadata rather than the URL, that paths merely *containing* `signin` are still
guarded, that catch-all routes stay authenticated, and that a caller's own
correlation and idempotency headers are preserved rather than overwritten.
