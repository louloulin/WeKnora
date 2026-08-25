#!/bin/bash
# Smoke test for the wiki page-level ACL endpoints (Build #7 backend +
# Build #10 frontend contract).
#
# This is a *dry-run safe* template — by default it does not hit any
# server. Set ACL_SMOKE_BASE_URL to the WeKnora instance you want to
# exercise, and ACL_SMOKE_TOKEN to a bearer token, and it will issue
# real PUT/GET requests and print the responses.
#
# Usage:
#   # dry-run (prints commands only)
#   ./scripts/smoke-wiki-acl.sh
#
#   # live smoke against a local dev instance
#   ACL_SMOKE_BASE_URL=http://localhost:8080 \
#   ACL_SMOKE_TOKEN=$YOUR_JWT \
#   ACL_SMOKE_KB_ID=kb_demo \
#   ACL_SMOKE_SLUG=welcome \
#   ./scripts/smoke-wiki-acl.sh
#
# The script demonstrates:
#   1. GET  .../wiki/pages/<slug>/acl        → 200 + canonical record
#   2. PUT  .../wiki/pages/<slug>/acl        → 200 + revision bumped
#   3. PUT  .../wiki/pages/<slug>/acl        → 409 when baseRevision is stale
#                                               and server returns currentAcl

set -euo pipefail

BASE_URL="${ACL_SMOKE_BASE_URL:-}"
TOKEN="${ACL_SMOKE_TOKEN:-}"
KB_ID="${ACL_SMOKE_KB_ID:-kb_smoke}"
SLUG="${ACL_SMOKE_SLUG:-welcome}"

RED='\033[0;31m'
GRN='\033[0;32m'
YEL='\033[1;33m'
BLU='\033[0;34m'
NC='\033[0m'

log()    { printf "%b\n" "${BLU}[INFO]${NC} $1"; }
warn()   { printf "%b\n" "${YEL}[WARN]${NC} $1"; }
ok()     { printf "%b\n" "${GRN}[✓]${NC} $1"; }
fail()   { printf "%b\n" "${RED}[✗]${NC} $1"; exit 1; }

acl_endpoint() {
  printf "%s/api/v1/knowledgebase/%s/wiki/pages/%s/acl" \
    "$BASE_URL" "$KB_ID" "$SLUG"
}

cmd_get() {
  log "GET $(acl_endpoint)"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -H 'Authorization: Bearer \$TOKEN' $(acl_endpoint)"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$(acl_endpoint)"
}

cmd_put() {
  local payload="$1"
  local desc="$2"
  log "PUT $(acl_endpoint) — ${desc}"
  if [[ -z "$BASE_URL" ]]; then
    log "(dry-run) curl -sS -X PUT -H 'Authorization: Bearer \$TOKEN' \\"
    log "  -H 'Content-Type: application/json' \\"
    log "  -d '${payload}' $(acl_endpoint)"
    return
  fi
  curl -sS -w "\nHTTP %{http_code}\n" \
    -X PUT \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "$(acl_endpoint)"
}

echo
log "== 1. GET current ACL (should 200 with mode=inherit on a fresh page) =="
cmd_get
echo

log "== 2. PUT ACL → private (happy path) =="
PAYLOAD_PRIVATE='{"mode":"private","allowUserIds":[],"allowGroupIds":[],"denyInherited":false,"baseRevision":0}'
cmd_put "$PAYLOAD_PRIVATE" "switch to private"
echo

log "== 3. PUT ACL → allow_list (bumps revision) =="
PAYLOAD_ALLOW='{"mode":"allow_list","allowUserIds":["u_demo"],"allowGroupIds":[],"denyInherited":false,"baseRevision":1}'
cmd_put "$PAYLOAD_ALLOW" "switch to allow_list"
echo

log "== 4. PUT ACL with STALE baseRevision → expect 409 + currentAcl in body =="
PAYLOAD_STALE='{"mode":"private","allowUserIds":[],"allowGroupIds":[],"denyInherited":false,"baseRevision":1}'
cmd_put "$PAYLOAD_STALE" "intentionally stale"
echo

if [[ -z "$BASE_URL" ]]; then
  warn "BASE_URL not set — dry-run only. Set ACL_SMOKE_BASE_URL + ACL_SMOKE_TOKEN to hit a real server."
else
  ok "Live smoke complete. Review the HTTP codes + bodies above."
fi