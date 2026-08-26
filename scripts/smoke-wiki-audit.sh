#!/bin/bash
# Smoke test for the wiki batch audit endpoints (Build #14).
#
# Dry-run safe: with BATCH_AUDIT_SMOKE_BASE_URL unset the script only prints
# the curl commands it WOULD run, so reviewers can audit the request shapes
# without standing up a server. Set the env vars below to point at a live
# WeKnora instance and it will exercise:
#
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-move (>=20 → async)
#   GET  /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>
#   GET  /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>/audit
#   POST /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>/cancel  (queued-only)
#   GET  /api/v1/knowledgebase/<kb>/wiki/batch-audit?actor=…
#   GET  /api/v1/knowledgebase/<kb>/wiki/batch-audit/export
#
# The audit chain we expect for a normal async batch is:
#   enqueue → start → finish   (succeeded/partial)
# For a cancelled queued job:
#   enqueue → cancel           (no start because the worker never picked it up)
#
# Usage:
#   # dry-run
#   ./scripts/smoke-wiki-audit.sh
#
#   # live smoke
#   BATCH_AUDIT_SMOKE_BASE_URL=http://localhost:8080 \
#   BATCH_AUDIT_SMOKE_TOKEN=$YOUR_JWT \
#   BATCH_AUDIT_SMOKE_KB_ID=kb_smoke \
#   BATCH_AUDIT_SMOKE_PAGE_PREFIX=audit-smoke \
#   ./scripts/smoke-wiki-audit.sh

set -euo pipefail

BASE_URL="${BATCH_AUDIT_SMOKE_BASE_URL:-}"
TOKEN="${BATCH_AUDIT_SMOKE_TOKEN:-}"
KB_ID="${BATCH_AUDIT_SMOKE_KB_ID:-kb_smoke}"
PAGE_PREFIX="${BATCH_AUDIT_SMOKE_PAGE_PREFIX:-audit-smoke}"
PAGE_COUNT="${BATCH_AUDIT_SMOKE_PAGE_COUNT:-25}"
POLL_INTERVAL="${BATCH_AUDIT_SMOKE_POLL_INTERVAL:-2}"
POLL_DEADLINE="${BATCH_AUDIT_SMOKE_POLL_DEADLINE:-30}"

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[1;33m'
BLU='\033[0;34m'
NC='\033[0m'

log()  { printf "%b\n" "${BLU}[INFO]${NC} $1"; }
warn() { printf "%b\n" "${YEL}[WARN]${NC} $1"; }
ok()   { printf "%b\n" "${GRN}[✓]${NC} $1"; }
fail() { printf "%b\n" "${RED}[✗]${NC} $1"; exit 1; }

# Threshold matches internal/types/wiki_page.go: WikiBatchAsyncThreshold
THRESHOLD=20

batch_endpoint() {
  local action="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/pages/batch-%s" \
    "$BASE_URL" "$KB_ID" "$action"
}

job_endpoint() {
  local id="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/batch-jobs/%s" \
    "$BASE_URL" "$KB_ID" "$id"
}

job_audit_endpoint() {
  local id="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/batch-jobs/%s/audit" \
    "$BASE_URL" "$KB_ID" "$id"
}

job_cancel_endpoint() {
  local id="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/batch-jobs/%s/cancel" \
    "$BASE_URL" "$KB_ID" "$id"
}

kb_audit_endpoint() {
  local qs="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/batch-audit%s" \
    "$BASE_URL" "$KB_ID" "$qs"
}

kb_audit_export_endpoint() {
  local qs="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/batch-audit/export%s" \
    "$BASE_URL" "$KB_ID" "$qs"
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
  local url="$1"
  local body="$2"
  local desc="$3"
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

cmd_get() {
  local url="$1"
  local desc="$2"
  log "GET ${url} — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' ${url}"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "${url}"
}

# poll_until_terminal exits 0 when state ∈ {succeeded,failed,partial},
# 1 on timeout. In dry-run mode it returns immediately.
poll_until_terminal() {
  local id="$1"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) would poll ${id} every ${POLL_INTERVAL}s until terminal"
    return 0
  fi
  local deadline=$((SECONDS + POLL_DEADLINE))
  while (( SECONDS < deadline )); do
    local body
    body="$(curl -sS \
        -H "Authorization: Bearer ${TOKEN}" \
        "$(job_endpoint "$id")")"
    local state
    state="$(printf '%s' "$body" | sed -n 's/.*"state":"\([a-z]*\)".*/\1/p' | head -1)"
    case "$state" in
      succeeded|failed|partial)
        ok "job $id reached terminal state=$state"
        return 0
        ;;
      queued|running)
        log "job $id state=$state, sleeping ${POLL_INTERVAL}s"
        sleep "$POLL_INTERVAL"
        ;;
      *)
        fail "unexpected job state for $id: '$state'"
        ;;
    esac
  done
  fail "job $id did not reach terminal state within ${POLL_DEADLINE}s"
}

