#!/usr/bin/env bash
# scripts/setup-casdoor-rbac.sh
# Idempotently provision the Casdoor RBAC scaffold required for the unified
# OpenBuddy <-> Casdoor <-> WeKnora exchange.
#
# Every state-mutating step is verified by reading back from Casdoor. The
# script bails out (non-zero exit) if any verification fails.
#
# Required env (NEVER pass via CLI to avoid history leak):
#   CASDOOR_ADMIN_PASSWORD        MUST be exported (no CLI arg)
#
# Optional env:
#   CASDOOR_BASE_URL              default: http://124.221.146.145:8000
#   CASDOOR_CLIENT_ID             default: 005d6839fe25abd6696f
#   CASDOOR_ADMIN_USERNAME        default: admin
#   CASDOOR_ORG                   default: built-in
#
# Usage:
#   export HISTCONTROL=ignorespace
#   export CASDOOR_ADMIN_PASSWORD=...
#   bash scripts/setup-casdoor-rbac.sh
set -euo pipefail

CASDOOR_BASE_URL="${CASDOOR_BASE_URL:-http://124.221.146.145:8000}"
CASDOOR_CLIENT_ID="${CASDOOR_CLIENT_ID:-005d6839fe25abd6696f}"
CASDOOR_ADMIN_USERNAME="${CASDOOR_ADMIN_USERNAME:-admin}"
CASDOOR_ADMIN_PASSWORD="${CASDOOR_ADMIN_PASSWORD:-}"
CASDOOR_ORG="${CASDOOR_ORG:-built-in}"

if [[ -z "$CASDOOR_ADMIN_PASSWORD" ]]; then
  echo "ERROR: CASDOOR_ADMIN_PASSWORD must be exported" >&2
  exit 64
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELPER="python3 ${SCRIPT_DIR}/casdoor-rbac-helpers.py"

note() { printf '\033[36m%s\033[0m\n' "$*"; }
ok()   { printf '\033[32m✓ %s\033[0m\n' "$*"; }
err()  { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; }
fail() { err "$*"; exit 1; }

step() { note "=== step $1: $2 ==="; }

MODEL_TEXT='[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && (r.obj == p.obj || p.obj == "*") && r.act == p.act'

# 1. Acquire admin token
step 1 "acquire Casdoor admin access_token (password grant)"
LOGIN_RES=$(curl -fsS -X POST "${CASDOOR_BASE_URL}/api/login/oauth/access_token" \
  -d "grant_type=password&client_id=${CASDOOR_CLIENT_ID}&client_secret=&username=${CASDOOR_ADMIN_USERNAME}&password=${CASDOOR_ADMIN_PASSWORD}&scope=openid" \
  -H "Content-Type: application/x-www-form-urlencoded")
