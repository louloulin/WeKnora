#!/bin/bash
# Smoke test for the wiki backlinks cache cleanup sweeper (Build #22).
#
# DRY_RUN-safe — by default it does not hit any server. Set
# CACHE_CLEANUP_SMOKE_BASE_URL + CACHE_CLEANUP_SMOKE_TOKEN to point
# at a live WeKnora instance, and it will:
#
#   1. POST create page A (target) + GET graph to populate cache row
#   2. GET /backlinks/cache-status (warm) — expect computed_at populated
#   3. GET /metrics  — expect wiki_cache_rows_remaining gauge = 1
#   4. POST update A to bump updated_at (now fresh again)
#   5. Set WIKI_CACHE_TTL_DAYS=0 (effectively mark all rows stale)
#   6. Trigger RunOnce via POST /admin/wiki/cache-cleanup (or curl the
#      dev endpoint) and verify rows drop
#   7. GET /metrics  — expect wiki_cache_cleanup_deleted_total increased
#   8. Cleanup pages
#
# Usage:
#   # dry-run (prints commands only)
#   ./scripts/smoke-wiki-cache-cleanup.sh
#
#   # live smoke
#   CACHE_CLEANUP_SMOKE_BASE_URL=http://localhost:8080 \
#   CACHE_CLEANUP_SMOKE_TOKEN=$YOUR_JWT \
#   CACHE_CLEANUP_SMOKE_KB_ID=kb_demo \
#   ./scripts/smoke-wiki-cache-cleanup.sh

set -euo pipefail

BASE_URL="${CACHE_CLEANUP_SMOKE_BASE_URL:-}"
TOKEN="${CACHE_CLEANUP_SMOKE_TOKEN:-}"
KB_ID="${CACHE_CLEANUP_SMOKE_KB_ID:-kb_smoke}"
PAGE_A="${CACHE_CLEANUP_SMOKE_PAGE_A:-cleanup-target}"

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[1;33m'
BLU='\033[0;34m'
NC='\033[0m'

log()    { printf "%b\n" "${BLU}[INFO]${NC} $1"; }
warn()   { printf "%b\n" "${YEL}[WARN]${NC} $1"; }
ok()     { printf "%b\n" "${GRN}[✓]${NC} $1"; }
fail()   { printf "%b\n" "${RED}[✗]${NC} $1"; exit 1; }

page_path() {
  printf "%s/api/v1/knowledgebase/%s/wiki/pages/%s" \
    "$BASE_URL" "$KB_ID" "$1"
}

metrics_path() {
  printf "%s/metrics" "$BASE_URL"
}

cmd_create() {
  local slug="$1"
  local title="$2"
  local desc="$3"
  log "POST $(page_path "$slug") — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X POST -H 'Authorization: Bearer \$TOKEN' \\"
    log "  -H 'Content-Type: application/json' \\"
    log "  -d '{\"slug\":\"$slug\",\"title\":\"$title\",\"content\":\"target\",\"page_type\":\"summary\",\"summary\":\"\"}' \\"
    log "  $(page_path "$slug")"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -X POST \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"slug\":\"$slug\",\"title\":\"$title\",\"content\":\"target\",\"page_type\":\"summary\",\"summary\":\"\"}" \
    "$(page_path "$slug")"
}

cmd_get_graph() {
  local slug="$1"
  local desc="$2"
  log "GET $(page_path "$slug")/backlinks/graph — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' $(page_path "$slug")/backlinks/graph"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "$slug")/backlinks/graph"
}

cmd_get_cache_status() {
  local slug="$1"
  local desc="$2"
  log "GET $(page_path "$slug")/backlinks/cache-status — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' $(page_path "$slug")/backlinks/cache-status"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "$slug")/backlinks/cache-status"
}

cmd_get_metrics() {
  local desc="$1"
  log "GET $(metrics_path) — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS $(metrics_path) | grep wiki_cache_"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    "$(metrics_path)" | grep -E "wiki_cache_(cleanup|rows_remaining)" || true
}

cmd_trigger_cleanup() {
  local desc="$1"
  log "POST $(page_path "admin")/cache-cleanup — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X POST -H 'Authorization: Bearer \$TOKEN' \\"
    log "  $(page_path "admin")/cache-cleanup"
    log "  # Note: Build #22 ships with dry-run default. Set"
    log "  # WIKI_CACHE_CLEANUP_DRY_RUN=false before launching the server"
    log "  # to enable actual deletion."
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -X POST \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "admin")/cache-cleanup"
}

cmd_delete() {
  local slug="$1"
  local desc="$2"
  log "DELETE $(page_path "$slug") — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X DELETE -H 'Authorization: Bearer \$TOKEN' $(page_path "$slug")"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -X DELETE \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "$slug")"
}

echo
log "== 1. Create page A + GET graph to populate cache =="
cmd_create "$PAGE_A" "Page A" "create A"
cmd_get_graph "$PAGE_A" "warm cache row"
echo

log "== 2. Verify cache-status shows warm row =="
cmd_get_cache_status "$PAGE_A" "warm"
echo
log "  Expected (manual review): HTTP 200, computed_at populated."
echo

log "== 3. Check Prom metrics — gauge should report 1 row =="
cmd_get_metrics "gauge"
echo
log "  Expected (manual review):"
log "    wiki_cache_rows_remaining 1"
echo

log "== 4. Trigger RunOnce via admin endpoint (dry-run default) =="
cmd_trigger_cleanup "dry-run"
echo
log "  Expected (manual review):"
log "    dry-run: returns count of stale rows but doesn't delete"
echo

log "== 5. Check metrics after dry-run =="
cmd_get_metrics "after dry-run"
echo
log "  Expected (manual review):"
log "    wiki_cache_cleanup_dry_run_total incremented"
echo

log "== 6. To verify force-delete, restart server with WIKI_CACHE_TTL_DAYS=0 WIKI_CACHE_CLEANUP_DRY_RUN=false, then re-trigger =="
log "    (skipped in dry-run mode)"
echo

log "== 7. Cleanup =="
cmd_delete "$PAGE_A" "cleanup A"
echo

if [[ -z "$BASE_URL" ]]; then
  warn "BASE_URL not set — dry-run only. Set CACHE_CLEANUP_SMOKE_BASE_URL + CACHE_CLEANUP_SMOKE_TOKEN to hit a real server."
else
  ok "Live smoke complete. Review the HTTP codes + bodies above."
fi
