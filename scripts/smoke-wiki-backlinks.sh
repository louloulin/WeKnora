#!/bin/bash
# Smoke test for the wiki page-level backlinks endpoint (Build #11).
#
# This is a *dry-run safe* template — by default it does not hit any
# server. Set BACKLINKS_SMOKE_BASE_URL + BACKLINKS_SMOKE_TOKEN to
# point at a live WeKnora instance, and it will issue real GET
# requests and demonstrate the full create-A / create-B-with-`[[A]]` /
# fetch backlinks / delete-B / re-fetch flow.
#
# Usage:
#   # dry-run (prints commands only)
#   ./scripts/smoke-wiki-backlinks.sh
#
#   # live smoke against a local dev instance
#   BACKLINKS_SMOKE_BASE_URL=http://localhost:8080 \
#   BACKLINKS_SMOKE_TOKEN=$YOUR_JWT \
#   BACKLINKS_SMOKE_KB_ID=kb_demo \
#   BACKLINKS_SMOKE_PAGE_A=summary-intro \
#   BACKLINKS_SMOKE_PAGE_B=summary-glossary \
#   ./scripts/smoke-wiki-backlinks.sh
#
# The script demonstrates:
#   1. POST create page A
#   2. POST create page B with content containing `[[A]]`
#   3. GET  .../wiki/pages/<A>/backlinks  → expect 1 row (slug=B)
#   4. DELETE page B
#   5. GET  .../wiki/pages/<A>/backlinks  → expect 0 rows
#      (server auto-cleans the orphan from `in_links` on delete)

set -euo pipefail

BASE_URL="${BACKLINKS_SMOKE_BASE_URL:-}"
TOKEN="${BACKLINKS_SMOKE_TOKEN:-}"
KB_ID="${BACKLINKS_SMOKE_KB_ID:-kb_smoke}"
PAGE_A="${BACKLINKS_SMOKE_PAGE_A:-summary-intro}"
PAGE_B="${BACKLINKS_SMOKE_PAGE_B:-summary-glossary}"

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

create_page() {
  local slug="$1"
  local title="$2"
  local content="$3"
  printf '{"slug":"%s","title":"%s","content":%s,"page_type":"summary","summary":""}' \
    "$slug" "$title" "$(printf '%s' "$content" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read()))')"
}

cmd_get_backlinks() {
  local slug="$1"
  log "GET $(page_path "$slug")/backlinks"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' $(page_path "$slug")/backlinks"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "$slug")/backlinks"
}

cmd_create() {
  local slug="$1"
  local title="$2"
  local content="$3"
  local desc="$4"
  log "POST $(page_path "$slug") — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X POST -H 'Authorization: Bearer \$TOKEN' \\"
    log "  -H 'Content-Type: application/json' \\"
    log "  -d '$(create_page "$slug" "$title" "$content")' $(page_path "$slug")"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -X POST \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$(create_page "$slug" "$title" "$content")" \
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

echo
log "== 1. Create page A (target of backlinks) =="
cmd_create "$PAGE_A" "Page A" "A target page" "create A"
echo

log "== 2. Create page B with content linking to A via [[A]] =="
CONTENT_B="This page links to [[${PAGE_A}]] in its body."
cmd_create "$PAGE_B" "Page B" "$CONTENT_B" "create B with [[A]]"
echo

log "== 3. GET A/backlinks — should include B =="
cmd_get_backlinks "$PAGE_A"
echo

log "== 4. DELETE page B (orphan should be auto-cleaned from A.in_links) =="
cmd_delete "$PAGE_B" "delete B"
echo

log "== 5. GET A/backlinks — should now be empty =="
cmd_get_backlinks "$PAGE_A"
echo

log "== 6. Cleanup: DELETE page A =="
cmd_delete "$PAGE_A" "cleanup A"
echo

if [[ -z "$BASE_URL" ]]; then
  warn "BASE_URL not set — dry-run only. Set BACKLINKS_SMOKE_BASE_URL + BACKLINKS_SMOKE_TOKEN to hit a real server."
else
  ok "Live smoke complete. Review the HTTP codes + bodies above."
fi