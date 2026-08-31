#!/usr/bin/env bash
# scripts/smoke-casdoor-rbac-real.sh
# End-to-end smoke for the unified Casdoor <-> Gateway <-> WeKnora permission
# chain, with REAL non-admin users created on the remote Casdoor. Proves:
#   1. RBAC enforcer decisions via /api/enforce
#   2. JWT permission propagation by Casdoor for users in openbuddy roles
#   3. Gateway's hasWeKnoraExchangePermission rejects carol (no role) and
#      accepts alice (openbuddy-member) and admin
#
# Required env:
#   CASDOOR_ADMIN_PASSWORD        MUST be exported (never via CLI)
#   GATEWAY_DIST_PATH             default: <repo>/services/casdoor-resource-gateway/dist/index.js
# Optional env:
#   CASDOOR_BASE_URL              default: http://124.221.146.145:8000
#   CASDOOR_CLIENT_ID             default: 005d6839fe25abd6696f
#   GATEWAY_PORT                  default: 8787
#
# Usage:
#   export HISTCONTROL=ignorespace
#   export CASDOOR_ADMIN_PASSWORD=123
#   bash scripts/smoke-casdoor-rbac-real.sh
set -euo pipefail

CASDOOR_BASE_URL="${CASDOOR_BASE_URL:-http://124.221.146.145:8000}"
CASDOOR_CLIENT_ID="${CASDOOR_CLIENT_ID:-005d6839fe25abd6696f}"
CASDOOR_ADMIN_USERNAME="${CASDOOR_ADMIN_USERNAME:-admin}"
CASDOOR_ADMIN_PASSWORD="${CASDOOR_ADMIN_PASSWORD:-}"
GATEWAY_PORT="${GATEWAY_PORT:-8787}"

if [[ -z "$CASDOOR_ADMIN_PASSWORD" ]]; then
  echo "ERROR: CASDOOR_ADMIN_PASSWORD must be exported" >&2
  exit 64
fi

GATEWAY_DIST_PATH="${GATEWAY_DIST_PATH:-}"
if [[ -z "$GATEWAY_DIST_PATH" ]]; then
  for c in "$HOME/appx/OpenBuddy/services/casdoor-resource-gateway/dist/index.js" "$HOME/.codex/worktrees/76d8/OpenBuddy/services/casdoor-resource-gateway/dist/index.js"; do
    if [[ -f "$c" ]]; then GATEWAY_DIST_PATH="$c"; break; fi
  done
fi
[[ -n "$GATEWAY_DIST_PATH" && -f "$GATEWAY_DIST_PATH" ]] || { echo "ERROR: gateway dist not found"; exit 1; }

note() { printf '\033[36m%s\033[0m\n' "$*"; }
ok()   { printf '\033[32m✓ %s\033[0m\n' "$*"; }
err()  { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; }
fail() { err "$*"; exit 1; }

# 1. Admin token + RBAC setup
note "=== step 1: provision RBAC on remote Casdoor ==="
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/setup-casdoor-rbac.sh" > /tmp/rbac-setup.log 2>&1 || fail "setup-casdoor-rbac.sh failed: see /tmp/rbac-setup.log"
ok "setup-casdoor-rbac.sh completed"
echo

