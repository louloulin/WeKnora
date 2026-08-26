#!/bin/bash
# Smoke test for the wiki batch dry-run preview endpoints (Build #16).
#
# Dry-run safe: with BATCH_PREVIEW_SMOKE_BASE_URL unset the script
# only prints the curl commands it WOULD run, so reviewers can audit
# the request shapes without standing up a server. Set the env vars
# below to point at a live WeKnora instance and it will exercise:
#
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-preview-move
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-preview-delete
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-preview-status
#
# Each endpoint returns a WikiBatchPreviewResponse body with shape:
#   {
#     "success": ["slug-1", ...],
#     "failed":  [{ "slug": "slug-x", "code": "not_found", "error": "..." }, ...],
#     "summary": { "total": N, "will_succeed": K, "will_fail": F }
#   }
#
# The preview endpoints are 100% read-only — they MUST NOT mutate
# the database (verified by the harness test in
# internal/application/service/wiki_batch_preview_test.go). After
# previewing a batch, re-running the same request must return the
# same response shape. State remains unchanged.
#
# Failure codes observed in practice:
#   not_found         — slug does not exist in this KB
#   folder_not_found  — folder_id missing in this KB
#   kb_mismatch       — slug belongs to another KB (HTTP 400)
#   invalid_status    — status not in {draft, published, archived}
#
# Usage:
#   # dry-run
#   ./scripts/smoke-wiki-batch-preview.sh
#
#   # live smoke against a local dev instance
#   BATCH_PREVIEW_SMOKE_BASE_URL=http://localhost:8080 \
#   BATCH_PREVIEW_SMOKE_TOKEN=$YOUR_JWT \
#   BATCH_PREVIEW_SMOKE_KB_ID=kb_smoke \
#   BATCH_PREVIEW_SMOKE_PAGE_PREFIX=preview-smoke \
#   BATCH_PREVIEW_SMOKE_FOLDER_ID=root \
#   ./scripts/smoke-wiki-batch-preview.sh

set -euo pipefail

BASE_URL="${BATCH_PREVIEW_SMOKE_BASE_URL:-}"
TOKEN="${BATCH_PREVIEW_SMOKE_TOKEN:-}"
KB_ID="${BATCH_PREVIEW_SMOKE_KB_ID:-kb_smoke}"
PAGE_PREFIX="${BATCH_PREVIEW_SMOKE_PAGE_PREFIX:-preview-smoke}"
PAGE_COUNT="${BATCH_PREVIEW_SMOKE_PAGE_COUNT:-25}"
FOLDER_ID="${BATCH_PREVIEW_SMOKE_FOLDER_ID:-root}"

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[1;33m'
BLU='\033[0;34m'
NC='\033[0m'

log()  { printf "%b\n" "${BLU}[INFO]${NC} $1"; }
warn() { printf "%b\n" "${YEL}[WARN]${NC} $1"; }
ok()   { printf "%b\n" "${GRN}[✓]${NC} $1"; }
fail() { printf "%b\n" "${RED}[✗]${NC} $1"; exit 1; }

# Build #16 mirrors Build #13: the auto-async threshold lives in
# internal/types/wiki_page.go: WikiBatchAsyncThreshold. The frontend
# also imports the same constant as WikiBatchAsyncThreshold in
# frontend/src/api/wiki/batchTypes.ts and gates the preview button
# on `count >= WikiBatchAsyncThreshold`.
THRESHOLD=20

preview_endpoint() {
  local action="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/pages/batch-preview-%s" \
    "$BASE_URL" "$KB_ID" "$action"
}

# build_json_array joins a list of slugs into a JSON array literal.
# uses bash printf for portability — no jq dependency.
build_json_array() {
  local sep=""
  printf '['
  for slug in "$@"; do
    printf '%s"%s"' "$sep" "$slug"
    sep=","
  done
  printf ']'
}

cmd_post() {
  local action="$1"
  local body="$2"
  local desc="$3"
  local url
  url="$(preview_endpoint "$action")"
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

main() {
  log "WeKnora wiki batch dry-run preview — Build #16 smoke (dry-run safe)"
  log "BASE_URL='${BASE_URL}' KB_ID='${KB_ID}' PAGE_COUNT='${PAGE_COUNT}' threshold='${THRESHOLD}'"
  echo
  if [[ -z "$BASE_URL" ]]; then
    warn "BATCH_PREVIEW_SMOKE_BASE_URL is empty → dry-run. Set it to exercise a live server."
  else
    ok "Live smoke against ${BASE_URL}"
    if (( PAGE_COUNT < THRESHOLD )); then
      warn "PAGE_COUNT=$PAGE_COUNT is below threshold=$THRESHOLD; the preview button is gated at >= $THRESHOLD on the frontend, but the backend endpoint accepts any size."
    fi
  fi
  echo

  # Build the slug list once — same N used across all three preview endpoints.
  # The frontend sends a mix of real and fake slugs to exercise partial failure.
  local slugs=()
  local i
  for (( i = 0; i < PAGE_COUNT; i++ )); do
    slugs+=("${PAGE_PREFIX}-${i}")
  done
  # Add a known-bad slug so the preview returns at least one failed row.
  slugs+=("${PAGE_PREFIX}-does-not-exist")
  # Add a duplicate to verify dedup.
  slugs+=("${PAGE_PREFIX}-0")
  local slug_array
  slug_array="$(build_json_array "${slugs[@]}")"

  log "Step 1/3 — batch-preview-move ${#slugs[@]} slugs (after dedup ${PAGE_COUNT}+1)"
  log "  Expected: most will fail with code='not_found' (test environment); preview only validates shape + per-slug code, no DB writes."
  local move_body="{\"slugs\":${slug_array},\"folder_id\":\"${FOLDER_ID}\"}"
  cmd_post "move" "$move_body" "preview-only; summary.{total,will_succeed,will_fail} returned; no folder change"
  echo

  log "Step 2/3 — batch-preview-delete ${#slugs[@]} slugs"
  log "  Expected: pure read-only delete check; no pages actually removed."
  local delete_body="{\"slugs\":${slug_array}}"
  cmd_post "delete" "$delete_body" "preview-only; no DeletePage side effects"
  echo

  log "Step 3/3 — batch-preview-status ${#slugs[@]} slugs (target: archived)"
  log "  Expected: invalid_status 400 is NOT exercised here — the frontend already filters. Server validates 'archived' is a known status and returns 200 with the same shape."
  local status_body="{\"slugs\":${slug_array},\"status\":\"archived\"}"
  cmd_post "status" "$status_body" "preview-only; no status writes"
  echo

  log "Negative cases (rejected at request level — return HTTP 400):"
  log "  • cross-KB slugs              → kb_mismatch / 400"
  log "  • status='garbage'            → invalid_status / 400"
  log "  • slugs with non-string items → 400 from validateBatchSlugs"
  log "  See wiki_batch_preview_test.go: TestPreviewBatchMove_CrossKBRejectsRequestLevel + TestPreviewBatchStatus_RejectsInvalidStatus"
  echo

  ok "smoke script completed — review the printed request shapes; rerun with BATCH_PREVIEW_SMOKE_BASE_URL set to hit a live server."
}

main "$@"