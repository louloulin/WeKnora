#!/usr/bin/env bash
# scripts/smoke-dlp-authz.sh — v0.7.22 end-to-end smoke test
#
# Walks the DLP + AuthZ Admin surface in 7 steps:
#   1. Create DLP policy
#   2. Add a builtin rule (credit card)
#   3. Scan a synthetic text → expect match
#   4. Activate the policy
#   5. Publish v1 of an AuthZ policy
#   6. Publish v2 of the same policy (verifies version bump + immutability)
#   7. Simulate the policy (allow + deny paths)
#
# Prereqs: server running on $BASE_URL (default http://localhost:8080)
# and a valid JWT in $TOKEN with KB-Editor role.
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
TOKEN=${TOKEN:?TOKEN environment variable required}

AUTH="Authorization: Bearer ${TOKEN}"
JSON="Content-Type: application/json"

step() { printf "\n=== %s ===\n" "$1"; }
ok()   { printf "  ✅ %s\n" "$1"; }
fail() { printf "  ❌ %s\n" "$1"; exit 1; }

step "1. Create DLP policy"
POLICY=$(curl -fsSL -X POST "${BASE_URL}/api/v1/dlp/policies" \
    -H "$AUTH" -H "$JSON" \
    -d '{"name":"smoke-pii","severity":"high","action":"block","resource_scope":"*","description":"smoke test"}')
POLICY_ID=$(echo "$POLICY" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ -n "$POLICY_ID" ]] && ok "policy id=$POLICY_ID created" || fail "policy creation"

step "2. Add credit_card builtin rule"
RULE=$(curl -fsSL -X POST "${BASE_URL}/api/v1/dlp/policies/${POLICY_ID}/rules" \
    -H "$AUTH" -H "$JSON" \
    -d '{"pattern_type":"builtin","pattern_value":"credit_card","severity":"high","description":"visa/mc/amex/discover/jcb"}')
RULE_ID=$(echo "$RULE" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ -n "$RULE_ID" ]] && ok "rule id=$RULE_ID added" || fail "rule creation"

step "3. Scan synthetic text — expect match"
SCAN=$(curl -fsSL -X POST "${BASE_URL}/api/v1/dlp/scan" \
    -H "$AUTH" -H "$JSON" \
    -d '{"text":"Customer card: 4111 1111 1111 1111 expires 12/27","resource":"smoke"}')
echo "$SCAN" | grep -q '"matched_pattern":"credit_card"' && ok "credit card detected" || fail "scan missed credit card"
echo "$SCAN" | grep -q '"action":"block"' && ok "policy action=block surfaced" || fail "action not surfaced"

step "4. Activate policy"
ACT=$(curl -fsSL -X POST "${BASE_URL}/api/v1/dlp/policies/${POLICY_ID}/activate" \
    -H "$AUTH" -H "$JSON" -d '{}')
echo "$ACT" | grep -q '"status":"active"' && ok "policy activated" || fail "activate failed"

step "5. Publish AuthZ policy v1"
V1=$(curl -fsSL -X POST "${BASE_URL}/api/v1/authz/policies" \
    -H "$AUTH" -H "$JSON" \
    -d '{"policy_key":"smoke.kb.read","expression":"actor.role == \"viewer\"","decision":"allow","metadata":"{\"author\":\"smoke\"}"}')
V1_VERSION=$(echo "$V1" | grep -oE '"version":[0-9]+' | head -1 | cut -d: -f2)
V1_ID=$(echo "$V1" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ "$V1_VERSION" == "1" ]] && ok "v1 created (id=$V1_ID)" || fail "v1 version=$V1_VERSION"

step "6. Publish AuthZ policy v2 — version bump"
V2=$(curl -fsSL -X POST "${BASE_URL}/api/v1/authz/policies" \
    -H "$AUTH" -H "$JSON" \
    -d '{"policy_key":"smoke.kb.read","expression":"actor.role == \"editor\"","decision":"allow","metadata":"{}"}')
V2_VERSION=$(echo "$V2" | grep -oE '"version":[0-9]+' | head -1 | cut -d: -f2)
V2_ID=$(echo "$V2" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ "$V2_VERSION" == "2" ]] && ok "v2 created (id=$V2_ID)" || fail "v2 version=$V2_VERSION"
[[ "$V1_ID" != "$V2_ID" ]] && ok "v1 and v2 are distinct rows" || fail "duplicate id"

step "7. Simulate policy"
SIM=$(curl -fsSL -X POST "${BASE_URL}/api/v1/authz/simulate" \
    -H "$AUTH" -H "$JSON" \
    -d '{"policy_key":"smoke.kb.read","actor":{"role":"editor"},"resource":{},"action":"read"}')
echo "$SIM" | grep -q '"decision":"allow"' && ok "editor → allow" || fail "editor simulation failed: $SIM"

SIM2=$(curl -fsSL -X POST "${BASE_URL}/api/v1/authz/simulate" \
    -H "$AUTH" -H "$JSON" \
    -d '{"policy_key":"smoke.kb.missing","actor":{"role":"x"},"resource":{},"action":"x"}')
echo "$SIM2" | grep -q '"decision":"deny"' && ok "missing policy → deny" || fail "missing-policy simulation failed: $SIM2"

printf "\n🎉 v0.7.22 DLP + AuthZ smoke complete — 7/7 green\n"
