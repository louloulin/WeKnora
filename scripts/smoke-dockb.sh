#!/usr/bin/env bash
# scripts/smoke-dockb.sh — v0.7.23 end-to-end smoke test
#
# Walks the Doc ↔ KB AI Bridge + WeKnora Base / Database surface in 7 steps:
#   1. Upsert a doc-kb summary for a chunk
#   2. Get the summary by (knowledge, chunk)
#   3. List summaries for the knowledge entry
#   4. Create a database with a 5-field schema
#   5. Insert a row validating all 5 types
#   6. Insert a row that fails validation (wrong type) — expect 400
#   7. List rows + delete
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

KB_ID=${KB_ID:-kb-smoke-001}
CHUNK_ID=${CHUNK_ID:-chunk-smoke-001}

step "1. Upsert doc-kb summary"
S=$(curl -fsSL -X PUT "${BASE_URL}/api/v1/dockb/chunks/${KB_ID}/${CHUNK_ID}" \
    -H "$AUTH" -H "$JSON" \
    -d '{"text":"Customer onboarding runs through three steps: identity verification, KYC screening, and account provisioning. KYC requires a government ID and a recent utility bill.","model_name":"smoke-noop"}')
echo "$S" | grep -q '"summary"' && ok "summary upserted" || fail "summary missing: $S"

step "2. Get summary by (kb, chunk)"
G=$(curl -fsSL "${BASE_URL}/api/v1/dockb/chunks/${KB_ID}/${CHUNK_ID}" -H "$AUTH")
echo "$G" | grep -q '"model_name":"smoke-noop"' && ok "summary roundtrip" || fail "summary mismatch: $G"

step "3. List summaries for knowledge"
L=$(curl -fsSL "${BASE_URL}/api/v1/dockb/summaries/${KB_ID}" -H "$AUTH")
echo "$L" | grep -q '"total":' && ok "summary list returned" || fail "list failed: $L"

step "4. Create database with 5-field schema"
DB=$(curl -fsSL -X POST "${BASE_URL}/api/v1/databases" \
    -H "$AUTH" -H "$JSON" \
    -d '{
      "name":"smoke-tasks",
      "description":"smoke test db",
      "schema":[
        {"name":"title","type":"text"},
        {"name":"score","type":"number"},
        {"name":"done","type":"checkbox"},
        {"name":"due","type":"date"},
        {"name":"priority","type":"select","options":["low","med","high"]}
      ]
    }')
DB_ID=$(echo "$DB" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ -n "$DB_ID" ]] && ok "database id=$DB_ID created" || fail "db create failed: $DB"

step "5. Insert row validating all 5 types"
ROW=$(curl -fsSL -X POST "${BASE_URL}/api/v1/databases/${DB_ID}/rows" \
    -H "$AUTH" -H "$JSON" \
    -d '{"values":{"title":"onboard acme","score":7.5,"done":false,"due":"2026-09-15","priority":"high"}}')
ROW_ID=$(echo "$ROW" | grep -oE '"id":[0-9]+' | head -1 | cut -d: -f2)
[[ -n "$ROW_ID" ]] && ok "row id=$ROW_ID inserted" || fail "row insert failed: $ROW"

step "6. Insert row with wrong type — expect 400"
HTTP=$(curl -s -o /tmp/wrong.json -w "%{http_code}" -X POST "${BASE_URL}/api/v1/databases/${DB_ID}/rows" \
    -H "$AUTH" -H "$JSON" \
    -d '{"values":{"score":"not-a-number"}}')
[[ "$HTTP" == "400" ]] && ok "wrong-type row rejected with 400" || fail "expected 400, got $HTTP: $(cat /tmp/wrong.json)"

step "7. List rows + delete"
LIST=$(curl -fsSL "${BASE_URL}/api/v1/databases/${DB_ID}/rows" -H "$AUTH")
echo "$LIST" | grep -q "\"id\":${ROW_ID}" && ok "row visible in list" || fail "row missing from list"

curl -fsSL -X DELETE "${BASE_URL}/api/v1/databases/${DB_ID}/rows/${ROW_ID}" -H "$AUTH" >/dev/null
curl -fsSL -X DELETE "${BASE_URL}/api/v1/databases/${DB_ID}" -H "$AUTH" >/dev/null
ok "row + database deleted"

printf "\n🎉 v0.7.23 Doc-KB + Database smoke complete — 7/7 green\n"
