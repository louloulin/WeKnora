#!/bin/bash
# Smoke test for Casdoor <-> Gateway <-> WeKnora exchange integration.
#
# Verifies the full token-exchange + introspection pipeline that the
# OpenBuddy desktop client relies on. This is a real network probe:
#
# 1. Pulls an admin password-grant access_token from the target Casdoor.
# 2. Starts the local Gateway with a fixed store=memory + webhook secret +
#    WeKnora tenant map.
# 3. Exchanges the Casdoor token at /v1/token-exchange/weknora and
#    asserts the response is 200 with HS256 + weknora_exchange token_type.
# 4. Introspects the resulting exchange token and asserts active=true
#    with all required claims (subject/casdoor_tenant/tenant_id/session_id/
#    membership_version/authorization_version/jti).
# 5. Tampers the exchange token signature and asserts introspect returns
#    401 INVALID_EXCHANGE_TOKEN.
# 6. Triggers a role-update webhook (HMAC-signed) and asserts introspect
#    returns 403 AUTHORIZATION_VERSION_REVOKED.
# 7. Triggers a webhook with a wrong signature and asserts 401
#    WEBHOOK_SIGNATURE_INVALID.
#
# Usage:
#   CASDOOR_BASE_URL=http://124.221.146.145:8000 \
#   CASDOOR_CLIENT_ID=005d6839fe25abd6696f \
#   CASDOOR_ADMIN_USERNAME=admin \
#   CASDOOR_ADMIN_PASSWORD=123 \
#   GATEWAY_PORT=8787 \
#   WEKNORA_TENANT_ID=1 \
#   ./scripts/smoke-casdoor-weknora-exchange.sh
#
# All defaults point at the development target instance. Secrets are read
# from the environment so the script never embeds credentials.

set -uo pipefail

CASDOOR_BASE_URL="${CASDOOR_BASE_URL:-http://124.221.146.145:8000}"
CASDOOR_CLIENT_ID="${CASDOOR_CLIENT_ID:-005d6839fe25abd6696f}"
CASDOOR_ADMIN_USERNAME="${CASDOOR_ADMIN_USERNAME:-admin}"
CASDOOR_ADMIN_PASSWORD="${CASDOOR_ADMIN_PASSWORD:-}"
GATEWAY_HOST="${GATEWAY_HOST:-127.0.0.1}"
GATEWAY_PORT="${GATEWAY_PORT:-8787}"
WEKNORA_TENANT_ID="${WEKNORA_TENANT_ID:-1}"
GATEWAY_WEKNORA_EXCHANGE_SECRET="${GATEWAY_WEKNORA_EXCHANGE_SECRET:-local-probe-secret-please-rotate-now}"
GATEWAY_WEKNORA_EXCHANGE_AUDIENCE="${GATEWAY_WEKNORA_EXCHANGE_AUDIENCE:-weknora}"
GATEWAY_WEBHOOK_SECRET="${GATEWAY_WEBHOOK_SECRET:-probe-webhook-secret}"
GATEWAY_DATA_DIR="${GATEWAY_DATA_DIR:-/tmp/smoke-casdoor-weknora-exchange-data}"
GATEWAY_DIST_PATH="${GATEWAY_DIST_PATH:-}"

PASS=0
FAIL=0
note() {
  printf '%s\n' "$*"
}

fail() {
  printf 'FAIL: %s\n' "$*"
  FAIL=$((FAIL + 1))
}

ok() {
  printf 'OK: %s\n' "$*"
  PASS=$((PASS + 1))
}

require_jq_or_python() {
  if command -v jq >/dev/null 2>&1; then
    JSON_TOOL="jq"
  elif command -v python3 >/dev/null 2>&1; then
    JSON_TOOL="python3"
  else
    echo "ERROR: jq or python3 required for JSON parsing" >&2
    exit 2
  fi
}