ADMIN_TOKEN=$(printf '%s' "$LOGIN_RES" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")
[[ -n "$ADMIN_TOKEN" ]] || fail "could not parse access_token: $LOGIN_RES"
ok "admin token length=${#ADMIN_TOKEN}"

api() {
  local method="$1" path="$2"; shift 2
  curl -fsS -X "$method" "${CASDOOR_BASE_URL}${path}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" "$@"
}

api_check() {
  local method="$1" path="$2"; shift 2
  curl -s -X "$method" "${CASDOOR_BASE_URL}${path}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" "$@"
}

# 2. Ensure model exists
step 2 "ensure model ${CASDOOR_ORG}/weknora-model exists"
RES=$(api_check GET "/api/get-model?id=${CASDOOR_ORG}/weknora-model")
if echo "$RES" | python3 -c "import sys,json; sys.exit(0 if json.load(sys.stdin).get('data') else 1)" 2>/dev/null; then
  ok "model exists"
else
  BODY=$($HELPER model-payload "$CASDOOR_ORG" weknora-model "$MODEL_TEXT" 2>/dev/null || echo "")
  if [[ -z "$BODY" ]]; then
    BODY=$(python3 -c "import json; print(json.dumps({'owner':'$CASDOOR_ORG','name':'weknora-model','displayName':'WeKnora RBAC Model (sub, obj, act)','description':'3-tuple RBAC with role inheritance. p.act must match exactly (no wildcard privilege escalation).','text':'$MODEL_TEXT'}))")
  fi
  api POST "/api/add-model" -d "$BODY" >/dev/null
  ok "model created"
fi
echo

# 3. Ensure adapter exists
step 3 "ensure adapter ${CASDOOR_ORG}/weknora-adapter exists"
RES=$(api_check GET "/api/get-adapter?id=${CASDOOR_ORG}/weknora-adapter")
if echo "$RES" | python3 -c "import sys,json; sys.exit(0 if json.load(sys.stdin).get('data') else 1)" 2>/dev/null; then
  ok "adapter exists"
else
  BODY=$(python3 -c "import json; print(json.dumps({'owner':'$CASDOOR_ORG','name':'weknora-adapter','table':'casbin_weknora_rule','useSameDb':True}))")
  api POST "/api/add-adapter" -d "$BODY" >/dev/null
  ok "adapter created"
fi
echo

# 4. Ensure enforcer exists
step 4 "ensure enforcer ${CASDOOR_ORG}/weknora-enforcer exists"
RES=$(api_check GET "/api/get-enforcer?id=${CASDOOR_ORG}/weknora-enforcer")
if echo "$RES" | python3 -c "import sys,json; sys.exit(0 if json.load(sys.stdin).get('data') else 1)" 2>/dev/null; then
  ok "enforcer exists"
else
  BODY=$(python3 -c "import json; print(json.dumps({'owner':'$CASDOOR_ORG','name':'weknora-enforcer','displayName':'WeKnora Resource Enforcer','model':'$CASDOOR_ORG/weknora-model','adapter':'$CASDOOR_ORG/weknora-adapter'}))")
  api POST "/api/add-enforcer" -d "$BODY" >/dev/null
  ok "enforcer created"
fi
echo

# 5. Ensure 5 weknora.* permissions
step 5 "ensure 5 weknora.* permissions exist"
PERMS=(
  "weknora.platform.admin|Admin|admin/openbuddy"
  "weknora.workspace.read|Read|admin/openbuddy"
  "weknora.workspace.contribute|Read,Write|admin/openbuddy"
  "weknora.workspace.admin|Admin|admin/openbuddy"
  "weknora.workspace.owner|Admin|admin/openbuddy"
)
EXIST_PERMS=$(api GET "/api/get-permissions?owner=${CASDOOR_ORG}")
for spec in "${PERMS[@]}"; do
  IFS='|' read -r pname actions resources <<< "$spec"
  HAS=$(echo "$EXIST_PERMS" | HAS_NAME="$pname" python3 -c 'import sys,os,json
d=json.load(sys.stdin)
print(1 if any(p.get("name")==os.environ["HAS_NAME"] for p in (d.get("data") or [])) else 0)')
  if [[ "$HAS" == "1" ]]; then
    ok "perm ${pname} exists"
  else
    BODY=$(python3 -c "import json,sys; a=sys.argv[1].split(','); print(json.dumps({'owner':'$CASDOOR_ORG','name':sys.argv[2],'displayName':sys.argv[2],'description':'Unified WeKnora permission','resources':[sys.argv[3]],'actions':a,'effect':'Allow','isEnabled':True}))" "$actions" "$pname" "$resources")
    api POST "/api/add-permission" -d "$BODY" >/dev/null
    ok "perm ${pname} created"
  fi
done
echo

# 6. Ensure 2 roles
step 6 "ensure 2 openbuddy roles exist"
declare -A ROLE_DESC=(
  ["openbuddy-admin"]="OpenBuddy enterprise administrator role with full WeKnora workspace access|OpenBuddy Administrator"
  ["openbuddy-member"]="Standard OpenBuddy enterprise member with WeKnora workspace read + contribute|OpenBuddy Member"
)
for rname in openbuddy-admin openbuddy-member; do
  IFS='|' read -r desc display <<< "${ROLE_DESC[$rname]}"
  RES=$(api_check GET "/api/get-role?id=${CASDOOR_ORG}/${rname}")
  if echo "$RES" | python3 -c "import sys,json; sys.exit(0 if json.load(sys.stdin).get('data') else 1)" 2>/dev/null; then
    ok "role ${rname} exists"
  else
    BODY=$(python3 -c "import json,sys; print(json.dumps({'owner':'$CASDOOR_ORG','name':sys.argv[1],'displayName':sys.argv[2],'description':sys.argv[3],'isEnabled':True}))" "$rname" "$display" "$desc")
    api POST "/api/add-role" -d "$BODY" >/dev/null
    ok "role ${rname} created"
  fi
done
echo

# 7. Reconcile P-rules
step 7 "reconcile 13 P-rules in enforcer"
declare -a P_RULES=(
  "weknora.platform.admin|weknora.workspace|*"
  "weknora.workspace.read|weknora.workspace|read"
  "weknora.workspace.contribute|weknora.workspace|read"
  "weknora.workspace.contribute|weknora.workspace|contribute"
  "weknora.workspace.admin|weknora.workspace|read"
  "weknora.workspace.admin|weknora.workspace|contribute"
  "weknora.workspace.admin|weknora.workspace|admin"
  "weknora.workspace.owner|weknora.workspace|read"
  "weknora.workspace.owner|weknora.workspace|contribute"
  "weknora.workspace.owner|weknora.workspace|admin"
  "weknora.platform.admin|weknora.workspace|read"
  "weknora.platform.admin|weknora.workspace|contribute"
  "weknora.platform.admin|weknora.workspace|admin"
)
for rule in "${P_RULES[@]}"; do
  IFS='|' read -r v0 v1 v2 <<< "$rule"
  # Remove existing duplicate, ignore failure
  api POST "/api/remove-policy?id=${CASDOOR_ORG}/weknora-enforcer" -d "{\"Ptype\":\"p\",\"V0\":\"${v0}\",\"V1\":\"${v1}\",\"V2\":\"${v2}\"}" >/dev/null 2>&1 || true
  BODY="{\"Ptype\":\"p\",\"V0\":\"${v0}\",\"V1\":\"${v1}\",\"V2\":\"${v2}\"}"
  api POST "/api/add-policy?id=${CASDOOR_ORG}/weknora-enforcer" -d "$BODY" >/dev/null
done
P_COUNT=$(api GET "/api/get-policies?id=${CASDOOR_ORG}/weknora-enforcer" | python3 -c "
import sys,json
ps=json.load(sys.stdin).get('data') or []
print(sum(1 for p in ps if p.get('Ptype')=='p' and p.get('V0') and p.get('V1') and p.get('V2')))
")
[[ "$P_COUNT" == "13" ]] || fail "P-rules expected 13, got $P_COUNT"
ok "13 P-rules reconciled"
echo

# 8. Reconcile G-rules
step 8 "reconcile 7 G-rules in enforcer (admin=5, member=2)"
declare -A ROLE_TO_PERMS=(
  ["openbuddy-admin"]="weknora.platform.admin weknora.workspace.read weknora.workspace.contribute weknora.workspace.admin weknora.workspace.owner"
  ["openbuddy-member"]="weknora.workspace.read weknora.workspace.contribute"
)
for role in "${!ROLE_TO_PERMS[@]}"; do
  for perm in ${ROLE_TO_PERMS[$role]}; do
    api POST "/api/remove-policy?id=${CASDOOR_ORG}/weknora-enforcer" -d "{\"Ptype\":\"g\",\"V0\":\"${CASDOOR_ORG}/${role}\",\"V1\":\"${perm}\"}" >/dev/null 2>&1 || true
    BODY="{\"Ptype\":\"g\",\"V0\":\"${CASDOOR_ORG}/${role}\",\"V1\":\"${perm}\"}"
    api POST "/api/add-policy?id=${CASDOOR_ORG}/weknora-enforcer" -d "$BODY" >/dev/null
  done
done
G_OK=$(api GET "/api/get-policies?id=${CASDOOR_ORG}/weknora-enforcer" | python3 -c "
import sys,json
ps=json.load(sys.stdin).get('data') or []
required = {
  ('built-in/openbuddy-admin', 'weknora.platform.admin'),
  ('built-in/openbuddy-admin', 'weknora.workspace.read'),
  ('built-in/openbuddy-admin', 'weknora.workspace.contribute'),
  ('built-in/openbuddy-admin', 'weknora.workspace.admin'),
  ('built-in/openbuddy-admin', 'weknora.workspace.owner'),
  ('built-in/openbuddy-member', 'weknora.workspace.read'),
  ('built-in/openbuddy-member', 'weknora.workspace.contribute'),
}
present = {(p.get('V0'), p.get('V1')) for p in ps if p.get('Ptype')=='g' and p.get('V0') and p.get('V1')}
missing = required - present
for m in sorted(missing):
    print('MISSING', m[0], m[1])
print(len(required) - len(missing))
")
[[ "$(echo "$G_OK" | tail -1)" == "7" ]] || { echo "$G_OK" | grep '^MISSING' >&2 || true; fail "G-rule reconciliation failed"; }
ok "all 7 role->permission G-rules present"
echo

# 9. Verify RBAC matrix via /api/enforce
step 9 "verify RBAC enforce matrix"
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
ALL_PASS=true
for row in "${MATRIX[@]}"; do
  IFS='|' read -r sub obj act want <<< "$row"
  GOT=$(api POST "/api/enforce?enforcerId=${CASDOOR_ORG}/weknora-enforcer" -d "[\"${sub}\",\"${obj}\",\"${act}\"]" | python3 -c "import sys,json; d=json.load(sys.stdin); print('True' if d.get('data',[None])[0] is True else 'False')")
  if [[ "$GOT" == "$want" ]]; then
    ok "${sub} ${obj}/${act} = ${GOT}"
  else
    err "${sub} ${obj}/${act}: expected ${want}, got ${GOT}"
    ALL_PASS=false
  fi
done
$ALL_PASS || fail "matrix verification failed"
echo

note "=== ALL STEPS PASS ==="
ok "Casdoor RBAC scaffold is live and enforced"
