#!/usr/bin/env bash
#
# scripts/smoke-wiki-sync-blocks.sh — v0.7.20 Synced Blocks smoke test.
#
# Usage:
#   WEKNORA_API=http://localhost:8080 \
#   WEKNORA_KB=kb-id \
#   WEKNORA_TOKEN=jwt \
#   ./scripts/smoke-wiki-sync-blocks.sh
set -euo pipefail

: "${WEKNORA_API:=http://localhost:8080}"
: "${WEKNORA_KB:?must set WEKNORA_KB}"
: "${WEKNORA_TOKEN:?must set WEKNORA_TOKEN}"

URL="${WEKNORA_API}/api/v1/knowledgebase/${WEKNORA_KB}/wiki/sync-blocks"
AUTH="Authorization: Bearer ${WEKNORA_TOKEN}"
CT="Content-Type: application/json"

echo "[smoke] GET list → ${URL}"
curl -sS -o /tmp/sb-list.json -w '%{http_code}\n' -H "$AUTH" "${URL}"
cat /tmp/sb-list.json && echo

BLOCK_ID="00000000-0000-0000-0000-000000000001"
echo "[smoke] POST create block ${BLOCK_ID}"
curl -sS -o /tmp/sb-create.json -w '%{http_code}\n' -X POST -H "$AUTH" -H "$CT" \
  -d "{\"block_id\":\"${BLOCK_ID}\",\"title\":\"Disclaimer\",\"content_json\":{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"text\":\"hello\"}]},\"content_md\":\"hello\"}" \
  "${URL}"
cat /tmp/sb-create.json && echo

echo "[smoke] GET canonical → ${URL}/${BLOCK_ID}"
curl -sS -o /tmp/sb-get.json -w '%{http_code}\n' -H "$AUTH" "${URL}/${BLOCK_ID}"
cat /tmp/sb-get.json && echo

echo "[smoke] PUT update canonical"
curl -sS -o /tmp/sb-put.json -w '%{http_code}\n' -X PUT -H "$AUTH" -H "$CT" \
  -d '{"title":"Disclaimer v2","content_json":{"type":"doc","content":[{"type":"paragraph","text":"hello v2"}]},"content_md":"hello v2"}' \
  "${URL}/${BLOCK_ID}"
cat /tmp/sb-put.json && echo

echo "[smoke] GET stats"
curl -sS -o /tmp/sb-stats.json -w '%{http_code}\n' -H "$AUTH" "${URL}/${BLOCK_ID}/stats"
cat /tmp/sb-stats.json && echo

echo "[smoke] GET refs"
curl -sS -o /tmp/sb-refs.json -w '%{http_code}\n' -H "$AUTH" "${URL}/${BLOCK_ID}/refs"
cat /tmp/sb-refs.json && echo

echo "[smoke] DELETE cascade"
curl -sS -o /tmp/sb-del.json -w '%{http_code}\n' -X DELETE -H "$AUTH" "${URL}/${BLOCK_ID}?mode=cascade"
cat /tmp/sb-del.json && echo

echo "[smoke] PASS — v0.7.20 synced-block endpoints respond end-to-end."