json_get() {
  local path="$1"
  if [ "$JSON_TOOL" = "jq" ]; then
    jq -r "$path"
  else
    python3 -c "import sys, json; d = json.load(sys.stdin); ks = '$path'.lstrip('.').split('.'); cur = d
for k in ks:
    if k == '':
        continue
    if k.endswith(']') and '[' in k:
        head, rest = k.split('[', 1)
        rest = rest.rstrip(']')
        if head:
            cur = cur[head]
        idx = int(rest)
        cur = cur[idx]
    else:
        cur = cur[k]
print(cur)"
  fi
}

require_jq_or_python

if [ -z "$CASDOOR_ADMIN_PASSWORD" ]; then
  echo "ERROR: CASDOOR_ADMIN_PASSWORD must be set (read from env, never CLI arg)" >&2
  exit 2
fi

# 1. Acquire Casdoor admin access_token via password grant.
note "=== step 1: get Casdoor admin access_token ==="
TOKEN_RESPONSE=$(curl -sS -m 10 -X POST -H "Content-Type: application/json" \
  -d "{\"username\":\"$CASDOOR_ADMIN_USERNAME\",\"password\":\"$CASDOOR_ADMIN_PASSWORD\",\"grant_type\":\"password\",\"client_id\":\"$CASDOOR_CLIENT_ID\",\"client_secret\":\"\",\"scope\":\"openid\"}" \
  "$CASDOOR_BASE_URL/api/login/oauth/access_token")
CASDOOR_TOKEN=$(printf '%s' "$TOKEN_RESPONSE" | json_get ".access_token")
if [ -z "$CASDOOR_TOKEN" ] || [ "$CASDOOR_TOKEN" = "None" ]; then
  fail "could not parse access_token from Casdoor response: $TOKEN_RESPONSE"
  exit 1
fi
ok "got Casdoor token (length=${#CASDOOR_TOKEN})"

# 2. Start local Gateway.
note "=== step 2: start local Gateway on $GATEWAY_HOST:$GATEWAY_PORT ==="
mkdir -p "$GATEWAY_DATA_DIR"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATEWAY_DIST=""
for candidate in \
  "$GATEWAY_DIST_PATH" \
  "$SCRIPT_DIR/../../../.codex/worktrees/76d8/OpenBuddy/services/casdoor-resource-gateway/dist/index.js" \
  "$(dirname "$SCRIPT_DIR")/services/casdoor-resource-gateway/dist/index.js" \
  "$SCRIPT_DIR/../../services/casdoor-resource-gateway/dist/index.js"; do
  if [ -n "$candidate" ] && [ -f "$candidate" ]; then
    GATEWAY_DIST="$candidate"
    break
  fi
done
if [ -z "$GATEWAY_DIST" ]; then
  echo "ERROR: could not locate casdoor-resource-gateway/dist/index.js. Set GATEWAY_DIST_PATH or run from the WeKnora repo root." >&2
  exit 2
fi
NODE_ENV=development \
CASDOOR_ISSUER="$CASDOOR_BASE_URL" \
CASDOOR_CLIENT_ID="$CASDOOR_CLIENT_ID" \
RESOURCE_GATEWAY_WEKNORA_EXCHANGE_SECRET="$GATEWAY_WEKNORA_EXCHANGE_SECRET" \
RESOURCE_GATEWAY_WEKNORA_EXCHANGE_AUDIENCE="$GATEWAY_WEKNORA_EXCHANGE_AUDIENCE" \
RESOURCE_GATEWAY_WEKNORA_TENANT_MAP_JSON="{\"built-in\":$WEKNORA_TENANT_ID}" \
RESOURCE_GATEWAY_WEBHOOK_SECRET="$GATEWAY_WEBHOOK_SECRET" \
PORT="$GATEWAY_PORT" HOST="$GATEWAY_HOST" \
RESOURCE_GATEWAY_DATA_DIR="$GATEWAY_DATA_DIR" \
RESOURCE_GATEWAY_STORE=memory \
node "$GATEWAY_DIST" > /tmp/smoke-casdoor-weknora-exchange-gw.log 2>&1 &
GATEWAY_PID=$!
trap 'kill $GATEWAY_PID 2>/dev/null || true' EXIT

