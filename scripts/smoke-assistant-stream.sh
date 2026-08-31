#!/usr/bin/env bash
# Smoke test the AI Assistant Q&A API end-to-end.
#
# Exercises:
#   1. POST /api/v1/assistant/ask           — JSON response
#   2. POST /api/v1/assistant/ask?stream=1  — SSE response (3 frames)
#   3. GET  /api/v1/assistant/conversations — tenant-scoped audit
#
# Assumes the WeKnora server is running locally with the v0.7.17+
# build. Adjust BASE_URL + JWT below for your environment.

set -euo pipefail

BASE_URL="${WEKNORA_BASE_URL:-http://localhost:8080}"
JWT="${WEKNORA_TEST_JWT:?WEKNORA_TEST_JWT must be set (use dev token)}"

echo "=== 1. POST /assistant/ask (JSON) ==="
ASK_RESP=$(curl -fsS -X POST "$BASE_URL/api/v1/assistant/ask" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"query":"WeKnora 怎么部署？","include_wiki":true,"max_results_per_source":3}')
echo "$ASK_RESP" | python3 -m json.tool | head -30
ANSWER_ID=$(echo "$ASK_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('answer_id',''))")
CONV_ID=$(echo "$ASK_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('conversation_id',''))")
echo "answer_id=$ANSWER_ID"
echo "conversation_id=$CONV_ID"

echo ""
echo "=== 2. POST /assistant/ask?stream=1 (SSE) ==="
curl -fsS -N -X POST "$BASE_URL/api/v1/assistant/ask?stream=1" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"SSE smoke\",\"conversation_id\":\"$CONV_ID\"}" \
  | head -20

echo ""
echo "=== 3. GET /assistant/conversations ==="
curl -fsS "$BASE_URL/api/v1/assistant/conversations?limit=5" \
  -H "Authorization: Bearer $JWT" | python3 -m json.tool | head -30

echo ""
echo "=== DONE ==="
