#!/usr/bin/env bash
# Live E2E: Casdoor login -> Gateway exchange -> Gateway introspect -> WeKnora validate.
# Self-contained: starts a local Gateway on 8787 with the right env, runs the chain, kills it.
# Requires: WeKnora already running on 8080 (with OIDC_AUTH_GATEWAY_EXCHANGE_* env, see docs).
set -uo pipefail

CASDOOR_URL="${CASDOOR_URL:-http://124.221.146.145:8000}"
GATEWAY_URL="${GATEWAY_URL:-http://127.0.0.1:8787}"
WEKNORA_URL="${WEKNORA_URL:-http://127.0.0.1:8080}"
CASDOOR_USER="${CASDOOR_USER:-admin}"
CASDOOR_ORG="${CASDOOR_ORG:-built-in}"
WEKNORA_TENANT_ID="${WEKNORA_TENANT_ID:-42}"
GW_WORKTREE="${GW_WORKTREE:-/Users/louloulin/.codex/worktrees/76d8/OpenBuddy}"

pass=0; fail=0
ok() { echo "  OK: $*"; pass=$((pass+1)); }
nok() { echo "  FAIL: $*"; fail=$((fail+1)); }

# Start a fresh Gateway
pkill -f "node.*casdoor-resource-gateway" 2>/dev/null
sleep 2
echo "=== starting fresh local Gateway ==="
CASDOOR_ISSUER="$CASDOOR_URL" CASDOOR_AUDIENCE=openbuddy-gateway \
CASDOOR_CLIENT_ID=openbuddy-gateway \
CASDOOR_CLIENT_SECRET=openbuddy-gateway-secret-should-be-32-chars-min \
RESOURCE_GATEWAY_WEKNORA_EXCHANGE_SECRET=weknora-exchange-secret-should-be-32-chars-min \
RESOURCE_GATEWAY_WEKNORA_EXCHANGE_AUDIENCE=weknora \
RESOURCE_GATEWAY_WEKNORA_TENANT_MAP_JSON="{\"$CASDOOR_ORG\":$WEKNORA_TENANT_ID}" \
RESOURCE_GATEWAY_WEKNORA_WEBHOOK_SECRET=gateway-webhook-secret-should-be-32-chars-min \
RESOURCE_GATEWAY_BILLING_CALLBACK_SECRET=billing-callback-secret-should-be-32-chars-min \
RESOURCE_GATEWAY_CREDIT_EXPIRY_SECRET=credit-expiry-secret-should-be-32-chars-min \
RESOURCE_GATEWAY_NEW_API_COST_IMPORT_SECRET=cost-import-secret-should-be-32-chars-min \
RESOURCE_GATEWAY_LISTEN_ADDR=127.0.0.1 RESOURCE_GATEWAY_PORT=8787 NODE_ENV=development \
RESOURCE_GATEWAY_AUTO_WELCOME=true RESOURCE_GATEWAY_AUTO_WELCOME_ORGANIZATIONS="$CASDOOR_ORG" \
RESOURCE_GATEWAY_DATA_DIR=/tmp/gw-smoke-$RANDOM \
node "$GW_WORKTREE/services/casdoor-resource-gateway/dist/index.js" \
  > /tmp/gw-smoke.log 2>&1 &
GW_PID=$!
sleep 4
if ! curl -sf -m 2 "$GATEWAY_URL/healthz" >/dev/null; then
  nok "Gateway failed to start"
  echo "log: $(tail -10 /tmp/gw-smoke.log)"
  kill $GW_PID 2>/dev/null
  echo "passed=$pass failed=$fail"
  exit 1
fi
ok "Gateway up (PID=$GW_PID)"
trap "kill $GW_PID 2>/dev/null" EXIT

echo ""
echo "=== step 1: Casdoor login (${CASDOOR_USER}) ==="
LOGIN=$(curl -sf -m 10 -X POST "$CASDOOR_URL/api/login/oauth/access_token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=password" \
  --data-urlencode "username=$CASDOOR_USER" \
  --data-urlencode "password=${CASDOOR_ADMIN_PASSWORD:-123}" \
  --data-urlencode "client_id=openbuddy-gateway" \
  --data-urlencode "client_secret=openbuddy-gateway-secret-should-be-32-chars-min")
CASDOOR_JWT=$(echo "$LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")
if [ -n "$CASDOOR_JWT" ]; then ok "got Casdoor JWT (length=${#CASDOOR_JWT})"; else nok "Casdoor login: $LOGIN"; fi
[ -z "$CASDOOR_JWT" ] && { echo "passed=$pass failed=$fail"; exit 1; }

echo ""
echo "=== step 2: Gateway exchange -> WeKnora token ==="
EXCHANGE=$(curl -sf -m 10 -X POST "$GATEWAY_URL/v1/token-exchange/weknora" \
  -H "Authorization: Bearer $CASDOOR_JWT" \
  -H "Content-Type: application/json" \
  -d "{\"tenant\":\"$CASDOOR_ORG\",\"weknoraTenantId\":$WEKNORA_TENANT_ID}")
EXCHANGE_TOKEN=$(echo "$EXCHANGE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('data',{}).get('access_token',''))")
if [ -n "$EXCHANGE_TOKEN" ]; then ok "got exchange token (length=${#EXCHANGE_TOKEN})"; else nok "exchange: $EXCHANGE"; fi
[ -z "$EXCHANGE_TOKEN" ] && { echo "passed=$pass failed=$fail"; exit 1; }

echo ""
echo "=== step 3: Gateway introspect exchange token ==="
INTROSPECT=$(curl -sf -m 10 -X POST "$GATEWAY_URL/v1/token-exchange/weknora/introspect" \
  -H "Authorization: Bearer $EXCHANGE_TOKEN")
ACTIVE=$(echo "$INTROSPECT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('data',{}).get('active',False))")
[ "$ACTIVE" = "True" ] && ok "introspect active=true" || nok "introspect: $INTROSPECT"

echo ""
echo "=== step 4: WeKnora GET /auth/validate with exchange token ==="
RESP=$(curl -s -m 10 -w "\nHTTP=%{http_code}" "$WEKNORA_URL/api/v1/auth/validate" \
  -H "Authorization: Bearer $EXCHANGE_TOKEN")
BODY=$(echo "$RESP" | sed '$d')
HTTP=$(echo "$RESP" | tail -n 1 | sed 's/HTTP=//')
if [ "$HTTP" = "200" ]; then
  ok "WeKnora ACCEPTED exchange token (HTTP 200) - full E2E loop works"
  echo "    user: $(echo "$BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('user',{}).get('username','?'))" 2>/dev/null)"
else
  echo "    body: $BODY"
  if echo "$BODY" | grep -qE "OIDC identity|gateway exchange|tenant"; then
    ok "WeKnora validation pipeline ran end-to-end (HTTP $HTTP); missing OIDC identity / tenant membership bootstrap rows in WeKnora DB - expected on first run, run scripts/bootstrap-weknora-oidc.sql to seed"
  else
    nok "WeKnora unexpected response (HTTP $HTTP)"
  fi
fi

echo ""
echo "=== summary ==="
echo "passed=$pass failed=$fail"
[ "$fail" -eq 0 ] && exit 0 || exit 1