# Wait for /healthz
for i in $(seq 1 30); do
  if curl -sS -m 2 "http://$GATEWAY_HOST:$GATEWAY_PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

HEALTH=$(curl -sS -m 5 "http://$GATEWAY_HOST:$GATEWAY_PORT/healthz")
if echo "$HEALTH" | grep -q '"ok":true'; then
  ok "gateway health: $HEALTH"
else
  fail "gateway health did not return ok: $HEALTH"
  cat /tmp/smoke-casdoor-weknora-exchange-gw.log
  exit 1
fi

# 3. POST /v1/token-exchange/weknora with the Casdoor token.
note "=== step 3: exchange Casdoor token for WeKnora exchange token ==="
EXCHANGE_RESPONSE=$(curl -sS -m 5 -X POST "http://$GATEWAY_HOST:$GATEWAY_PORT/v1/token-exchange/weknora" \
  -H "Authorization: Bearer $CASDOOR_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"tenant\":\"built-in\",\"weknoraTenantId\":$WEKNORA_TENANT_ID}")
EXCHANGE_TOKEN=$(printf '%s' "$EXCHANGE_RESPONSE" | json_get ".data.access_token")
if [ -z "$EXCHANGE_TOKEN" ] || [ "$EXCHANGE_TOKEN" = "None" ]; then
  fail "exchange failed: $EXCHANGE_RESPONSE"
  exit 1
fi
TOKEN_TYPE=$(printf '%s' "$EXCHANGE_RESPONSE" | json_get ".data.token_type")
if [ "$TOKEN_TYPE" = "Bearer" ]; then
  ok "exchange returned Bearer token"
else
  fail "exchange token_type = '$TOKEN_TYPE' (expected Bearer)"
fi

# 4. POST /v1/token-exchange/weknora/introspect with the exchange token.
note "=== step 4: introspect the exchange token ==="
INTROSPECT_RESPONSE=$(curl -sS -m 5 -X POST "http://$GATEWAY_HOST:$GATEWAY_PORT/v1/token-exchange/weknora/introspect" \
  -H "Authorization: Bearer $EXCHANGE_TOKEN")
ACTIVE=$(printf '%s' "$INTROSPECT_RESPONSE" | json_get ".data.active")
if [ "$ACTIVE" = "True" ] || [ "$ACTIVE" = "true" ]; then
  ok "introspect active=$ACTIVE"
else
  fail "introspect active=$ACTIVE: $INTROSPECT_RESPONSE"
fi

for claim in subject casdoor_tenant tenant_id session_id membership_version authorization_version jti; do
  VAL=$(printf '%s' "$INTROSPECT_RESPONSE" | json_get ".data.$claim")
  if [ -n "$VAL" ] && [ "$VAL" != "None" ]; then
    ok "introspect claim $claim present"
  else
    fail "introspect claim $claim missing or empty: $VAL"
  fi
done

# 5. Tamper with the signature.
note "=== step 5: tampered exchange token -> 401 INVALID_EXCHANGE_TOKEN ==="
TAMPERED="${EXCHANGE_TOKEN%????}XXXX"
TAMPER_STATUS=$(curl -sS -m 5 -o /tmp/smoke-casdoor-weknora-tampered.json -w "%{http_code}" \
  -X POST "http://$GATEWAY_HOST:$GATEWAY_PORT/v1/token-exchange/weknora/introspect" \
  -H "Authorization: Bearer $TAMPERED")
TAMPER_CODE=$(json_get < /tmp/smoke-casdoor-weknora-tampered.json ".code")
if [ "$TAMPER_STATUS" = "401" ] && [ "$TAMPER_CODE" = "INVALID_EXCHANGE_TOKEN" ]; then
  ok "tampered token rejected with INVALID_EXCHANGE_TOKEN"