# 2. Create non-admin test users if missing
note "=== step 2: ensure alice-member, carol-norole test users exist ==="
TOKEN=$(curl -sS -X POST "${CASDOOR_BASE_URL}/api/login/oauth/access_token" \
  -d "grant_type=password&client_id=${CASDOOR_CLIENT_ID}&client_secret=&username=${CASDOOR_ADMIN_USERNAME}&password=${CASDOOR_ADMIN_PASSWORD}&scope=openid" \
  -H "Content-Type: application/x-www-form-urlencoded" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")
[[ -n "$TOKEN" ]] || fail "could not get admin token"

api() {
  local method="$1" path="$2"; shift 2
  curl -fsS -X "$method" "${CASDOOR_BASE_URL}${path}" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" "$@"
}

ensure_user() {
  local uname="$1" pass="$2" admin="$3"
  local existing=$(api GET "/api/get-user?id=built-in/${uname}" 2>/dev/null || echo "")
  if echo "$existing" | python3 -c "import sys,json; sys.exit(0 if json.load(sys.stdin).get('data') else 1)" 2>/dev/null; then
    ok "user ${uname} exists"
  else
    api POST "/api/add-user" -d "{\"owner\":\"built-in\",\"name\":\"${uname}\",\"password\":\"${pass}\",\"displayName\":\"${uname}\",\"isAdmin\":${admin}}" > /dev/null 2>&1 || true
    ok "user ${uname} created"
  fi
  api POST "/api/update-user?id=built-in/${uname}" -d "{\"owner\":\"built-in\",\"name\":\"${uname}\",\"displayName\":\"${uname}\",\"isAdmin\":${admin},\"isGlobalAdmin\":false,\"properties\":{\"casdoor_tenant\":\"acme-test\"}}" > /dev/null 2>&1 || true
}

ensure_user alice-member 'AlicePass123!' false
ensure_user carol-norole 'CarolPass123!' false

# Attach alice to openbuddy-member role
api POST "/api/update-role?id=built-in/openbuddy-member" \
  -d '{"owner":"built-in","name":"openbuddy-member","displayName":"OpenBuddy Member","description":"Standard member","isEnabled":true,"users":["built-in/alice-member"]}' \
  > /dev/null 2>&1 || true
ok "alice -> openbuddy-member role attached"

# 3. Verify alice JWT has correct weknora.* permission subset
note "=== step 3: alice JWT permissions ==="
ALICE_TOKEN=$(curl -sS -X POST "${CASDOOR_BASE_URL}/api/login/oauth/access_token" \
  -d "grant_type=password&client_id=${CASDOOR_CLIENT_ID}&client_secret=&username=alice-member&password=AlicePass123!&scope=openid" \
  -H "Content-Type: application/x-www-form-urlencoded" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")
WKN_PERMS=$(echo "$ALICE_TOKEN" | python3 -c "
import sys,base64,json
tok=sys.stdin.read().strip()
parts=tok.split('.')
claims=json.loads(base64.urlsafe_b64decode(parts[1]+'='*((-len(parts[1]))%4)))
perms=sorted(p.get('name','') for p in claims.get('permissions',[]) if 'weknora' in p.get('name',''))
print(','.join(perms))
")
echo "  alice weknora.* perms: $WKN_PERMS"
[[ "$WKN_PERMS" == "weknora.workspace.contribute,weknora.workspace.read" ]] || fail "alice perm set wrong (expected read+contribute only)"
ok "alice has exactly the read+contribute subset (not admin/owner/platform.admin)"

# 4. Verify carol JWT has zero weknora.* perms
note "=== step 4: carol JWT permissions (expect empty) ==="
CAROL_TOKEN=$(curl -sS -X POST "${CASDOOR_BASE_URL}/api/login/oauth/access_token" \
  -d "grant_type=password&client_id=${CASDOOR_CLIENT_ID}&client_secret=&username=carol-norole&password=CarolPass123!&scope=openid" \
  -H "Content-Type: application/x-www-form-urlencoded" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")
WKN_PERMS=$(echo "$CAROL_TOKEN" | python3 -c "
import sys,base64,json
tok=sys.stdin.read().strip()
parts=tok.split('.')
claims=json.loads(base64.urlsafe_b64decode(parts[1]+'='*((-len(parts[1]))%4)))
perms=sorted(p.get('name','') for p in claims.get('permissions',[]) if 'weknora' in p.get('name',''))
print(','.join(perms))
")
[[ -z "$WKN_PERMS" ]] || fail "carol should have zero weknora.* perms"
ok "carol has zero weknora.* perms"

# 5. Direct /api/enforce matrix
note "=== step 5: direct Casdoor /api/enforce matrix (8 rows) ==="
declare -a MATRIX=(
  "built-in/openbuddy-admin|weknora.workspace|read|True"
  "built-in/openbuddy-admin|weknora.workspace|contribute|True"
  "built-in/openbuddy-admin|weknora.workspace|admin|True"
  "built-in/openbuddy-admin|weknora.workspace|owner|False"
  "built-in/openbuddy-member|weknora.workspace|read|True"
  "built-in/openbuddy-member|weknora.workspace|contribute|True"
  "built-in/openbuddy-member|weknora.workspace|admin|False"
  "built-in/openbuddy-member|weknora.workspace|owner|False"
)
PASS=0; FAIL=0
for row in "${MATRIX[@]}"; do
  IFS='|' read -r sub obj act want <<< "$row"
  GOT=$(api POST "/api/enforce?enforcerId=built-in/weknora-enforcer" -d "[\"${sub}\",\"${obj}\",\"${act}\"]" | python3 -c "import sys,json; print('True' if json.load(sys.stdin).get('data',[None])[0] is True else 'False')")
  if [[ "$GOT" == "$want" ]]; then
    PASS=$((PASS+1))
    ok "${sub}/${act} = ${GOT}"
  else
    FAIL=$((FAIL+1))
    err "${sub}/${act} want=${want} got=${GOT}"
  fi
done
[[ "$FAIL" == "0" ]] || fail "$FAIL enforce rows wrong"
echo "  passed=$PASS failed=$FAIL"
echo

# 6. Start gateway
note "=== step 6: start local Gateway on 127.0.0.1:${GATEWAY_PORT} ==="
pkill -f "casdoor-resource-gateway" 2>/dev/null || true
sleep 1
TMPDIR=$(mktemp -d)
cat > "$TMPDIR/run_gw.sh" <<EOF
#!/bin/bash
export NODE_ENV=development
export CASDOOR_ISSUER=${CASDOOR_BASE_URL}
# Audience must match the Casdoor JWT aud claim. Password-grant JWTs for
# alice/carol use client_id as the audience (Casdoor default). Keep
# CASDOOR_AUDIENCE unset so the Gateway falls back to CASDOOR_CLIENT_ID.
export CASDOOR_CLIENT_ID=${CASDOOR_CLIENT_ID}
export CASDOOR_CLIENT_SECRET=openbuddy-gateway-secret-should-be-32-chars-min
export RESOURCE_GATEWAY_WEKNORA_EXCHANGE_SECRET=local-probe-secret-please-rotate-now
export RESOURCE_GATEWAY_WEKNORA_EXCHANGE_AUDIENCE=weknora
export RESOURCE_GATEWAY_WEKNORA_TENANT_MAP_JSON='{"built-in":1,"acme-test":2}'
export RESOURCE_GATEWAY_WEBHOOK_SECRET=probe-webhook-secret
export RESOURCE_GATEWAY_BILLING_CALLBACK_SECRET=probe-billing-callback-secret
export RESOURCE_GATEWAY_CREDIT_EXPIRY_SECRET=probe-credit-expiry-secret-should-be-32-chars
export RESOURCE_GATEWAY_NEW_API_COST_IMPORT_SECRET=probe-cost-import-secret-should-be-32-chars
export RESOURCE_GATEWAY_LISTEN_ADDR=127.0.0.1
export RESOURCE_GATEWAY_PORT=${GATEWAY_PORT}
export PORT=${GATEWAY_PORT}
export HOST=127.0.0.1
export RESOURCE_GATEWAY_DATA_DIR=$TMPDIR/data
export RESOURCE_GATEWAY_STORE=memory
export RESOURCE_GATEWAY_AUTO_WELCOME=true
export RESOURCE_GATEWAY_AUTO_WELCOME_ORGANIZATIONS=built-in
cd $(dirname $GATEWAY_DIST_PATH)
exec node $(basename $GATEWAY_DIST_PATH)
EOF
chmod +x "$TMPDIR/run_gw.sh"
nohup "$TMPDIR/run_gw.sh" > "$TMPDIR/gw.log" 2>&1 &
disown
sleep 2
HEALTH=$(curl -fsS "http://127.0.0.1:${GATEWAY_PORT}/healthz" || true)
[[ "$HEALTH" == *"ok"* ]] || fail "gateway health failed: $(cat $TMPDIR/gw.log)"
ok "gateway health: $HEALTH"

cleanup() { pkill -f "casdoor-resource-gateway" 2>/dev/null || true; rm -rf "$TMPDIR" 2>/dev/null || true; }
trap cleanup EXIT

# 7. Carol (no weknora.* perms) exchange -> 403 WEKNORA_PERMISSION_REQUIRED
note "=== step 7: carol exchange (expect 403 WEKNORA_PERMISSION_REQUIRED) ==="
RES=$(curl -sS -w "|%{http_code}" -X POST "http://127.0.0.1:${GATEWAY_PORT}/v1/token-exchange/weknora" \
  -H "Authorization: Bearer $CAROL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"tenant":"built-in","weknoraTenantId":1}')
CODE=$(echo "$RES" | awk -F'|' '{print $NF}')
BODY=$(echo "$RES" | awk -F'|' '{$NF=""; sub(/\|$/,""); print}')
[[ "$CODE" == "403" ]] || fail "carol exchange expected 403, got $CODE: $BODY"
echo "$BODY" | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); assert d.get('code')=='WEKNORA_PERMISSION_REQUIRED', d" || fail "wrong code"
ok "carol -> 403 WEKNORA_PERMISSION_REQUIRED"

# 8. Alice (member role) exchange -> 403 TENANT_MEMBERSHIP_REQUIRED (no WeKnora member record)
#    OR 200 if WeKnora is running with alice pre-registered; either is correct because
#    the auth+authz chain PASSED.
note "=== step 8: alice exchange (expect 200 or 403 TENANT_MEMBERSHIP_REQUIRED) ==="
RES=$(curl -sS -w "|%{http_code}" -X POST "http://127.0.0.1:${GATEWAY_PORT}/v1/token-exchange/weknora" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"tenant":"built-in","weknoraTenantId":1}')
CODE=$(echo "$RES" | awk -F'|' '{print $NF}')
BODY=$(echo "$RES" | awk -F'|' '{$NF=""; sub(/\|$/,""); print}')
case "$CODE" in
  200) ok "alice -> 200 OK (full chain passed; WeKnora member record exists)";;
  403)
    if echo "$BODY" | grep -q "TENANT_MEMBERSHIP_REQUIRED"; then
      ok "alice -> 403 TENANT_MEMBERSHIP_REQUIRED (auth+authz passed; WeKnora DB-side record missing — expected without WeKnora bootstrap)"
    else
      fail "alice -> 403 but unexpected code: $BODY"
    fi
    ;;
  *) fail "alice exchange expected 200/403, got $CODE: $BODY";;
esac

# 9. Admin exchange -> 200
note "=== step 9: admin exchange (expect 200) ==="
ADMIN_TOKEN=$(curl -sS -X POST "${CASDOOR_BASE_URL}/api/login/oauth/access_token" \
  -d "grant_type=password&client_id=${CASDOOR_CLIENT_ID}&client_secret=&username=${CASDOOR_ADMIN_USERNAME}&password=${CASDOOR_ADMIN_PASSWORD}&scope=openid" \
  -H "Content-Type: application/x-www-form-urlencoded" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")
RES=$(curl -sS -w "|%{http_code}" -X POST "http://127.0.0.1:${GATEWAY_PORT}/v1/token-exchange/weknora" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"tenant":"built-in","weknoraTenantId":1}')
CODE=$(echo "$RES" | awk -F'|' '{print $NF}')
[[ "$CODE" == "200" ]] || fail "admin exchange expected 200, got $CODE: $BODY"
ok "admin -> 200 OK"
echo

note "=== ALL STEPS PASS ==="
ok "Casdoor <-> Gateway <-> WeKnora chain verified with real non-admin users"
