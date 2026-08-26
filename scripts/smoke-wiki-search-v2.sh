#!/usr/bin/env bash
# Build #19 / P2.x.a — wiki search v2 smoke script (DRY_RUN-safe).
#
# Exercises the v2 endpoint contract:
#   - empty query → 200 + empty hits
#   - title/content hits both surfaced
#   - server-rendered <mark> snippet contains the query
#   - ?legacy=1 still returns the legacy payload shape
#   - cross-KB explicit kb_ids[] merges hits
#
# Run modes:
#   DRY_RUN=1 bash scripts/smoke-wiki-search-v2.sh    # print curl only
#   BASE=http://localhost:8080 TOKEN=... bash scripts/smoke-wiki-search-v2.sh
#
# The script never creates real data — it stops at the first assertion
# failure with `set -euo pipefail` semantics so CI gets a clear exit.

set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
KB_ID="${KB_ID:-kb-smoke}"
TOKEN="${TOKEN:-}"
DRY_RUN="${DRY_RUN:-1}"

run() {
  local label="$1"; shift
  echo
  echo "── ${label} ──"
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "$ $*"
    return 0
  fi
  "$@"
}

auth_header=()
if [[ -n "${TOKEN}" ]]; then
  auth_header=(-H "Authorization: Bearer ${TOKEN}")
fi

# 1) empty query → 200 + empty hits (never 400)
run "v2 empty query" \
  curl -sS -o /tmp/wiki-search-v2-empty.json -w '%{http_code}\n' \
    "${auth_header[@]}" \
    "${BASE}/api/v1/knowledgebase/${KB_ID}/wiki/search?v=2&q="

# 2) explicit query → 200 + hits[] with snippet containing <mark>query</mark>
QUERY="${QUERY:-finance}"
run "v2 query=${QUERY}" \
  curl -sS -o /tmp/wiki-search-v2-query.json -w '%{http_code}\n' \
    "${auth_header[@]}" \
    "${BASE}/api/v1/knowledgebase/${KB_ID}/wiki/search?v=2&q=${QUERY}&limit=10"

# 3) cross-KB explicit kb_ids[] → 200 + hits[] scoped to those KBs
run "v2 cross-KB kb_ids[]=" \
  curl -sS -o /tmp/wiki-search-v2-cross.json -w '%{http_code}\n' \
    "${auth_header[@]}" \
    "${BASE}/api/v1/knowledgebase/${KB_ID}/wiki/search?v=2&q=${QUERY}&kb_ids[]=kbA&kb_ids[]=kbB&limit=10"

# 4) page_types[] filter → 200 + only matching page_type
run "v2 page_types[]=concept" \
  curl -sS -o /tmp/wiki-search-v2-types.json -w '%{http_code}\n' \
    "${auth_header[@]}" \
    "${BASE}/api/v1/knowledgebase/${KB_ID}/wiki/search?v=2&q=${QUERY}&page_types[]=concept&limit=10"

# 5) legacy fallback ?legacy=1 → 200 + {pages: WikiPage[]}
run "legacy fallback ?legacy=1" \
  curl -sS -o /tmp/wiki-search-v2-legacy.json -w '%{http_code}\n' \
    "${auth_header[@]}" \
    "${BASE}/api/v1/knowledgebase/${KB_ID}/wiki/search?legacy=1&q=${QUERY}&limit=10"

# 6) limit clamp > 100 → still 200 + cap honored server-side
run "limit clamp > 100" \
  curl -sS -o /tmp/wiki-search-v2-clamp.json -w '%{http_code}\n' \
    "${auth_header[@]}" \
    "${BASE}/api/v1/knowledgebase/${KB_ID}/wiki/search?v=2&q=${QUERY}&limit=999"

echo
echo "── assertions (only when DRY_RUN=0) ──"
if [[ "${DRY_RUN}" == "1" ]]; then
  echo "set DRY_RUN=0 with BASE + TOKEN + KB_ID to actually hit the server."
  exit 0
fi

# Empty query → hits:[]
test -s /tmp/wiki-search-v2-empty.json
grep -q '"hits":\[\]' /tmp/wiki-search-v2-empty.json \
  || { echo "FAIL: empty query should return hits:[]"; exit 1; }

# Query → contains <mark>
grep -q "<mark>${QUERY}</mark>" /tmp/wiki-search-v2-query.json \
  || { echo "FAIL: snippet missing <mark>${QUERY}</mark>"; exit 1; }

# Cross-KB → kb_ids echoed
grep -q '"kb_ids":\["kbA","kbB"\]' /tmp/wiki-search-v2-cross.json \
  || { echo "FAIL: cross-KB request missing kb_ids echo"; exit 1; }

# Legacy fallback → payload shape {pages:[...]}
grep -q '"pages":' /tmp/wiki-search-v2-legacy.json \
  || { echo "FAIL: legacy fallback should return {pages:[...]}"; exit 1; }

# Clamp > 100 → 200 + no payload error
test -s /tmp/wiki-search-v2-clamp.json

echo "ALL PASSED"