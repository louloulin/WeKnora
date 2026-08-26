#!/bin/bash
# Smoke test for the wiki backlinks graph cache (Build #21).
#
# This is a *dry-run safe* template — by default it does not hit any
# server. Set BACKLINKS_CACHE_SMOKE_BASE_URL + BACKLINKS_CACHE_SMOKE_TOKEN
# to point at a live WeKnora instance, and it will:
#
#   1. POST create page A (target) — cold cache expected (computed_at = null)
#   2. POST create page B with content `[[A]]` → 1-hop direct of A
#   3. GET  .../wiki/pages/<A>/backlinks/cache-status (cold) — expect 200 + computed_at=null
#   4. GET  .../wiki/pages/<A>/backlinks/graph            → warms the cache writeback
#   5. GET  .../wiki/pages/<A>/backlinks/cache-status (warm) — expect computed_at populated
#   6. POST update page A (changes content) → invalidates cache (delete hook fires)
#   7. GET  .../wiki/pages/<A>/backlinks/cache-status (post-update) — expect cold again
#   8. GET  .../wiki/pages/<A>/backlinks                  — Build #11 zero regression
#   9. DELETE pages B, A — cleanup
#
# Usage:
#   # dry-run (prints commands only)
#   ./scripts/smoke-wiki-backlinks-cache.sh
#
#   # live smoke against a local dev instance
#   BACKLINKS_CACHE_SMOKE_BASE_URL=http://localhost:8080 \
#   BACKLINKS_CACHE_SMOKE_TOKEN=$YOUR_JWT \
#   BACKLINKS_CACHE_SMOKE_KB_ID=kb_demo \
#   ./scripts/smoke-wiki-backlinks-cache.sh

set -euo pipefail

BASE_URL="${BACKLINKS_CACHE_SMOKE_BASE_URL:-}"
TOKEN="${BACKLINKS_CACHE_SMOKE_TOKEN:-}"
KB_ID="${BACKLINKS_CACHE_SMOKE_KB_ID:-kb_smoke}"
PAGE_A="${BACKLINKS_CACHE_SMOKE_PAGE_A:-cache-target}"
PAGE_B="${BACKLINKS_CACHE_SMOKE_PAGE_B:-cache-source}"

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

create_body() {
  local slug="$1"
  local title="$2"
  local content="$3"
  local ptype="${4:-summary}"
  printf '{"slug":"%s","title":"%s","content":%s,"page_type":"%s","summary":""}' \
    "$slug" "$title" \
    "$(printf '%s' "$content" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read()))')" \
    "$ptype"
}

cmd_create() {
  local slug="$1"
  local title="$2"
  local content="$3"
  local ptype="${4:-summary}"
  local desc="$5"
  log "POST $(page_path "$slug") — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X POST -H 'Authorization: Bearer \$TOKEN' \\"
    log "  -H 'Content-Type: application/json' \\"
    log "  -d '$(create_body "$slug" "$title" "$content" "$ptype")' \\"
    log "  $(page_path "$slug")"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -X POST \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$(create_body "$slug" "$title" "$content" "$ptype")" \
    "$(page_path "$slug")"
}

cmd_update() {
  local slug="$1"
  local new_content="$2"
  local desc="$3"
  log "PUT $(page_path "$slug") — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X PUT -H 'Authorization: Bearer \$TOKEN' \\"
    log "  -H 'Content-Type: application/json' \\"
    log "  -d '$(create_body "$slug" "Page A (updated)" "$new_content")' \\"
    log "  $(page_path "$slug")"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -X PUT \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$(create_body "$slug" "Page A (updated)" "$new_content")" \
    "$(page_path "$slug")"
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

cmd_get_cache_status() {
  local slug="$1"
  local desc="$2"
  log "GET $(page_path "$slug")/backlinks/cache-status — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' \\"
    log "  $(page_path "$slug")/backlinks/cache-status"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "$slug")/backlinks/cache-status"
}

cmd_get_backlinks_graph() {
  local slug="$1"
  local desc="$2"
  log "GET $(page_path "$slug")/backlinks/graph — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' \\"
    log "  $(page_path "$slug")/backlinks/graph?max_indirect=50&max_related=10&jaccard=0.3"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "$slug")/backlinks/graph?max_indirect=50&max_related=10&jaccard=0.3"
}

cmd_get_backlinks() {
  local slug="$1"
  local desc="$2"
  log "GET $(page_path "$slug")/backlinks — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' $(page_path "$slug")/backlinks"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "$slug")/backlinks"
}

echo
log "== 1. Create page A (target) =="
cmd_create "$PAGE_A" "Page A" "Target page" "summary" "create A"
echo

log "== 2. Create page B with [[A]] → 1-hop direct of A =="
CONTENT_B="This page links to [[${PAGE_A}]]."
cmd_create "$PAGE_B" "Page B" "$CONTENT_B" "summary" "create B → 1-hop"
echo

log "== 3. GET A/backlinks/cache-status — expect cold (computed_at = null) =="
cmd_get_cache_status "$PAGE_A" "cold cache after create"
echo
log "  Expected (manual review): HTTP 200, computed_at = null, updated_at = null."
echo

log "== 4. GET A/backlinks/graph — warms the cache row =="
cmd_get_backlinks_graph "$PAGE_A" "trigger cache writeback"
echo

log "== 5. GET A/backlinks/cache-status — expect warm (computed_at populated) =="
cmd_get_cache_status "$PAGE_A" "warm cache after graph read"
echo
log "  Expected (manual review): HTTP 200, computed_at non-null RFC3339 timestamp."
echo

log "== 6. PUT update page A (changes body) — invalidates cache via UpdatePage hook =="
cmd_update "$PAGE_A" "Updated body for A. Still links to nothing." "update A → wipe self ∪ out_links"
echo

log "== 7. GET A/backlinks/cache-status — expect cold again (UpdatePage hook wiped) =="
cmd_get_cache_status "$PAGE_A" "cold cache after update"
echo
log "  Expected (manual review): HTTP 200, computed_at = null."
echo

log "== 8. GET A/backlinks — Build #11 endpoint still works (zero regression) =="
cmd_get_backlinks "$PAGE_A" "Build #11 endpoint sanity"
echo
log "  Expected (manual review): HTTP 200, ≥ 1 row (B)."
echo

log "== 9. Cleanup: DELETE pages B, A =="
cmd_delete "$PAGE_B" "cleanup B"
cmd_delete "$PAGE_A" "cleanup A"
echo

if [[ -z "$BASE_URL" ]]; then
  warn "BASE_URL not set — dry-run only. Set BACKLINKS_CACHE_SMOKE_BASE_URL + BACKLINKS_CACHE_SMOKE_TOKEN to hit a real server."
else
  ok "Live smoke complete. Review the HTTP codes + bodies above."
fi