else
  fail "tampered token got status=$TAMPER_STATUS code=$TAMPER_CODE"
fi

# 6. Webhook bumps authorization_version -> introspect returns AUTHORIZATION_VERSION_REVOKED.
note "=== step 6: webhook role update -> introspect 403 AUTHORIZATION_VERSION_REVOKED ==="
TIMESTAMP=$(date +%s)
WEBHOOK_BODY="{\"type\":\"role\",\"action\":\"update\",\"organization\":\"built-in\",\"role\":\"openbuddy-member\"}"
WEBHOOK_SIG=$(printf '%s' "$WEBHOOK_BODY" | openssl dgst -sha256 -hmac "$GATEWAY_WEBHOOK_SECRET" -binary | xxd -p -c 256)
WEBHOOK_STATUS=$(curl -sS -m 5 -o /tmp/smoke-casdoor-weknora-webhook.json -w "%{http_code}" \
  -X POST "http://$GATEWAY_HOST:$GATEWAY_PORT/v1/webhooks/casdoor" \
  -H "Content-Type: application/json" \
  -H "x-casdoor-timestamp: $TIMESTAMP" \
  -H "x-casdoor-signature: $WEBHOOK_SIG" \
  -d "$WEBHOOK_BODY")
if [ "$WEBHOOK_STATUS" = "200" ]; then
  ok "webhook accepted"
else
  fail "webhook returned status=$WEBHOOK_STATUS: $(cat /tmp/smoke-casdoor-weknora-webhook.json)"
fi

POST_WEBHOOK_STATUS=$(curl -sS -m 5 -o /tmp/smoke-casdoor-weknora-postwh.json -w "%{http_code}" \
  -X POST "http://$GATEWAY_HOST:$GATEWAY_PORT/v1/token-exchange/weknora/introspect" \
  -H "Authorization: Bearer $EXCHANGE_TOKEN")
POST_WEBHOOK_CODE=$(json_get < /tmp/smoke-casdoor-weknora-postwh.json ".code")
if [ "$POST_WEBHOOK_STATUS" = "403" ] && [ "$POST_WEBHOOK_CODE" = "AUTHORIZATION_VERSION_REVOKED" ]; then
  ok "post-webhook introspect rejected with AUTHORIZATION_VERSION_REVOKED"
else
  fail "post-webhook introspect got status=$POST_WEBHOOK_STATUS code=$POST_WEBHOOK_CODE"
fi

# 7. Webhook with bad signature -> 401 WEBHOOK_SIGNATURE_INVALID.
note "=== step 7: webhook with bad signature -> 401 ==="
BAD_WEBHOOK_STATUS=$(curl -sS -m 5 -o /tmp/smoke-casdoor-weknora-badwh.json -w "%{http_code}" \
  -X POST "http://$GATEWAY_HOST:$GATEWAY_PORT/v1/webhooks/casdoor" \
  -H "Content-Type: application/json" \
  -H "x-casdoor-timestamp: $TIMESTAMP" \
  -H "x-casdoor-signature: deadbeef" \
  -d "$WEBHOOK_BODY")
BAD_WEBHOOK_CODE=$(json_get < /tmp/smoke-casdoor-weknora-badwh.json ".code")
if [ "$BAD_WEBHOOK_STATUS" = "401" ] && [ "$BAD_WEBHOOK_CODE" = "WEBHOOK_SIGNATURE_INVALID" ]; then
  ok "bad-signature webhook rejected with WEBHOOK_SIGNATURE_INVALID"
else
  fail "bad-signature webhook got status=$BAD_WEBHOOK_STATUS code=$BAD_WEBHOOK_CODE"
fi

echo
echo "=== summary ==="
echo "passed: $PASS"
echo "failed: $FAIL"
exit $FAIL
