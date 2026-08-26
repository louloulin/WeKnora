#!/bin/bash
# Smoke test for the wiki batch observability endpoints (Build #15).
#
# Exercises the per-slug progress + per-slug failure-ledger surface that
# was added on top of the Build #13 async job pool:
#
#   GET  /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>          (poll progress)
#   GET  /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>/failures (per-slug ledger)
#   GET  /api/v1/knowledgebase/<kb>/wiki/batch-jobs/<id>/failures?code=internal
#
# Dry-run safe: with BATCH_OBS_SMOKE_BASE_URL unset the script only
# prints the curl commands it WOULD run, so reviewers can audit the
# request shapes without standing up a server. Set the env vars below
# to point at a live WeKnora instance and it will exercise the
# progress field on the job poll response plus the failure-list query
# parameters (`code`, `page`, `page_size`).
#
# Usage:
#   # dry-run
#   ./scripts/smoke-wiki-batch-observability.sh
#
#   # live smoke against a local dev instance
#   BATCH_OBS_SMOKE_BASE_URL=http://localhost:8080 \
#   BATCH_OBS_SMOKE_TOKEN=$YOUR_JWT \
#   BATCH_OBS_SMOKE_KB_ID=kb_smoke \
#   BATCH_OBS_SMOKE_JOB_ID=$YOUR_JOB_ID \
#   ./scripts/smoke-wiki-batch-observability.sh

set -euo pipefail

BASE_URL="${BATCH_OBS_SMOKE_BASE_URL:-}"
TOKEN="${BATCH_OBS_SMOKE_TOKEN:-}"
KB_ID="${BATCH_OBS_SMOKE_KB_ID:-kb_smoke}"
JOB_ID="${BATCH_OBS_SMOKE_JOB_ID:-REPLACE_WITH_JOB_ID}"

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[1;33m'
BLU='\033[0;34m'
NC='\033[0m'

log()  { printf "%b\n" "${BLU}[INFO]${NC} $1"; }
warn() { printf "%b\n" "${YEL}[WARN]${NC} $1"; }
ok()   { printf "%b\n" "${GRN}[✓]${NC} $1"; }
fail() { printf "%b\n" "${RED}[✗]${NC} $1"; exit 1; }

job_endpoint() {
  local id="$1"
  printf "%s/api/v1/knowledgebase/%s/wiki/batch-jobs/%s" \
    "$BASE_URL" "$KB_ID" "$id"
}

failures_endpoint() {
  local id="$1"
  local query="${2:-}"
  printf "%s/api/v1/knowledgebase/%s/wiki/batch-jobs/%s/failures%s" \
    "$BASE_URL" "$KB_ID" "$id" "$query"
}

cmd_get_job() {
  local id="$1"
  local url
  url="$(job_endpoint "$id")"
  log "GET ${url} — progress snapshot (Build #15)"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' ${url}"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "${url}"
}

cmd_get_failures() {
  local id="$1"
  local query="$2"
  local desc="$3"
  local url
  url="$(failures_endpoint "$id" "$query")"
  log "GET ${url} — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' '${url}'"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "${url}"
}

main() {
  log "WeKnora wiki batch observability — Build #15 smoke (dry-run safe)"
  log "BASE_URL='${BASE_URL}' KB_ID='${KB_ID}' JOB_ID='${JOB_ID}'"
  echo
  if [[ -z "$BASE_URL" ]]; then
    warn "BATCH_OBS_SMOKE_BASE_URL is empty → dry-run. Set it to exercise a live server."
    warn "Replace JOB_ID with a real batch job id (from a prior batch-* POST) to inspect its failure ledger."
  else
    ok "Live smoke against ${BASE_URL}"
    if [[ "$JOB_ID" == "REPLACE_WITH_JOB_ID" ]]; then
      fail "JOB_ID is still REPLACE_WITH_JOB_ID — set BATCH_OBS_SMOKE_JOB_ID to a real id"
    fi
  fi
  echo

  log 'Step 1/3 — job poll: confirm "progress" snapshot is present on terminal state'
  cmd_get_job "$JOB_ID"
  echo

  log "Step 2/3 — failure ledger: full set (no code filter), page 1"
  cmd_get_failures "$JOB_ID" "?page=1&page_size=50" \
    "per-slug failures oldest-first + per-code groups + total"
  echo

  log "Step 3/3 — failure ledger: code=internal filter, page 1"
  cmd_get_failures "$JOB_ID" "?code=internal&page=1&page_size=50" \
    "filter narrows list and tabs still show group counts over the filtered set"
  echo

  ok "smoke script completed — see HTTP %{http_code} lines above for actual server responses"
  if [[ -z "$BASE_URL" ]]; then
    warn "Dry-run only — re-run with BATCH_OBS_SMOKE_BASE_URL set + JOB_ID of a real batch job."
  fi
}

main "$@"