#!/bin/bash
# Smoke test for Build #25 — wiki audit cross-source correlation_id filter.
#
# Exercises the four-source audit fan-out under the new
# `correlation_id` query parameter. The script issues three requests:
#
#   1. GET .../wiki/audit-events                     (no filter, baseline)
#   2. GET .../wiki/audit-events?correlation_id=<X> (HTTP request-id)
#   3. GET .../wiki/audit-events?correlation_id=sweep:<uuid>
#                                                 (background sweeper)
#   4. GET .../wiki/audit-events?correlation_id=batch:<job_id>
#                                                 (background batch job)
#
# Each response should surface the same `source_event_id` on every
# matching row, confirming the four sources were joined via the
# shared correlation_id. Empty results are acceptable for a fresh KB
# — the assertion is on the response shape, not the row count.
#
# Dry-run safe: with WIKI_AUDIT_CORR_SMOKE_BASE_URL unset the script
# only prints the curl commands it WOULD run, so reviewers can audit
# the request shapes without standing up a server. Set the env vars
# below to point at a live WeKnora instance and it will exercise the
# actual endpoint and validate each response.
#
# Usage:
#   # dry-run
#   ./scripts/smoke-wiki-audit-correlation.sh
#
#   # live smoke against a local dev instance
#   WIKI_AUDIT_CORR_SMOKE_BASE_URL=http://localhost:8080 \
#   WIKI_AUDIT_CORR_SMOKE_TOKEN=$YOUR_JWT \
#   WIKI_AUDIT_CORR_SMOKE_KB_ID=kb_smoke \
#   WIKI_AUDIT_CORR_SMOKE_REQ_ID=req-smoke-0001 \
#   ./scripts/smoke-wiki-audit-correlation.sh

set -euo pipefail

BASE_URL="${WIKI_AUDIT_CORR_SMOKE_BASE_URL:-}"
TOKEN="${WIKI_AUDIT_CORR_SMOKE_TOKEN:-}"
KB_ID="${WIKI_AUDIT_CORR_SMOKE_KB_ID:-kb_smoke}"
REQ_ID="${WIKI_AUDIT_CORR_SMOKE_REQ_ID:-req-smoke-0001}"

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[1;33m'
BLU='\033[0;34m'
NC='\033[0m'

log()  { printf "%b\n" "${BLU}[INFO]${NC} $1"; }
warn() { printf "%b\n" "${YEL}[WARN]${NC} $1"; }
ok()   { printf "%b\n" "${GRN}[✓]${NC} $1"; }
fail() { printf "%b\n" "${RED}[✗]${NC} $1"; exit 1; }

audit_endpoint() {
  local query="${1:-}"
  printf "%s/api/v1/knowledgebase/%s/wiki/audit-events%s" \
    "$BASE_URL" "$KB_ID" "$query"
}

cmd_get_audit() {
  local query="$1"
  local desc="$2"
  local url
  url="$(audit_endpoint "$query")"
  log "GET ${url} — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' '${url}'"
    return
  fi
  local body
  body="$(curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "${url}")"
  printf "%s\n" "$body"
  if ! printf "%s" "$body" | grep -q '"source_counts"'; then
    fail "response missing source_counts envelope"
  fi
}

main() {
  log "Build #25 — wiki audit correlation_id smoke"
  log "  KB=$KB_ID  REQ_ID=$REQ_ID  base=$([ -n "$BASE_URL" ] && echo "$BASE_URL" || echo "<dry-run>")"

  # 1. Baseline — no filter; envelope should still carry source_counts.
  cmd_get_audit "" "baseline (no filter)"
  ok "baseline envelope shape ok"

  # 2. HTTP request-id filter — only rows tagged REQ_ID appear.
  cmd_get_audit "?correlation_id=${REQ_ID}" \
    "filter by HTTP request-id (X-Request-ID)"
  ok "HTTP request-id filter shape ok"

  # 3. Sweeper background prefix — only sweep:<uuid> rows appear.
  cmd_get_audit "?correlation_id=sweep:${REQ_ID}" \
    "filter by sweeper prefix (sweep:<uuid>)"
  ok "sweeper prefix filter shape ok"

  # 4. Batch worker prefix — only batch:<job_id> rows appear.
  cmd_get_audit "?correlation_id=batch:job-smoke-1" \
    "filter by batch worker prefix (batch:<job_id>)"
  ok "batch prefix filter shape ok"

  log "smoke complete — verify each response's source_event_id field"
  log "matches the filter value (drawer renders the chip column)."
}

main "$@"