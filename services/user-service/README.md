# User Service

Identity and delivery preferences: who the recipient is, how to reach them, and
whether they have consented to each channel.

Part of the [Notification System](../../README.md).

| | |
|---|---|
| **Stack** | NestJS 11, Prisma 6, bcrypt, Passport JWT, ioredis |
| **Port** | 3007 |
| **Owns** | `user_service_db` — `User`, `Preference` |

---

## API

| Method | Route | Auth |
|---|---|---|
| `POST` | `/user/signup` | Public |
| `POST` | `/user/signin` | Public |
| `GET` | `/user` | Admin |
| `GET` | `/user/preference` | Admin |
| `GET` | `/user/:id` | JWT |
| `GET` | `/user/preference/:id` | JWT (a user's or a service's) |
| `GET` | `/user/:id/delivery-profile` | JWT — contact details + consent, in one call |
| `PATCH` | `/user/:id/preference` | JWT (self) |
| `PATCH` | `/user/:id/device-tokens` | JWT (self) |
| `PATCH` | `/user/:id/role` | Admin |
| `GET` | `/user-service/health` | Public |

`delivery-profile` exists so the orchestrator can resolve a recipient in one
request instead of stitching several endpoints together.

---

## Roles

`role` is **not** settable at signup. Signup is public, so accepting a role from
the request body would let any caller register as an admin. Roles are assigned
through `PATCH /user/:id/role`, which requires an existing admin.

That leaves a bootstrap problem, solved explicitly rather than by a back door:

```bash
ADMIN_EMAIL=admin@example.com ADMIN_PASSWORD='...' pnpm seed:admin
```

Idempotent — it promotes an existing account rather than duplicating it — and it
enforces a stricter password floor than signup, because the account can read
every user's delivery profile.

---

## Device tokens

`device_tokens` holds FCM registration tokens as `[{ token, platform }]`, matching
what the push service sends through the FCM HTTP v1 API.

The whole set is replaced on write: FCM registration tokens rotate, so the client
owns the list. Duplicates are removed.

---

## Running it

```bash
docker compose -f ../../infra/docker-compose.local.yaml up user-service

# or directly
pnpm install && pnpm prisma migrate deploy && pnpm start:dev
```

| Variable | Purpose |
|---|---|
| `PORT` | Listen port (3007) |
| `DATABASE_URL` | Postgres connection string |
| `JWT_SECRET` | Must match every other service |
| `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD` | Cache |

---

## Tests

```bash
pnpm test
```

15 tests. The load-bearing ones: signup never takes `role` from the request body,
passwords are hashed rather than stored, and signin returns an identical error
for an unknown email and a wrong password so it cannot be used to enumerate
accounts.
