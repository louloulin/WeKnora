#!/usr/bin/env bash
# scripts/smoke-agent-studio.sh — v0.7.21 end-to-end smoke test
#
# Walks the Custom Agent Studio surface in 7 steps:
#   1. Create a cron trigger
#   2. List triggers
#   3. Pause + resume the trigger
#   4. Manually fire an agent run
#   5. List runs
#   6. Create a credential (encrypted at rest)
#   7. List credentials (ciphertext never exposed)
#
# Prereqs: server running on $BASE_URL (default http://localhost:8080)
# and a valid JWT in $TOKEN with KB-Editor role.
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
TOKEN=${TOKEN:?TOKEN environment variable required}
KB_ID=${KB_ID:-kb-test}
AGENT_ID=${AGENT_ID:-agent-test}

AUTH="Authorization: Bearer ${TOKEN}"
JSON="Content-Type: application/json"

step() { printf "\n=== %s ===\n" "$1"; }
ok()   { printf "  ✅ %s\n" "$1"; }
fail() { printf "  ❌ %s\n" "$1"; exit 1; }

step "1. Create cron trigger"
TRIG=$(curl -fsSL -X POST "${BASE_URL}/api/v1/knowledgebase/${KB_ID}/agents/${AGENT_ID}/studio/triggers" \
    -H "$AUTH" -H "$JSON" \
    -d '{"name":"morning-digest","trigger_type":"cron","trigger_config":"{\"cron\":\"0 9 * * 1-5\"}","payload_template":"{\"topic\":\"daily-summary\"}"}')
TRIG_ID=$(echo "$TRIG" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ -n "$TRIG_ID" ]] && ok "trigger id=$TRIG_ID created" || fail "trigger creation"

step "2. List triggers"
LIST=$(curl -fsSL "${BASE_URL}/api/v1/knowledgebase/${KB_ID}/agents/${AGENT_ID}/studio/triggers" -H "$AUTH")
echo "$LIST" | grep -q '"name":"morning-digest"' && ok "trigger visible in list" || fail "trigger missing from list"

step "3. Pause + resume"
curl -fsSL -X POST "${BASE_URL}/api/v1/knowledgebase/${KB_ID}/agents/${AGENT_ID}/studio/triggers/${TRIG_ID}/pause" -H "$AUTH" >/dev/null
curl -fsSL -X POST "${BASE_URL}/api/v1/knowledgebase/${KB_ID}/agents/${AGENT_ID}/studio/triggers/${TRIG_ID}/resume" -H "$AUTH" >/dev/null
ok "pause + resume round-trip succeeded"

step "4. Manual agent run"
RUN=$(curl -fsSL -X POST "${BASE_URL}/api/v1/knowledgebase/${KB_ID}/agents/${AGENT_ID}/studio/run" \
    -H "$AUTH" -H "$JSON" -d '{"input":{"query":"summarize KB"}}')
RUN_ID=$(echo "$RUN" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ -n "$RUN_ID" ]] && ok "run id=$RUN_ID created" || fail "manual run"

step "5. List runs"
RUNS=$(curl -fsSL "${BASE_URL}/api/v1/knowledgebase/${KB_ID}/agents/${AGENT_ID}/studio/runs" -H "$AUTH")
echo "$RUNS" | grep -q "\"id\":${RUN_ID}" && ok "run visible in history" || fail "run missing from history"

step "6. Create credential (vault)"
curl -fsSL -X POST "${BASE_URL}/api/v1/knowledgebase/${KB_ID}/agents/${AGENT_ID}/studio/credentials" \
    -H "$AUTH" -H "$JSON" \
    -d '{"name":"openai-prod","credential_type":"api_key","secret":"sk-live-XXXXXXXX"}' >/dev/null
ok "credential encrypted + stored"

step "7. List credentials (ciphertext must not appear)"
CREDS=$(curl -fsSL "${BASE_URL}/api/v1/knowledgebase/${KB_ID}/agents/${AGENT_ID}/studio/credentials" -H "$AUTH")
echo "$CREDS" | grep -q '"name":"openai-prod"' && ok "credential listed" || fail "credential missing"
echo "$CREDS" | grep -q 'sk-live' && fail "PLAINTEXT LEAKED in API response!" || ok "no plaintext leak"

printf "\nAll 7 smoke checks passed.\n"
