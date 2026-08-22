# Template Service

Message content: templates, their versions, and rendering.

Part of the [Notification System](../../README.md).

| | |
|---|---|
| **Stack** | NestJS 11, Prisma 6, Handlebars, ioredis |
| **Port** | 3003 |
| **Owns** | `template_service_db` — `Template`, `TemplateVersion` |

---

## API

| Method | Route | Auth |
|---|---|---|
| `POST` | `/template` | Admin |
| `GET` | `/template` | JWT — paginated; filter by `name`, `language`, `event`, `channel` |
| `GET` | `/template/:id` | JWT — `?history=true` returns every version |
| `POST` | `/template/:id/render` | JWT — compiles against a supplied context |
| `GET` | `/template/event/:event/channel/:channel` | JWT |
| `PATCH` | `/template/:id` | JWT — creates a new version |
| `DELETE` | `/template/:id` | JWT |
| `GET` | `/template-service/health` | Public |

---

## Versions are immutable

A `PATCH` creates a new `TemplateVersion` rather than mutating the current one.
Editing a template in place would silently change messages already in flight and
rewrite what past notifications said they sent. Versioning keeps rendering
reproducible and content changes auditable.

Templates are unique on `(event, language)`, which is what makes localized
lookup by event unambiguous.

---

## Rendering is pure

`POST /template/:id/render` takes a template and a context and returns compiled
content — one message per channel the template declares, with `subject`/`html`
for email and `title`/`body` for push.

It does **not** look up users. It previously resolved recipients from this
service's own `User` table, a schema copy nothing ever wrote to, so it returned
404 for every possible input while also duplicating consent logic the
orchestrator owns. Deciding *who* receives a notification belongs to the
orchestrator; this service owns *what it says*.

---

## Running it

```bash
docker compose -f ../../infra/docker-compose.local.yaml up template-service

# or directly
pnpm install && pnpm prisma migrate deploy && pnpm start:dev
```

| Variable | Purpose |
|---|---|
| `PORT` | Listen port (3003) |
| `DATABASE_URL` | Postgres connection string |
| `JWT_SECRET` | Must match every other service |
| `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD` | Cache (30 min TTL) |

---

## Tests

```bash
pnpm test
```

10 tests covering rendering: that the latest version wins, that each declared
channel gets its own message with the right fields, that an empty context
renders rather than throwing — and that no user lookup happens.
