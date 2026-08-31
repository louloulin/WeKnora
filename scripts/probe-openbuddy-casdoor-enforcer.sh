#!/usr/bin/env bash
# scripts/probe-openbuddy-casdoor-enforcer.sh
# Exercises the EXACT HTTP request that OpenBuddy's authorizeResourceRemotely()
# makes to Casdoor /api/enforce. This is what OpenBuddy should be doing in
# production (instead of mocked fetch in the unit test):
#
#   POST {issuer}/api/enforce?enforcerId={enforcerId}
#   Authorization: Basic base64(clientId:clientSecret)
#   body: JSON array [subject, object, action]
#
# Required env:
#   CASDOOR_ADMIN_PASSWORD
# Optional env:
#   CASDOOR_BASE_URL       default: http://124.221.146.145:8000
#   CASDOOR_CLIENT_ID      default: 005d6839fe25abd6696f
#   CASDOOR_CLIENT_SECRET  default: "" (built-in admin app uses empty secret)
#
# Usage:
#   export HISTCONTROL=ignorespace
#   export CASDOOR_ADMIN_PASSWORD=123
#   bash scripts/probe-openbuddy-casdoor-enforcer.sh
set -euo pipefail

CASDOOR_BASE_URL="${CASDOOR_BASE_URL:-http://124.221.146.145:8000}"
CASDOOR_CLIENT_ID="${OPENBUDDY_CASDOOR_CLIENT_ID:-${CASDOOR_CLIENT_ID:-openbuddy-gateway}}"
CASDOOR_CLIENT_SECRET="${OPENBUDDY_CASDOOR_CLIENT_SECRET:-${CASDOOR_CLIENT_SECRET:-openbuddy-gateway-secret-should-be-32-chars-min}}"
CASDOOR_ENFORCER_ID="${CASDOOR_ENFORCER_ID:-built-in/weknora-enforcer}"
CASDOOR_ADMIN_PASSWORD="${CASDOOR_ADMIN_PASSWORD:-}"
[[ -z "$CASDOOR_ADMIN_PASSWORD" ]] && { echo "ERROR: CASDOOR_ADMIN_PASSWORD must be exported" >&2; exit 64; }

note() { printf '\033[36m%s\033[0m\n' "$*"; }
ok()   { printf '\033[32m✓ %s\033[0m\n' "$*"; }
err()  { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; }
fail() { err "$*"; exit 1; }

# Acquire admin token (used as the admin client to call Casdoor — OpenBuddy does
# the same with its own clientId/clientSecret via Basic Auth header).
ADMIN_TOKEN=$(curl -fsS -X POST "${CASDOOR_BASE_URL}/api/login/oauth/access_token" \
  -d "grant_type=password&client_id=${CASDOOR_CLIENT_ID}&client_secret=&username=admin&password=${CASDOOR_ADMIN_PASSWORD}&scope=openid" \
  -H "Content-Type: application/x-www-form-urlencoded" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")
[[ -n "$ADMIN_TOKEN" ]] || fail "could not get admin token"

# Build Basic Auth header (this is what OpenBuddy sends per casdoor-auth.ts:660)
BASIC_AUTH=$(printf '%s:%s' "${CASDOOR_CLIENT_ID}" "${CASDOOR_CLIENT_SECRET}" | base64)

# Probe matrix — same checks OpenBuddy's authorizeResourceRemotely would make
declare -a PROBES=(
  "built-in/openbuddy-admin|weknora.workspace|read|True"
  "built-in/openbuddy-admin|weknora.workspace|contribute|True"
  "built-in/openbuddy-admin|weknora.workspace|admin|True"
  "built-in/openbuddy-member|weknora.workspace|read|True"
  "built-in/openbuddy-member|weknora.workspace|contribute|True"
  "built-in/openbuddy-member|weknora.workspace|admin|False"
  "built-in/openbuddy-member|weknora.workspace|delete|False"
  "built-in/some-other-user|weknora.workspace|read|False"
  "built-in/openbuddy-admin|weknora.workspace|write|False"
  "built-in/openbuddy-member|weknora.workspace|write|False"
  "built-in/openbuddy-admin|weknora.billing|read|False"
)

PASS=0; FAIL=0
note "=== OpenBuddy-style /api/enforce probes (Basic Auth, body=[sub,obj,act]) ==="
for row in "${PROBES[@]}"; do
  IFS='|' read -r sub obj act want <<< "$row"
  RESP=$(curl -sS -X POST "${CASDOOR_BASE_URL}/api/enforce?enforcerId=${CASDOOR_ENFORCER_ID}" \
    -H "Authorization: Basic ${BASIC_AUTH}" \
    -H "Content-Type: application/json" \
    -d "[\"${sub}\",\"${obj}\",\"${act}\"]")
  ALLOWED=$(echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print('True' if d.get('data',[None])[0] is True else 'False')")
  STATUS=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status'))")
  if [[ "$STATUS" != "ok" ]]; then
    err "${sub}/${obj}/${act} HTTP error: $RESP"
    FAIL=$((FAIL+1))
    continue
  fi
  if [[ "$ALLOWED" == "$want" ]]; then
    PASS=$((PASS+1))
    ok "${sub}/${obj}/${act} = ${ALLOWED}"
  else
    FAIL=$((FAIL+1))
    err "${sub}/${obj}/${act} want=${want} got=${ALLOWED}"
  fi
done
echo
echo "passed=$PASS failed=$FAIL"

# Probe error path — bad enforcerId
note "=== Probe: bad enforcerId ==="
RESP=$(curl -sS -X POST "${CASDOOR_BASE_URL}/api/enforce?enforcerId=built-in/nonexistent" \
  -H "Authorization: Basic ${BASIC_AUTH}" \
  -H "Content-Type: application/json" \
  -d '["openbuddy-admin","workspace","read"]')
echo "  $RESP"
echo "$RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); sys.exit(0 if d.get('status')=='error' else 1)" || fail "expected error"
ok "bad enforcerId returns status=error"

note "=== ALL PROBES COMPLETE ==="
[[ "$FAIL" == "0" ]] || fail "$FAIL probes failed"
ok "OpenBuddy's enforcer integration path is verified against live Casdoor"
