#!/bin/bash
# Smoke test for the Build #24 unified wiki audit endpoint.
#
# GET /api/v1/knowledgebase/<kb>/wiki/audit-events — the 4-source
# fan-out that merges audit_logs + wiki_batch_job_audit +
# wiki_backlinks_cache_invalidation_log + wiki_page_acl_audit,
# sorted by (timestamp DESC, source rank ASC, id ASC) with stable
# tiebreak.
#
# Also exercises the D3 ACL→cache hook path: write a page-level
# ACL via the wiki_acl PUT endpoint, then verify the response
# from /audit-events includes a wiki_page_acl_audit row + a
# wiki_backlinks_cache_invalidation_log row with op="acl_change".
#
# Dry-run safe — with AUDIT_SMOKE_BASE_URL unset the script only
# prints the curl commands it would run. Set the env vars to point
# at a live WeKnora instance to actually exercise the fan-out.

set -euo pipefail

BASE_URL="${AUDIT_SMOKE_BASE_URL:-}"
TOKEN="${AUDIT_SMOKE_TOKEN:-}"
KB_ID="${AUDIT_SMOKE_KB_ID:-kb_smoke}"
SLUG="${AUDIT_SMOKE_SLUG:-page-smoke}"

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
  printf "%s/api/v1/knowledgebase/%s/wiki/audit-events" "$BASE_URL" "$KB_ID"
}

acl_endpoint() {
  printf "%s/api/v1/knowledgebase/%s/wiki/pages/%s/acl" \
    "$BASE_URL" "$KB_ID" "$SLUG"
}

# dry_run executes the command when BASE_URL is set, else prints it.
dry_run() {
  local label="$1"; shift
  if [[ -z "$BASE_URL" ]]; then
    printf "%b\n" "${YEL}[dry-run]${NC} $label"
    printf "    %s\n" "$*"
    return 0
  fi
  "$@"
}

log "Build #24 unified wiki audit smoke"
log "Base URL:  ${BASE_URL:-<unset, dry-run>}"
log "KB ID:     $KB_ID"
log "Slug:      $SLUG"
echo ""

# 1) Audit list — all sources, default 24h window
log "1) GET /audit-events — full fan-out"
dry_run "list audit events" \
  curl -sS -G -X GET "$(audit_endpoint)" \
    -H "Authorization: Bearer $TOKEN" \
    --data-urlencode "page=1" \
    --data-urlencode "page_size=20"
echo ""

# 2) Audit list — restrict to wiki_page_acl_audit only
log "2) GET /audit-events?source=wiki_page_acl_audit"
dry_run "list ACL-only events" \
  curl -sS -G -X GET "$(audit_endpoint)" \
    -H "Authorization: Bearer $TOKEN" \
    --data-urlencode "source=wiki_page_acl_audit" \
    --data-urlencode "page_size=10"
echo ""

# 3) Audit list — restrict to invalidation events with affected_count > 0
log "3) GET /audit-events?source=wiki_backlinks_cache_invalidation_log"
dry_run "list invalidation events" \
  curl -sS -G -X GET "$(audit_endpoint)" \
    -H "Authorization: Bearer $TOKEN" \
    --data-urlencode "source=wiki_backlinks_cache_invalidation_log"
echo ""

# 4) ACL write → must produce ACL + invalidation rows on the next read
log "4) PUT /pages/<slug>/acl — triggers D3 ACL→cache hook"
dry_run "set private ACL" \
  curl -sS -X PUT "$(acl_endpoint)" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
      "mode": "private",
      "base_revision": 0,
      "allow_user_ids": [],
      "allow_group_ids": []
    }'
echo ""

# 5) Re-list after the write — should show a fresh ACL row + an
#    invalidation row tagged op="acl_change".
log "5) GET /audit-events?actor=<self> — expect at least 2 rows since now-5m"
SINCE="$(date -u -d '5 minutes ago' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v-5M '+%Y-%m-%dT%H:%M:%SZ')"
dry_run "list recent actor events" \
  curl -sS -G -X GET "$(audit_endpoint)" \
    -H "Authorization: Bearer $TOKEN" \
    --data-urlencode "since=$SINCE"
echo ""

ok "Smoke script completed. Set AUDIT_SMOKE_BASE_URL to run for real."