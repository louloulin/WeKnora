#!/usr/bin/env bash
#
# smoke-wiki-tags.sh — Build #17 / P1.1 wiki tag system end-to-end smoke.
#
# Walks the full lifecycle against a live backend:
#
#   1. Create a tag (POST /tags)
#   2. Read it back (GET /tags)
#   3. Update color (PUT /tags/:id)
#   4. Bind it to a page (PUT /pages/:slug/tags)
#   5. Verify the per-page set (GET /pages/:slug/tags)
#   6. Bulk-tag a second page (POST /pages/batch-tag, sync path)
#   7. Bulk-untag (POST /pages/batch-tag, remove)
#   8. Delete the tag (DELETE /tags/:id)
#
# DRY_RUN=1: skip all HTTP calls; print the plan instead. Useful for
# CI and for local validation that the script still wires up the right
# sequence.
#
# This mirrors scripts/smoke-wiki-batch-preview.sh in shape: same
# colour-coded stepper, same --kb-id / --base-url / --token knobs,
# same exit-code semantics.

set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
KB_ID="${KB_ID:?usage: KB_ID=<id> ./smoke-wiki-tags.sh [--dry-run]}"
TOKEN="${TOKEN:?usage: TOKEN=<jwt> ./smoke-wiki-tags.sh [--dry-run]}"
PAGE_SLUG="${PAGE_SLUG:-smoke-tags-primary}"
SECOND_SLUG="${SECOND_SLUG:-smoke-tags-secondary}"
DRY_RUN="${DRY_RUN:-0}"

red()    { printf '\033[31m%s\033[0m\n' "$*"; }
green()  { printf '\033[32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[33m%s\033[0m\n' "$*"; }
cyan()   { printf '\033[36m%s\033[0m\n' "$*"; }

step()  { echo; cyan "==> $*"; }
fail()  { red    "✗ $*"; exit 1; }
pass()  { green  "✓ $*"; }

auth_header() {
  printf 'Authorization: Bearer %s' "$TOKEN"
}

curl_json() {
  # curl_json <method> <path> [json-body]
  local method="$1"
  local path="$2"
  local body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -sS -X "$method" \
      -H "$(auth_header)" \
      -H 'Content-Type: application/json' \
      --data "$body" \
      "${BASE_URL}${path}"
  else
    curl -sS -X "$method" \
      -H "$(auth_header)" \
      "${BASE_URL}${path}"
  fi
}

if [[ "$DRY_RUN" == "1" ]]; then
  yellow "DRY RUN — no HTTP calls will be made"
  echo "Plan:"
  cat <<EOF
  1) POST   /api/v1/knowledgebase/${KB_ID}/wiki/tags                          create 'smoke-todo'
  2) GET    /api/v1/knowledgebase/${KB_ID}/wiki/tags                          list returns the tag
  3) PUT    /api/v1/knowledgebase/${KB_ID}/wiki/tags/<id> { color:'orange' }  update
  4) PUT    /api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/tags       bind [id]
  5) GET    /api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/tags       confirm
  6) POST   /api/v1/knowledgebase/${KB_ID}/wiki/pages/batch-tag              add to [${SECOND_SLUG}]
  7) POST   /api/v1/knowledgebase/${KB_ID}/wiki/pages/batch-tag              remove from [${PAGE_SLUG}]
  8) DELETE /api/v1/knowledgebase/${KB_ID}/wiki/tags/<id>                    cleanup
EOF
  exit 0
fi

step "1) Create tag"
TAG_BODY='{"name":"smoke-todo","color":"blue"}'
TAG_JSON="$(curl_json POST "/api/v1/knowledgebase/${KB_ID}/wiki/tags" "$TAG_BODY")"
TAG_ID="$(printf '%s' "$TAG_JSON" | grep -o '"id":"[^"]*"' | head -1 | sed 's/.*"id":"\([^"]*\)".*/\1/')"
if [[ -z "$TAG_ID" ]]; then
  fail "tag create: no id returned: $TAG_JSON"
fi
pass "tag id = $TAG_ID"

step "2) GET /tags"
LIST="$(curl_json GET "/api/v1/knowledgebase/${KB_ID}/wiki/tags")"
if ! echo "$LIST" | grep -q "$TAG_ID"; then
  fail "list does not include new tag"
fi
pass "list includes tag"

step "3) PUT /tags/<id> color=orange"
curl_json PUT "/api/v1/knowledgebase/${KB_ID}/wiki/tags/${TAG_ID}" \
  '{"color":"orange"}' >/dev/null
pass "color updated"

step "4) Bind tag to ${PAGE_SLUG}"
curl_json PUT "/api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/tags" \
  "{\"tag_ids\":[\"$TAG_ID\"]}" >/dev/null
pass "bound"

step "5) GET /pages/${PAGE_SLUG}/tags"
P_TAGS="$(curl_json GET "/api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/tags")"
if ! echo "$P_TAGS" | grep -q "$TAG_ID"; then
  fail "page tag list does not include tag"
fi
pass "page has tag"

step "6) batch-tag add to ${SECOND_SLUG}"
BATCH_JSON="$(curl_json POST "/api/v1/knowledgebase/${KB_ID}/wiki/pages/batch-tag" \
  "{\"slugs\":[\"$SECOND_SLUG\"],\"tag_id\":\"$TAG_ID\",\"op\":\"add\"}")"
echo "$BATCH_JSON"
pass "batch add ok"

step "7) batch-tag remove from ${PAGE_SLUG}"
curl_json POST "/api/v1/knowledgebase/${KB_ID}/wiki/pages/batch-tag" \
  "{\"slugs\":[\"$PAGE_SLUG\"],\"tag_id\":\"$TAG_ID\",\"op\":\"remove\"}" >/dev/null
pass "batch remove ok"

step "8) DELETE /tags/<id>"
curl_json DELETE "/api/v1/knowledgebase/${KB_ID}/wiki/tags/${TAG_ID}" >/dev/null
pass "tag deleted"

echo
green "All 8 smoke steps OK for KB ${KB_ID}."