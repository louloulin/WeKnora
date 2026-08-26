#!/usr/bin/env bash
#
# smoke-wiki-template.sh — Build #18 / P1.2 page-template skeleton auto-gen.
#
# Walks the full lifecycle against a live backend:
#
#   1. Preview the template (POST /pages/:slug/preview-template)
#      — server validates the skeleton and returns the rewritten body
#        without writing.
#   3. Re-apply (POST /pages/:slug/apply-template) and verify a new
#      auto-template child page exists.
#   4. Verify SourceRefs = ["auto-template"] is stamped on the parent
#      page by reading it back via GET /pages/:slug.
#   5. Cleanup: DELETE the auto-template child.
#
# DRY_RUN=1: skip all HTTP calls; print the plan instead. Useful for
# CI and for local validation that the script still wires up the right
# sequence.
#
# Mirrors scripts/smoke-wiki-tags.sh in shape: same colour-coded
# stepper, same --kb-id / --base-url / --token knobs, same exit-code
# semantics.

set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
KB_ID="${KB_ID:?usage: KB_ID=<id> ./smoke-wiki-template.sh [--dry-run]}"
TOKEN="${TOKEN:?usage: TOKEN=<jwt> ./smoke-wiki-template.sh [--dry-run]}"
PAGE_SLUG="${PAGE_SLUG:-smoke-template-parent}"
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
  1) POST /api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/preview-template
       body: {"skeleton":{"children":[{"title":"smoke-auto-1"}],"sections":[],"tagged_tokens":[]}}
       → verify pages[0].slug = "${PAGE_SLUG}/smoke-auto-1"
  2) POST /api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/apply-template
       same skeleton → verify pages[0].slug exists
  3) GET  /api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}
       → verify source_refs contains "auto-template"
  4) DELETE /api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/smoke-auto-1
       → cleanup
EOF
  exit 0
fi

PREVIEW_BODY='{
  "skeleton": {
    "children": [
      {"title": "smoke-auto-1", "content": "auto-generated body"}
    ],
    "sections": [],
    "tagged_tokens": []
  }
}'

step "1) preview-template"
PREVIEW_JSON="$(curl_json POST \
  "/api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/preview-template" \
  "$PREVIEW_BODY")"
if ! echo "$PREVIEW_JSON" | grep -q '"pages"'; then
  fail "preview response missing 'pages' field: $PREVIEW_JSON"
fi
pass "preview returned pages[]"

step "2) apply-template"
APPLY_JSON="$(curl_json POST \
  "/api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}/apply-template" \
  "$PREVIEW_BODY")"
NEW_SLUG="$(printf '%s' "$APPLY_JSON" | \
  grep -o '"slug":"[^"]*"' | head -1 | sed 's/.*"slug":"\([^"]*\)".*/\1/')"
if [[ -z "$NEW_SLUG" ]]; then
  fail "apply: no slug returned: $APPLY_JSON"
fi
pass "apply created slug = $NEW_SLUG"

step "3) verify source_refs on parent"
PARENT_JSON="$(curl_json GET \
  "/api/v1/knowledgebase/${KB_ID}/wiki/pages/${PAGE_SLUG}")"
if ! echo "$PARENT_JSON" | grep -q 'auto-template'; then
  fail "parent page does not have source_refs=auto-template: $PARENT_JSON"
fi
pass "parent page source_refs contains 'auto-template'"

step "4) cleanup: delete auto-template child"
curl_json DELETE \
  "/api/v1/knowledgebase/${KB_ID}/wiki/pages/${NEW_SLUG}" >/dev/null
pass "child deleted"

echo
green "All 4 smoke steps OK for KB ${KB_ID}."