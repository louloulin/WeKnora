#!/usr/bin/env bash
# Smoke test the AI Assistant Q&A API with a real OpenAI key.
# Falls back to the NoopProvider when OPENAI_API_KEY is unset, so
# CI runs produce deterministic placeholder text instead of failing
# on a missing key.
#
# Requires: WeKnora server running locally with v0.7.17.x build.

set -euo pipefail

BASE_URL="${WEKNORA_BASE_URL:-http://localhost:8080}"
JWT="${WEKNORA_TEST_JWT:?WEKNORA_TEST_JWT must be set (use dev token)}"

if [[ -z "${OPENAI_API_KEY:-}" ]]; then
    echo "(OPENAI_API_KEY not set — running against NoopProvider fallback)"
    MODE="noop"
else
    echo "(OPENAI_API_KEY present — running against real OpenAI provider)"
    MODE="openai"
fi

echo ""
echo "=== POST /assistant/ask (JSON, mode=$MODE) ==="
RESP=$(curl -fsS -X POST "$BASE_URL/api/v1/assistant/ask" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "WeKnora 在 Kubernetes 里怎么部署？",
    "include_wiki": true,
    "max_results_per_source": 3
  }')
echo "$RESP" | python3 -m json.tool

ANSWER_TEXT=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('answer_text',''))")
MODEL_NAME=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('model_name',''))")

if [[ "$MODE" == "openai" ]]; then
    if [[ -z "$ANSWER_TEXT" || "$MODEL_NAME" != "gpt-4o-mini" && "$MODEL_NAME" != *"gpt"* ]]; then
        echo "FAIL: expected real LLM response with model_name containing gpt, got '$MODEL_NAME'"
        exit 1
    fi
    echo "PASS: model_name=$MODEL_NAME, answer_len=${#ANSWER_TEXT}"
else
    if [[ "$MODEL_NAME" != "noop" ]]; then
        echo "FAIL: expected NoopProvider fallback, got model_name=$MODEL_NAME"
        exit 1
    fi
    echo "PASS: noop placeholder, model_name=noop, answer_len=${#ANSWER_TEXT}"
fi

echo ""
echo "=== POST /assistant/ask?stream=1 (SSE, mode=$MODE) ==="
curl -fsS -N -X POST "$BASE_URL/api/v1/assistant/ask?stream=1" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"query":"流式输出 smoke 测试"}' \
  | head -25

echo ""
echo "=== DONE ==="