main() {
  log "WeKnora wiki batch audit endpoints — Build #14 smoke (dry-run safe)"
  log "BASE_URL='${BASE_URL}' KB_ID='${KB_ID}' PAGE_COUNT='${PAGE_COUNT}' threshold='${THRESHOLD}'"
  echo
  if [[ -z "$BASE_URL" ]]; then
    warn "BATCH_AUDIT_SMOKE_BASE_URL is empty → dry-run. Set it to exercise a live server."
  else
    ok "Live smoke against ${BASE_URL}"
    if (( PAGE_COUNT < THRESHOLD )); then
      fail "PAGE_COUNT=$PAGE_COUNT is below threshold=$THRESHOLD; the sync path will not exercise async"
    fi
  fi
  echo

  # Build the slug list once — same N used across all three endpoints.
  local slugs=()
  local i
  for (( i = 0; i < PAGE_COUNT; i++ )); do
    slugs+=("${PAGE_PREFIX}-${i}")
  done
  local slug_array
  slug_array="$(build_json_array "${slugs[@]}")"

  log "Step 1/5 — POST batch-move (>=${THRESHOLD}, async) so audit records enqueue→start→finish"
  local move_body="{\"slugs\":${slug_array},\"folder_id\":\"root\"}"
  cmd_post "$(batch_endpoint move)" "$move_body" "async path → returns {kind:'job', job:{...}}"
  echo

  log "Step 2/5 — POST batch-move AGAIN with one slug; that job will be cancelled before the worker grabs it"
  cmd_post "$(batch_endpoint move)" "$move_body" "second batch to exercise cancel path"
  echo

  log "Step 3/5 — exercise per-job + KB-wide audit + cancel + export endpoints"
  log "  Replace JOB_ID below with the id from step 1's response, then run:"
  log "    cmd_get  $(job_audit_endpoint REPLACE_WITH_JOB_ID)  'per-job audit (oldest-first, <=7 events)'"
  log "    cmd_post $(job_cancel_endpoint REPLACE_WITH_JOB_ID_2)  '{}'  'cancel queued job (200 or 409)'"
  log "    cmd_get  $(kb_audit_endpoint '?page=1&page_size=20')  'KB-wide audit page 1'"
  log "    cmd_get  $(kb_audit_endpoint '?actor=system')  'KB-wide audit filtered by system actor'"
  log "    cmd_get  $(kb_audit_export_endpoint '?actor=system')  'KB-wide audit CSV export'"
  echo
  cmd_get  "$(job_audit_endpoint "REPLACE_WITH_JOB_ID")"   "per-job audit"
  cmd_post "$(job_cancel_endpoint "REPLACE_WITH_JOB_ID_2")" "{}" "cancel queued job"
  cmd_get  "$(kb_audit_endpoint "?page=1&page_size=20")"    "KB-wide audit page 1"
  cmd_get  "$(kb_audit_endpoint "?actor=system")"            "KB-wide audit filter actor=system"
  cmd_get  "$(kb_audit_export_endpoint "?actor=system")"     "KB-wide audit CSV export"
  echo

  log "Step 4/5 — poll the step-1 job until terminal so the audit chain is complete"
  log "  Replace JOB_ID below with the id from step 1, then run:"
  log "    poll_until_terminal REPLACE_WITH_JOB_ID"
  echo
  poll_until_terminal "REPLACE_WITH_JOB_ID"
  echo

  log "Step 5/5 — re-fetch the per-job audit to confirm the finish event landed"
  cmd_get "$(job_audit_endpoint "REPLACE_WITH_JOB_ID")" "per-job audit after finish"
  echo

  ok "smoke script completed — see HTTP %{http_code} lines above for actual server responses"
  if [[ -z "$BASE_URL" ]]; then
    warn "Dry-run only — re-run with BATCH_AUDIT_SMOKE_BASE_URL set to exercise poll_until_terminal + cancel + audit GETs."
  fi
}

main "$@"