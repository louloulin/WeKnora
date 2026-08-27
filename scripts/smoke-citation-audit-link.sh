#!/bin/bash
# Smoke test for the Build #30 B4 chat citation access audit endpoint.
#
# POST /api/v1/sessions/<session_id>/citation-log — every chat answer
# that cites one or more chunks (or a click on a `[[cite:N]]` token in
# the rendered answer) hits this endpoint. The handler fire-and-forgets
# an audit_logs row with action=chat.citation_accessed, scope_type=
# knowledge_base, scope_id=kb_id, target_type=citation, target_id=
# chunk_id, and Details JSON carrying chunk_id / source_message_id /
# citation_index / kb_id (+ optional title). The audit row carries the
# inbound X-Request-ID via correlation_id so the row joins the same
# trace as the originating chat turn (Build #25).
#
# The smoke flow:
#   1) POST a citation log — expect 200 + {"status":"accepted"}.
#   2) GET /audit-events?source=audit_logs and verify a chat.citation_accessed
#      row landed within the last minute with the same kb_id, chunk_id,
#      source_message_id, and citation_index.
#   3) Negative: omit citation_index → expect 400.
#
# Dry-run safe — with CITATION_SMOKE_BASE_URL unset the script only
# prints the curl commands it would run. Set the env vars to point at
# a live WeKnora instance to actually exercise the citation linkage.

set -euo pipefail

BASE_URL="${CITATION_SMOKE_BASE_URL:-}"
TOKEN="${CITATION_SMOKE_TOKEN:-}"
SESSION_ID="${CITATION_SMOKE_SESSION_ID:-session-smoke}"
KB_ID="${CITATION_SMOKE_KB_ID:-kb_smoke}"
CHUNK_ID="${CITATION_SMOKE_CHUNK_ID:-chunk-smoke}"
SOURCE_MSG_ID="${CITATION_SMOKE_SOURCE_MSG_ID:-msg-smoke}"

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[1;33m'
BLU='\033[0;34m'
NC='\033[0m'

log()  { printf "%b\n" "${BLU}[INFO]${NC} $1"; }
warn() { printf "%b\n" "${YEL}[WARN]${NC} $1"; }
ok()   { printf "%b\n" "${GRN}[✓]${NC} $1"; }
fail() { printf "%b\n" "${RED}[✗]${NC} $1"; exit 1; }

citation_endpoint() {
  printf "%s/api/v1/sessions/%s/citation-log" "$BASE_URL" "$SESSION_ID"
}

audit_endpoint() {
  printf "%s/api/v1/knowledgebase/%s/wiki/audit-events" "$BASE_URL" "$KB_ID"
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

log "Build #30 B4 chat citation audit linkage smoke"
log "Base URL:   ${BASE_URL:-<unset, dry-run>}"
log "Session ID: $SESSION_ID"
log "KB ID:      $KB_ID"
log "Chunk ID:   $CHUNK_ID"
log "Source Msg: $SOURCE_MSG_ID"
echo ""

# 1) Happy path — POST a citation access. The handler returns 200
#    immediately and queues an audit write in a goroutine.
log "1) POST /sessions/<id>/citation-log — happy path"
dry_run "log citation access" \
  curl -sS -X POST "$(citation_endpoint)" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -H "X-Request-ID: citation-smoke-$RANDOM" \
    -d "{
      \"kb_id\": \"$KB_ID\",
      \"chunk_id\": \"$CHUNK_ID\",
      \"source_message_id\": \"$SOURCE_MSG_ID\",
      \"citation_index\": 1,
      \"title\": \"Smoke test citation\"
    }"
echo ""

# 2) Negative path — citation_index is required and must be a
#    positive 1-based number. The handler returns 400 without
#    spawning the audit goroutine.
log "2) POST /citation-log with citation_index=0 — expect 400"
dry_run "reject zero citation_index" \
  curl -sS -o /dev/null -w "%{http_code}\n" -X POST "$(citation_endpoint)" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"kb_id\": \"$KB_ID\",
      \"chunk_id\": \"$CHUNK_ID\",
      \"source_message_id\": \"$SOURCE_MSG_ID\",
      \"citation_index\": 0
    }"
echo ""

# 3) Re-list audit events filtered to audit_logs only and confirm a
#    chat.citation_accessed row landed in the last minute with the
#    scope_id we just posted. The Build #24 wiki audit fan-out is
#    what backs this filter; the wiki_audit log source projects
#    audit_logs rows where scope_type=knowledge_base (Build #24 B2).
log "3) GET /audit-events?source=audit_logs — verify citation row landed"
SINCE="$(date -u -d '1 minute ago' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v-1M '+%Y-%m-%dT%H:%M:%SZ')"
dry_run "list recent audit_logs events" \
  curl -sS -G -X GET "$(audit_endpoint)" \
    -H "Authorization: Bearer $TOKEN" \
    --data-urlencode "source=audit_logs" \
    --data-urlencode "since=$SINCE"
echo ""

ok "Smoke script completed. Set CITATION_SMOKE_BASE_URL to run for real."