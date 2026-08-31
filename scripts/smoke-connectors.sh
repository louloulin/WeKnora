#!/usr/bin/env bash
# scripts/smoke-connectors.sh - v0.7.24 end-to-end smoke test
#
# Walks the AI Connector framework surface in 7 steps:
#   1. Create Slack connector with sample config
#   2. List connectors - verify it appears
#   3. Trigger Slack connector - expect result_count > 0
#   4. Get jobs for connector
#   5. Create Email connector + trigger
#   6. Create with unknown kind - expect 400
#   7. Delete both + verify
#
# Prereqs: server running on $BASE_URL (default http://localhost:8080)
# and a valid JWT in $TOKEN with KB-Editor role.
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
TOKEN=${TOKEN:?TOKEN environment variable required}

AUTH="Authorization: Bearer ${TOKEN}"
JSON="Content-Type: application/json"

step() { printf "\n=== %s ===\n" "$1"; }
ok()   { printf "  PASS %s\n" "$1"; }
fail() { printf "  FAIL %s\n" "$1"; exit 1; }

step "1. Create Slack connector"
SLACK=$(curl -fsSL -X POST "${BASE_URL}/api/v1/connectors" \
    -H "$AUTH" -H "$JSON" \
    -d '{
      "name":"smoke-slack",
      "kind":"slack",
      "knowledge_base_id":"smoke-kb",
      "config":{
        "channel":"#general",
        "messages":[
          {"ts":"1700000000.000100","user":"U1","text":"hello smoke"},
          {"ts":"1700000060.000200","user":"U2","text":"second message"}
        ]
      }
    }')
SLACK_ID=$(echo "$SLACK" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ -n "$SLACK_ID" ]] && ok "slack connector id=$SLACK_ID created" || fail "slack creation"

step "2. List connectors - verify it appears"
LIST=$(curl -fsSL "${BASE_URL}/api/v1/connectors" -H "$AUTH")
echo "$LIST" | grep -q '"smoke-slack"' && ok "slack connector listed" || fail "slack not in list"
echo "$LIST" | grep -q '"slack"' && ok "kinds list includes slack" || fail "kinds list missing slack"

step "3. Trigger Slack connector - expect result_count > 0"
JOB=$(curl -fsSL -X POST "${BASE_URL}/api/v1/connectors/${SLACK_ID}/trigger" \
    -H "$AUTH" -H "$JSON" -d '{}')
echo "$JOB" | grep -qE '"result_count":[1-9]' && ok "slack trigger ingested >= 1 message" || fail "slack trigger returned 0 results"
echo "$JOB" | grep -q '"status":"succeeded"' && ok "job status=succeeded" || fail "job status not succeeded"

step "4. Get jobs for connector"
JOBS=$(curl -fsSL "${BASE_URL}/api/v1/connectors/${SLACK_ID}/jobs" -H "$AUTH")
echo "$JOBS" | grep -q '"succeeded"' && ok "slack jobs listed" || fail "slack jobs missing"

step "5. Create Email connector + trigger"
EMAIL=$(curl -fsSL -X POST "${BASE_URL}/api/v1/connectors" \
    -H "$AUTH" -H "$JSON" \
    -d '{
      "name":"smoke-email",
      "kind":"email",
      "knowledge_base_id":"smoke-kb",
      "config":{
        "mailbox":"inbox@example.com",
        "messages":[
          {"from":"alice@example.com","subject":"smoke","body":"first","date":"2024-01-01T00:00:00Z"},
          {"from":"bob@example.com","subject":"smoke 2","body":"second","date":"Mon, 01 Jan 2024 00:01:00 +0000"}
        ]
      }
    }')
EMAIL_ID=$(echo "$EMAIL" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ -n "$EMAIL_ID" ]] && ok "email connector id=$EMAIL_ID created" || fail "email creation"

EJOB=$(curl -fsSL -X POST "${BASE_URL}/api/v1/connectors/${EMAIL_ID}/trigger" \
    -H "$AUTH" -H "$JSON" -d '{}')
echo "$EJOB" | grep -qE '"result_count":[1-9]' && ok "email trigger ingested >= 1 message" || fail "email trigger returned 0 results"

step "6. Create with unknown kind - expect 400"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/connectors" \
    -H "$AUTH" -H "$JSON" \
    -d '{"name":"bad","kind":"does-not-exist","knowledge_base_id":"x","config":{}}')
[[ "$HTTP_CODE" == "400" ]] && ok "unknown kind rejected with 400" || fail "unknown kind returned $HTTP_CODE, expected 400"

step "7. Delete both + verify"
curl -fsSL -X DELETE "${BASE_URL}/api/v1/connectors/${SLACK_ID}" -H "$AUTH" >/dev/null
curl -fsSL -X DELETE "${BASE_URL}/api/v1/connectors/${EMAIL_ID}" -H "$AUTH" >/dev/null
LIST2=$(curl -fsSL "${BASE_URL}/api/v1/connectors" -H "$AUTH")
echo "$LIST2" | grep -q '"smoke-slack"' && fail "slack still listed after delete" || ok "slack deleted"
echo "$LIST2" | grep -q '"smoke-email"' && fail "email still listed after delete" || ok "email deleted"

printf "\n=== AI Connector framework smoke test passed ===\n"
