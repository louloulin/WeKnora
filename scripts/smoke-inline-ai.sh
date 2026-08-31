#!/usr/bin/env bash
# scripts/smoke-inline-ai.sh - v0.7.25 Build #23 end-to-end smoke test
#
# Walks the paragraph-level AI surface (6 actions):
#   1. Summarize: short input -> non-empty result
#   2. Translate: English -> Chinese
#   3. Rewrite: original text -> shorter rewrite
#   4. Explain: short input -> explanation with TL;DR
#   5. Extract tasks: prose -> bulleted task list
#   6. Generate table: prose -> markdown table
#   7. Bad input: empty text -> 400
#   8. Unknown action -> 400
#   9. Too long (>16 KB) -> 400
#
# Prereqs: server running on $BASE_URL with a valid $TOKEN and at least
# one knowledge-QA model configured for the tenant.
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
TOKEN=${TOKEN:?TOKEN environment variable required}

AUTH="Authorization: Bearer ${TOKEN}"
JSON="Content-Type: application/json"
URL="${BASE_URL}/api/v1/ai/inline"

step() { printf "\n=== %s ===\n" "$1"; }
ok()   { printf "  PASS %s\n" "$1"; }
fail() { printf "  FAIL %s\n" "$1"; exit 1; }

call_inline() {
    local body="$1"
    curl -fsSL -X POST "$URL" -H "$AUTH" -H "$JSON" -d "$body"
}

expect_nonempty() {
    local payload="$1" expected_field="$2"
    echo "$payload" | grep -q "\"${expected_field}\":\"[^\"]" || fail "expected non-empty ${expected_field} in $payload"
}

step "1. Summarize"
R1=$(call_inline '{"action":"summarize","text":"WeKnora is a RAG-first enterprise knowledge platform with multi-tenant IAM, AuthZ, wiki, agents, and connectors. It integrates with Notion, Feishu, Slack, and 10+ IM platforms. The recent v0.7.24 release added an AI Connector framework for Slack/Email/Webhook/RSS/Confluence/Notion/Jira ingest."}')
expect_nonempty "$R1" "result"
ok "summarize returned non-empty result"

step "2. Translate English -> Chinese"
R2=$(call_inline '{"action":"translate","text":"WeKnora is a RAG-first platform.","target_language":"Chinese"}')
expect_nonempty "$R2" "result"
ok "translate returned non-empty result"

step "3. Rewrite"
R3=$(call_inline '{"action":"rewrite","text":"This is text that is perhaps a bit wordy and could probably be tightened up and made cleaner and easier to read."}')
expect_nonempty "$R3" "result"
ok "rewrite returned non-empty result"

step "4. Explain"
R4=$(call_inline '{"action":"explain","text":"CRDT"}')
expect_nonempty "$R4" "result"
ok "explain returned non-empty result"

step "5. Extract tasks"
R5=$(call_inline '{"action":"extract_task","text":"We need to ship v0.7.25 next Friday. Alice will write the migration. Bob will deploy the staging environment. Carol will write the smoke script."}')
expect_nonempty "$R5" "result"
ok "extract_task returned non-empty result"

step "6. Generate table"
R6=$(call_inline '{"action":"generate_table","text":"Notion 5; Feishu 4; Confluence 3; Guru 3; Loop 4; Tana 3"}')
expect_nonempty "$R6" "result"
ok "generate_table returned non-empty result"

step "7. Bad input - empty text"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$URL" -H "$AUTH" -H "$JSON" -d '{"action":"summarize","text":""}')
[[ "$HTTP_CODE" == "400" ]] && ok "empty text rejected with 400" || fail "expected 400, got $HTTP_CODE"

step "8. Unknown action"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$URL" -H "$AUTH" -H "$JSON" -d '{"action":"hack","text":"x"}')
[[ "$HTTP_CODE" == "400" ]] && ok "unknown action rejected with 400" || fail "expected 400, got $HTTP_CODE"

step "9. Too long (>16 KB)"
LONG=$(python3 -c 'print("x"*17000)' 2>/dev/null || printf 'x%.0s' {1..17000})
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$URL" -H "$AUTH" -H "$JSON" -d "{\"action\":\"summarize\",\"text\":\"$LONG\"}")
[[ "$HTTP_CODE" == "400" ]] && ok "oversized text rejected with 400" || fail "expected 400, got $HTTP_CODE"

printf "\n=== Inline AI smoke test passed ===\n"
