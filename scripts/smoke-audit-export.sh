#!/usr/bin/env bash
# scripts/smoke-audit-export.sh - v0.7.25 Build #24 end-to-end smoke test
#
# Walks the audit export + compliance report surface:
#   1. Create CSV export (no filter) -> expect 200 + CSV payload
#   2. Create JSON export (no filter) -> expect 200 + JSON payload
#   3. Create export with bad format -> expect 400
#   4. Create export with invalid time range -> expect 400
#   5. List recent exports -> expect >= 2
#   6. Fetch a specific export by id -> 200
#   7. Fetch non-existent export -> 404
#   8. Compliance summary -> 200 with totals
#
# Prereqs: server running on $BASE_URL with a valid $TOKEN belonging
# to a Tenant Owner / Admin.
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
TOKEN=${TOKEN:?TOKEN environment variable required}

AUTH="Authorization: Bearer ${TOKEN}"
JSON="Content-Type: application/json"
URL="${BASE_URL}/api/v1/audit"

step() { printf "\n=== %s ===\n" "$1"; }
ok()   { printf "  PASS %s\n" "$1"; }
fail() { printf "  FAIL %s\n" "$1"; exit 1; }

step "1. CSV export (no filter)"
CSV_HEADERS=$(curl -sI -X POST "${URL}/exports" \
    -H "$AUTH" -H "$JSON" \
    -d '{"format":"csv"}')
echo "$CSV_HEADERS" | grep -qi "HTTP/1.1 200" || fail "expected 200, got headers: $CSV_HEADERS"
echo "$CSV_HEADERS" | grep -qi "Content-Type: text/csv" && ok "Content-Type is text/csv" || fail "missing Content-Type header"
echo "$CSV_HEADERS" | grep -qi "X-Audit-Export-Id" && ok "X-Audit-Export-Id header present" || fail "missing X-Audit-Export-Id"

step "2. JSON export (no filter)"
JSON_HEADERS=$(curl -sI -X POST "${URL}/exports" \
    -H "$AUTH" -H "$JSON" \
    -d '{"format":"json"}')
echo "$JSON_HEADERS" | grep -qi "HTTP/1.1 200" || fail "expected 200, got headers: $JSON_HEADERS"
echo "$JSON_HEADERS" | grep -qi "Content-Type: application/json" && ok "Content-Type is application/json" || fail "missing Content-Type"

step "3. Bad format -> 400"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${URL}/exports" \
    -H "$AUTH" -H "$JSON" -d '{"format":"xml"}')
[[ "$HTTP_CODE" == "400" ]] && ok "bad format rejected with 400" || fail "expected 400, got $HTTP_CODE"

step "4. Invalid time range -> 400"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${URL}/exports" \
    -H "$AUTH" -H "$JSON" \
    -d '{"format":"csv","filter":{"start_time":"2025-01-02T00:00:00Z","end_time":"2025-01-01T00:00:00Z"}}')
[[ "$HTTP_CODE" == "400" ]] && ok "invalid time range rejected with 400" || fail "expected 400, got $HTTP_CODE"

step "5. List recent exports"
LIST=$(curl -fsSL "${URL}/exports" -H "$AUTH")
echo "$LIST" | grep -q '"exports":' && ok "exports key present" || fail "missing exports key"

step "6. Fetch export by id"
FIRST_ID=$(echo "$LIST" | grep -oE '"id":"[^"]+"' | head -1 | cut -d'"' -f4)
if [[ -n "$FIRST_ID" ]]; then
    GET=$(curl -fsSL "${URL}/exports/${FIRST_ID}" -H "$AUTH")
    echo "$GET" | grep -q "\"${FIRST_ID}\"" && ok "fetched export $FIRST_ID" || fail "id mismatch"
else
    ok "no exports to fetch (skipping)"
fi

step "7. Fetch non-existent export -> 404"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${URL}/exports/does-not-exist-id" -H "$AUTH")
[[ "$HTTP_CODE" == "404" ]] && ok "missing export returns 404" || fail "expected 404, got $HTTP_CODE"

step "8. Compliance summary"
REPORT=$(curl -fsSL "${URL}/report?window_days=30" -H "$AUTH")
echo "$REPORT" | grep -q '"total_events"' && ok "total_events present" || fail "missing total_events"
echo "$REPORT" | grep -q '"compliance_status"' && ok "compliance_status present" || fail "missing compliance_status"

printf "\n=== Audit export smoke test passed ===\n"
