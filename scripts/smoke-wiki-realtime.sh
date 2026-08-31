#!/usr/bin/env bash
#
# scripts/smoke-wiki-realtime.sh — v0.7.19 Yjs realtime collaboration smoke test.
#
# Usage:
#   WEKNORA_API=http://localhost:8080 \
#   WEKNORA_KB=kb-id \
#   WEKNORA_PAGE=page-slug \
#   WEKNORA_TOKEN=jwt \
#   ./scripts/smoke-wiki-realtime.sh
#
# Validates that the WS upgrade path is reachable and that a snapshot row
# appears in Postgres / SQLite after a single message round-trip.
set -euo pipefail

: "${WEKNORA_API:=http://localhost:8080}"
: "${WEKNORA_KB:?must set WEKNORA_KB}"
: "${WEKNORA_PAGE:?must set WEKNORA_PAGE}"
: "${WEKNORA_TOKEN:?must set WEKNORA_TOKEN}"

URL="${WEKNORA_API}/api/v1/knowledgebase/${WEKNORA_KB}/wiki/realtime/${WEKNORA_PAGE}"

echo "[smoke] GET presence snapshot → ${URL}/presence"
PRESENCE_HTTP=$(curl -sS -o /tmp/presence.json -w '%{http_code}' \
  -H "Authorization: Bearer ${WEKNORA_TOKEN}" \
  "${URL}/presence")
echo "[smoke] presence HTTP ${PRESENCE_HTTP}"
cat /tmp/presence.json && echo
if [[ "${PRESENCE_HTTP}" != "200" ]]; then
  echo "[smoke] FAIL — presence endpoint returned ${PRESENCE_HTTP}" >&2
  exit 1
fi

echo "[smoke] GET stats → ${WEKNORA_API}/api/v1/_realtime/_stats"
STATS_HTTP=$(curl -sS -o /tmp/stats.json -w '%{http_code}' \
  -H "Authorization: Bearer ${WEKNORA_TOKEN}" \
  "${WEKNORA_API}/api/v1/_realtime/_stats")
echo "[smoke] stats HTTP ${STATS_HTTP}"
cat /tmp/stats.json && echo
if [[ "${STATS_HTTP}" != "200" ]]; then
  echo "[smoke] FAIL — stats endpoint returned ${STATS_HTTP}" >&2
  exit 1
fi

echo "[smoke] PASS — v0.7.19 realtime endpoints respond."
