#!/bin/bash
# Smoke test for the wiki page bulk operations endpoints (Build #12).
#
# This is a *dry-run safe* template — by default it does not hit any
# server. Set BATCH_SMOKE_BASE_URL + BATCH_SMOKE_TOKEN to point at a
# live WeKnora instance and it will exercise the three batch endpoints:
#
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-move
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-delete
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-status
#
# Each endpoint is documented to return the partial-success shape:
#
#   { "succeeded": ["..."], "failed": [{ "slug": "...", "code": "...", "error": "..." }] }
#
# Failure tokens currently recognized:
#   not_found | folder_not_found | folder_conflict | folder_not_empty | internal
#
# Usage:
#   # dry-run (prints commands only)
#   ./scripts/smoke-wiki-batch.sh
#
#   # live smoke against a local dev instance
#   BATCH_SMOKE_BASE_URL=http://localhost:8080 \
#   BATCH_SMOKE_TOKEN=$YOUR_JWT \
#   BATCH_SMOKE_KB_ID=kb_smoke \
#   BATCH_SMOKE_PAGE_PREFIX=batch-smoke \
#   ./scripts/smoke-wiki-batch.sh

set -euo pipefail

BASE_URL="${BATCH_SMOKE_BASE_URL:-}"
TOKEN="${BATCH_SMOKE_TOKEN:-}"
KB_ID="${BATCH_SMOKE_KB_ID:-kb_smoke}"
PAGE_PREFIX="${BATCH_SMOKE_PAGE_PREFIX:-batch-smoke}"

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

batch_endpoint() {
  local action="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/pages/batch-%s" \
    "$BASE_URL" "$KB_ID" "$action"
}

create_page_body() {
  local slug="$1"
  printf '{"slug":"%s","title":"%s","content":"smoke","page_type":"summary","summary":""}' \
    "$slug" "$slug"
}

cmd_post() {
  local action="$1"
  local body="$2"
  local desc="$3"
  local url
  url="$(batch_endpoint "$action")"
  log "POST ${url} — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X POST -H 'Authorization: Bearer \$TOKEN' -H 'Content-Type: application/json' ${url} -d '${body}'"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -X POST \
    "${url}" \
    -d "${body}"
}

cmd_create() {
  local slug="$1"
  local desc="$2"
  local body
  body="$(create_page_body "$slug")"
  log "POST $(page_path "$slug") — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X POST -H 'Authorization: Bearer \$TOKEN' -H 'Content-Type: application/json' $(page_path "$slug") -d '${body}'"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -X POST \
    "$(page_path "$slug")" \
    -d "${body}"
}

cmd_delete() {
  local slug="$1"
  log "DELETE $(page_path "$slug")"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X DELETE -H 'Authorization: Bearer \$TOKEN' $(page_path "$slug")"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    -X DELETE \
    "$(page_path "$slug")"
}

main() {
  log "WeKnora wiki batch endpoints — Build #12 smoke (dry-run safe)"
  log "BASE_URL='${BASE_URL}' KB_ID='${KB_ID}' prefix='${PAGE_PREFIX}'"
  echo
  if [[ -z "$BASE_URL" ]]; then
    warn "BATCH_SMOKE_BASE_URL is empty → dry-run. Set it to exercise a live server."
  else
    ok "Live smoke against ${BASE_URL}"
  fi
  echo

  log "Step 1/5 — seed three pages for the bulk test"
  cmd_create "${PAGE_PREFIX}-a" "create page a (target)"
  cmd_create "${PAGE_PREFIX}-b" "create page b (target)"
  cmd_create "${PAGE_PREFIX}-c" "create page c (target)"
  echo

  log "Step 2/5 — batch-move all three into the root folder (folder_id=\"\")"
  cmd_post "move" \
    "{\"slugs\":[\"${PAGE_PREFIX}-a\",\"${PAGE_PREFIX}-b\",\"${PAGE_PREFIX}-c\"],\"folder_id\":\"\"}" \
    "all → root"
  echo

  log "Step 3/5 — batch-status: archive two of three + a bogus slug"
  cmd_post "status" \
    "{\"slugs\":[\"${PAGE_PREFIX}-a\",\"${PAGE_PREFIX}-b\",\"${PAGE_PREFIX}-ghost\"],\"status\":\"archived\"}" \
    "mixed: 2 success + 1 not_found"
  echo

  log "Step 4/5 — batch-move with duplicate + empty slug entries (server dedupes)"
  cmd_post "move" \
    "{\"slugs\":[\"${PAGE_PREFIX}-a\",\" ${PAGE_PREFIX}-a \",\"\",\"${PAGE_PREFIX}-c\"],\"folder_id\":\"root\"}" \
    "dedupe trim smoke"
  echo

  log "Step 5/5 — batch-delete the three pages"
  cmd_post "delete" \
    "{\"slugs\":[\"${PAGE_PREFIX}-a\",\"${PAGE_PREFIX}-b\",\"${PAGE_PREFIX}-c\"]}" \
    "clean up"
  echo

  ok "smoke script completed — see HTTP %{http_code} lines above for actual server responses"
}

main "$@"