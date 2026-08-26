#!/bin/bash
# Smoke test for the async batch job endpoints (Build #13).
#
# Dry-run safe: with BATCH_ASYNC_SMOKE_BASE_URL unset the script
# only prints the curl commands it WOULD run, so reviewers can audit
# the request shapes without standing up a server. Set the env vars
# below to point at a live WeKnora instance and it will exercise:
#
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-move   (>=20 → async)
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-delete (>=20 → async)
#   POST /api/v1/knowledgebase/<kb>/wiki/pages/batch-status (>=20 → async)
#   GET  /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>   (poll)
#   POST /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>/undo
#
# Auto-routing threshold is 20 slugs: at/above this the server returns
#   { "kind": "job", "job": { "id": "...", "state": "queued", ... } }
# and the client is expected to poll until the state is one of
#   succeeded | failed | partial
# (see frontend/src/api/wiki/batchTypes.ts: WikiBatchJobTerminalStates).
#
# Undoable job types: move | delete. Status jobs return 422 on undo.
#
# Usage:
#   # dry-run
#   ./scripts/smoke-wiki-batch-async.sh
#
#   # live smoke against a local dev instance
#   BATCH_ASYNC_SMOKE_BASE_URL=http://localhost:8080 \
#   BATCH_ASYNC_SMOKE_TOKEN=$YOUR_JWT \
#   BATCH_ASYNC_SMOKE_KB_ID=kb_smoke \
#   BATCH_ASYNC_SMOKE_PAGE_PREFIX=async-smoke \
#   ./scripts/smoke-wiki-batch-async.sh

set -euo pipefail

BASE_URL="${BATCH_ASYNC_SMOKE_BASE_URL:-}"
TOKEN="${BATCH_ASYNC_SMOKE_TOKEN:-}"
KB_ID="${BATCH_ASYNC_SMOKE_KB_ID:-kb_smoke}"
PAGE_PREFIX="${BATCH_ASYNC_SMOKE_PAGE_PREFIX:-async-smoke}"
PAGE_COUNT="${BATCH_ASYNC_SMOKE_PAGE_COUNT:-25}"
POLL_INTERVAL="${BATCH_ASYNC_SMOKE_POLL_INTERVAL:-2}"
POLL_DEADLINE="${BATCH_ASYNC_SMOKE_POLL_DEADLINE:-30}"

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[1;33m'
BLU='\033[0;34m'
NC='\033[0m'

log()  { printf "%b\n" "${BLU}[INFO]${NC} $1"; }
warn() { printf "%b\n" "${YEL}[WARN]${NC} $1"; }
ok()   { printf "%b\n" "${GRN}[✓]${NC} $1"; }
fail() { printf "%b\n" "${RED}[✗]${NC} $1"; exit 1; }

# threshold matches internal/types/wiki_page.go: WikiBatchAsyncThreshold
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

job_undo_endpoint() {
  local id="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/batch-jobs/%s/undo" \
    "$BASE_URL" "$KB_ID" "$id"
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

cmd_get() {
  local id="$1"
  local desc="$2"
  local url
  url="$(job_endpoint "$id")"
  log "GET ${url} — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' ${url}"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "${url}"
}

cmd_undo() {
  local id="$1"
  local desc="$2"
  local url
  url="$(job_undo_endpoint "$id")"
  log "POST ${url} — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X POST -H 'Authorization: Bearer \$TOKEN' ${url} -d '{}'"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -X POST \
    "${url}" \
    -d '{}'
}

# Wait for the job to leave queued/running. Exits 0 when terminal
# state is reached, 1 on timeout. In dry-run mode the function
# returns immediately so the script can be reviewed without a
# running worker pool.
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
  log "WeKnora wiki batch async endpoints — Build #13 smoke (dry-run safe)"
  log "BASE_URL='${BASE_URL}' KB_ID='${KB_ID}' PAGE_COUNT='${PAGE_COUNT}' threshold='${THRESHOLD}'"
  echo
  if [[ -z "$BASE_URL" ]]; then
    warn "BATCH_ASYNC_SMOKE_BASE_URL is empty → dry-run. Set it to exercise a live server."
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

  log "Step 1/4 — batch-move ${PAGE_COUNT} slugs (>= ${THRESHOLD}, async)"
  local move_body="{\"slugs\":${slug_array},\"folder_id\":\"root\"}"
  cmd_post "move" "$move_body" "async path → returns {kind:'job', job:{...}}"
  echo

  log "Step 2/4 — batch-delete ${PAGE_COUNT} slugs (>= ${THRESHOLD}, async)"
  local delete_body="{\"slugs\":${slug_array}}"
  cmd_post "delete" "$delete_body" "async path → returns {kind:'job', job:{...}}"
  echo

  log "Step 3/4 — batch-status ${PAGE_COUNT} slugs (>= ${THRESHOLD}, async, NOT undoable)"
  local status_body="{\"slugs\":${slug_array},\"status\":\"archived\"}"
  cmd_post "status" "$status_body" "async path; undo on status jobs returns 422"
  echo

  log "Step 4/4 — poll + undo round-trip (manual job id)"
  log "  Replace JOB_ID below with the id from step 1's response, then run:"
  log "    cmd_get  JOB_ID 'poll progress'"
  log "    cmd_undo JOB_ID 'undo the move batch'"
  echo
  cmd_get  "REPLACE_WITH_JOB_ID" "poll job progress"
  cmd_undo "REPLACE_WITH_JOB_ID" "undo move batch (expect 200 + expires_at cleared)"
  echo

  ok "smoke script completed — see HTTP %{http_code} lines above for actual server responses"
  if [[ -z "$BASE_URL" ]]; then
    warn "Dry-run only — re-run with BATCH_ASYNC_SMOKE_BASE_URL set to exercise poll_until_terminal + undo."
  fi
}

main "$@"