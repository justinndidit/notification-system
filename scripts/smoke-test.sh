#!/usr/bin/env bash
#
# End-to-end smoke test: one notification request, one delivered email.
#
# Exercises the full path — gateway auth, orchestrator enrichment, template
# rendering, RabbitMQ, the queue bridge, and the Celery worker — and asserts
# the message actually arrived in MailHog.
#
# Usage:  ./scripts/smoke-test.sh
# Assumes: docker compose -f infra/docker-compose.local.yaml up

set -euo pipefail

GATEWAY="${GATEWAY_URL:-http://localhost:8000}"
TEMPLATE="${TEMPLATE_URL:-http://localhost:3003}"
ORCH="${ORCH_URL:-http://localhost:3002}"
MAILHOG="${MAILHOG_URL:-http://localhost:8025}"

RUN_ID="$(date +%s)"
EMAIL="smoke-$RUN_ID@example.com"
PASSWORD="smoke-password-123"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-45}"

pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }

need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }
need curl
need jq

# ---------------------------------------------------------------- preflight
step "Preflight"
for probe in "$GATEWAY/health" "$TEMPLATE/template-service/health" "$ORCH/health" "$MAILHOG/api/v2/messages"; do
  curl -fsS --max-time 5 "$probe" >/dev/null 2>&1 \
    || fail "not reachable: $probe"
done
pass "gateway, template service, orchestrator and MailHog are up"

# ------------------------------------------------------------------ sign up
step "Create a user"
SIGNUP=$(curl -fsS -X POST "$GATEWAY/user/signup" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"Smoke Test\",\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}") \
  || fail "signup failed"

USER_ID=$(echo "$SIGNUP" | jq -r '.data.user.id // .user.id // empty')

SIGNIN=$(curl -fsS -X POST "$GATEWAY/user/signin" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")

TOKEN=$(echo "$SIGNIN" | jq -r '.data.token // .token')
[ -n "$TOKEN" ] && [ "$TOKEN" != "null" ] || fail "no token returned from signin"
pass "signed up and authenticated as $EMAIL"

# Fall back to the JWT payload if signup did not echo the id back.
if [ -z "$USER_ID" ]; then
  PAYLOAD=$(echo "$TOKEN" | cut -d. -f2)
  # base64url -> base64, padded
  PAYLOAD=$(echo "$PAYLOAD" | tr '_-' '/+')
  case $(( ${#PAYLOAD} % 4 )) in 2) PAYLOAD="$PAYLOAD==" ;; 3) PAYLOAD="$PAYLOAD=" ;; esac
  USER_ID=$(echo "$PAYLOAD" | base64 -d 2>/dev/null | jq -r '.user_id // empty')
fi
[ -n "$USER_ID" ] || fail "could not determine the new user's id"
pass "user id $USER_ID"

# ---------------------------------------------------------------- template
step "Create a template"

# Creating a template requires an admin JWT, and there is currently no
# bootstrap path for the first admin (role is no longer settable at signup, and
# PATCH /user/:id/role itself requires an admin). For a local smoke test we mint
# an admin token from the shared JWT_SECRET. Replace this once the services
# grow a seed/bootstrap command.
ADMIN_TOKEN=$(python3 - "$USER_ID" "${JWT_SECRET:?JWT_SECRET must be set}" <<'PYJWT'
import base64, hashlib, hmac, json, sys, time

def b64(raw):
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()

user_id, secret = sys.argv[1], sys.argv[2]
header = b64(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
payload = b64(json.dumps(
    {"user_id": user_id, "role": "admin", "iat": int(time.time()), "exp": int(time.time()) + 900},
    separators=(",", ":"),
).encode())
signing_input = f"{header}.{payload}".encode()
signature = b64(hmac.new(secret.encode(), signing_input, hashlib.sha256).digest())
print(f"{header}.{payload}.{signature}")
PYJWT
)
[ -n "$ADMIN_TOKEN" ] || fail "could not mint an admin token"

TEMPLATE_RESP=$(curl -fsS -X POST "$TEMPLATE/template" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "Smoke Test Welcome",
    "event": "SMOKE_TEST_'"$RUN_ID"'",
    "channel": ["EMAIL"],
    "language": "en",
    "subject": "Welcome, {{name}}",
    "body": "<p>Hello {{name}}, your smoke test passed. <a href=\"{{link}}\">Continue</a></p>"
  }') || fail "template creation failed"

TEMPLATE_ID=$(echo "$TEMPLATE_RESP" | jq -r '.data.id // .id')
[ -n "$TEMPLATE_ID" ] && [ "$TEMPLATE_ID" != "null" ] || fail "no template id returned"
pass "template $TEMPLATE_ID"

# ------------------------------------------------------------ notification
step "Submit a notification"
IDEM="smoke-$(date +%s)-$RANDOM"

ACCEPTED=$(curl -fsS -X POST "$GATEWAY/notifications" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Idempotency-Key: $IDEM" \
  -d "{
    \"notification_type\": \"email\",
    \"user_id\": \"$USER_ID\",
    \"template_code\": \"$TEMPLATE_ID\",
    \"variables\": { \"name\": \"Smoke Test\", \"link\": \"https://example.com/start\" },
    \"request_id\": \"$IDEM\",
    \"priority\": 2
  }") || fail "notification submission was rejected"

CORRELATION=$(echo "$ACCEPTED" | jq -r '.data.correlation_id')
[ -n "$CORRELATION" ] && [ "$CORRELATION" != "null" ] || fail "no correlation id returned"
pass "accepted, correlation $CORRELATION"

# ----------------------------------------------------------------- delivery
step "Wait for delivery"
deadline=$(( SECONDS + TIMEOUT_SECONDS ))
delivered=""

while [ $SECONDS -lt $deadline ]; do
  delivered=$(curl -fsS "$MAILHOG/api/v2/messages" \
    | jq -r --arg to "$EMAIL" \
      '.items[]? | select(.To[]?.Mailbox + "@" + .To[]?.Domain == $to) | .ID' \
    | head -1)
  [ -n "$delivered" ] && break
  sleep 2
done

[ -n "$delivered" ] || fail "no email reached MailHog within ${TIMEOUT_SECONDS}s (check: docker compose logs email-bridge email-worker)"
pass "email delivered to $EMAIL"

SUBJECT=$(curl -fsS "$MAILHOG/api/v2/messages" \
  | jq -r --arg id "$delivered" '.items[] | select(.ID == $id) | .Content.Headers.Subject[0]')

case "$SUBJECT" in
  *"Smoke Test"*) pass "subject rendered from the template: \"$SUBJECT\"" ;;
  *) fail "subject was not rendered as expected: \"$SUBJECT\"" ;;
esac

# --------------------------------------------------------------- idempotency
step "Verify idempotency"
REPEAT=$(curl -fsS -X POST "$GATEWAY/notifications" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Idempotency-Key: $IDEM" \
  -d "{
    \"notification_type\": \"email\",
    \"user_id\": \"$USER_ID\",
    \"template_code\": \"$TEMPLATE_ID\",
    \"variables\": { \"name\": \"Smoke Test\", \"link\": \"https://example.com/start\" },
    \"request_id\": \"$IDEM\",
    \"priority\": 2
  }")

echo "$REPEAT" | jq -e '.message | test("[Dd]uplicate")' >/dev/null \
  && pass "repeat submission recognised as a duplicate" \
  || fail "repeat submission was not deduplicated"

printf '\n\033[32m\033[1mSmoke test passed.\033[0m Inbox: %s\n\n' "$MAILHOG"
