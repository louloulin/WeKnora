#!/bin/bash
# Smoke test for the wiki backlinks graph endpoint (Build #20).
#
# This is a *dry-run safe* template — by default it does not hit any
# server. Set BACKLINKS_V2_SMOKE_BASE_URL + BACKLINKS_V2_SMOKE_TOKEN
# to point at a live WeKnora instance, and it will:
#
#   1. POST create page A (target)
#   2. POST create page B with content `[[A]]` → 1-hop direct
#   3. POST create page C with content `[[B]]` → 2-hop indirect of A
#   4. POST create page D with content `[[A]] [[missing]]` → broken link
#   5. POST create page E with content `[[A]] [[B]]` → jaccard candidate
#   6. GET  .../wiki/pages/<A>/backlinks/graph → expect 4 sections + stats
#   7. GET  .../wiki/pages/<A>/backlinks      → expect ≥1 row (Build #11 zero regression)
#
# Usage:
#   # dry-run (prints commands only)
#   ./scripts/smoke-wiki-backlinks-graph.sh
#
#   # live smoke against a local dev instance
#   BACKLINKS_V2_SMOKE_BASE_URL=http://localhost:8080 \
#   BACKLINKS_V2_SMOKE_TOKEN=$YOUR_JWT \
#   BACKLINKS_V2_SMOKE_KB_ID=kb_demo \
#   ./scripts/smoke-wiki-backlinks-graph.sh

set -euo pipefail

BASE_URL="${BACKLINKS_V2_SMOKE_BASE_URL:-}"
TOKEN="${BACKLINKS_V2_SMOKE_TOKEN:-}"
KB_ID="${BACKLINKS_V2_SMOKE_KB_ID:-kb_smoke}"
PAGE_A="${BACKLINKS_V2_SMOKE_PAGE_A:-summary-intro}"
PAGE_B="${BACKLINKS_V2_SMOKE_PAGE_B:-summary-glossary}"
PAGE_C="${BACKLINKS_V2_SMOKE_PAGE_C:-summary-faq}"
PAGE_D="${BACKLINKS_V2_SMOKE_PAGE_D:-summary-changelog}"
PAGE_E="${BACKLINKS_V2_SMOKE_PAGE_E:-summary-handbook}"

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

cmd_get_backlinks_graph() {
  local slug="$1"
  log "GET $(page_path "$slug")/backlinks/graph?max_indirect=50&max_related=10&jaccard=0.3"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' \\"
    log "  '$(page_path "$slug")/backlinks/graph?max_indirect=50&max_related=10&jaccard=0.3'"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "$slug")/backlinks/graph?max_indirect=50&max_related=10&jaccard=0.3"
}

cmd_get_backlinks() {
  local slug="$1"
  log "GET $(page_path "$slug")/backlinks  (Build #11 endpoint — zero regression)"
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

log "== 3. Create page C with [[B]] → 2-hop indirect of A (via B) =="
CONTENT_C="This page links to [[${PAGE_B}]] (which links to [[${PAGE_A}]])."
cmd_create "$PAGE_C" "Page C" "$CONTENT_C" "summary" "create C → 2-hop"
echo

log "== 4. Create page D with [[missing-slug]] → broken link in A =="
CONTENT_D="This page links to [[does-not-exist-anymore]] and [[${PAGE_A}]]."
cmd_create "$PAGE_D" "Page D" "$CONTENT_D" "summary" "create D → broken + direct"
echo

log "== 5. Create page E with [[A]] [[B]] → jaccard candidate =="
CONTENT_E="This page links to [[${PAGE_A}]] and [[${PAGE_B}]] for jaccard overlap."
cmd_create "$PAGE_E" "Page E" "$CONTENT_E" "summary" "create E → jaccard"
echo

log "== 6. GET A/backlinks/graph — should return 4 sections + stats =="
cmd_get_backlinks_graph "$PAGE_A"
echo
log "  Expected (manual review of body):"
log "    direct   ≥ 2 (B, D, E)"
log "    indirect ≥ 1 (C via B)"
log "    related  ≥ 1 (E if jaccard >= 0.3)"
log "    broken   = []  (A itself has no broken links)"
log "    stats.out_link_count >= 1"
echo

log "== 7. GET A/backlinks — Build #11 endpoint still works (zero regression) =="
cmd_get_backlinks "$PAGE_A"
echo
log "  Expected: ≥ 1 row (slug of B/D/E)."
echo

log "== 8. GET nonexistent/backlinks/graph — should 404 =="
if [[ -z "$BASE_URL" ]]; then
  log "(dry-run) curl -sS -w 'HTTP %{http_code}\\n' -H 'Authorization: Bearer \$TOKEN' \\"
  log "  $(page_path "this-slug-does-not-exist")/backlinks/graph"
else
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(page_path "this-slug-does-not-exist")/backlinks/graph"
fi
echo

log "== 9. Cleanup: DELETE pages C, D, E, B, A =="
cmd_delete "$PAGE_C" "cleanup C"
cmd_delete "$PAGE_D" "cleanup D"
cmd_delete "$PAGE_E" "cleanup E"
cmd_delete "$PAGE_B" "cleanup B"
cmd_delete "$PAGE_A" "cleanup A"
echo

if [[ -z "$BASE_URL" ]]; then
  warn "BASE_URL not set — dry-run only. Set BACKLINKS_V2_SMOKE_BASE_URL + BACKLINKS_V2_SMOKE_TOKEN to hit a real server."
else
  ok "Live smoke complete. Review the HTTP codes + bodies above."
fi