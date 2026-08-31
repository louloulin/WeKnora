#!/usr/bin/env bash
# scripts/smoke-wiki-comments.sh - v0.7.25 Build #22 end-to-end smoke test
#
# Walks the wiki page comments surface (reply / resolve / delete):
#   1. List comments on a fresh page (expect empty)
#   2. Create a top-level comment
#   3. Create a reply (parent_id set)
#   4. List again - expect 2 comments, 1 reply stat
#   5. Resolve the top-level thread
#   6. Verify stats: total_resolved=1, total_replies=1
#   7. Update the reply body
#   8. Delete the reply
#   9. Verify only the top-level comment remains
#
# Prereqs: server running on $BASE_URL with a valid $TOKEN. The slug
# "smoke-comments-page" must exist in the KB; if not, the smoke can
# create a dummy wiki page first or run against an existing one.
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
KB_ID=${KB_ID:?KB_ID environment variable required (target knowledge base UUID)}
SLUG=${SLUG:-smoke-comments-page}
TOKEN=${TOKEN:?TOKEN environment variable required}

AUTH="Authorization: Bearer ${TOKEN}"
JSON="Content-Type: application/json"

step() { printf "\n=== %s ===\n" "$1"; }
ok()   { printf "  PASS %s\n" "$1"; }
fail() { printf "  FAIL %s\n" "$1"; exit 1; }

COMMENT_PATH="${BASE_URL}/api/v1/knowledgebase/${KB_ID}/wiki/pages/${SLUG}/comments"

step "1. List comments on fresh page - expect empty"
LIST=$(curl -fsSL "$COMMENT_PATH" -H "$AUTH")
echo "$LIST" | grep -q '"comments":\[\]' && ok "no comments yet" || fail "expected empty list, got $LIST"

step "2. Create top-level comment"
TOP=$(curl -fsSL -X POST "$COMMENT_PATH" -H "$AUTH" -H "$JSON" -d '{"body":"Top-level discussion thread"}')
TOP_ID=$(echo "$TOP" | grep -oE '"id":"[^"]+"' | head -1 | cut -d'"' -f4)
[[ -n "$TOP_ID" ]] && ok "top-level comment id=$TOP_ID" || fail "creation failed: $TOP"

step "3. Create reply (parent_id)"
REPLY=$(curl -fsSL -X POST "$COMMENT_PATH" -H "$AUTH" -H "$JSON" \
    -d "{\"body\":\"This is a reply\",\"parent_id\":\"${TOP_ID}\"}")
REPLY_ID=$(echo "$REPLY" | grep -oE '"id":"[^"]+"' | head -1 | cut -d'"' -f4)
[[ -n "$REPLY_ID" ]] && ok "reply id=$REPLY_ID" || fail "reply creation failed: $REPLY"

step "4. List - expect 2 comments"
LIST2=$(curl -fsSL "$COMMENT_PATH" -H "$AUTH")
echo "$LIST2" | grep -q "\"id\":\"${TOP_ID}\"" && ok "top-level present" || fail "top-level missing"
echo "$LIST2" | grep -q "\"id\":\"${REPLY_ID}\"" && ok "reply present" || fail "reply missing"

step "5. Resolve the thread"
RESOLVED=$(curl -fsSL -X POST "${COMMENT_PATH}/${TOP_ID}/resolve" -H "$AUTH" -H "$JSON" -d '{"resolved":true}')
echo "$RESOLVED" | grep -q '"resolved_by"' && ok "resolved_by set" || fail "resolve failed: $RESOLVED"

step "6. Stats: 1 resolved, 1 reply"
echo "$LIST2" | grep -q '"total_replies":1' && ok "reply stat = 1" || fail "reply stat wrong"
LIST3=$(curl -fsSL "$COMMENT_PATH" -H "$AUTH")
echo "$LIST3" | grep -q '"total_resolved":1' && ok "resolved stat = 1" || fail "resolved stat wrong"

step "7. Update reply body"
UPDATED=$(curl -fsSL -X PUT "${COMMENT_PATH}/${REPLY_ID}" -H "$AUTH" -H "$JSON" \
    -d '{"body":"Edited reply text"}')
echo "$UPDATED" | grep -q '"body":"Edited reply text"' && ok "reply edited" || fail "update failed: $UPDATED"

step "8. Delete the reply"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "${COMMENT_PATH}/${REPLY_ID}" -H "$AUTH")
[[ "$HTTP_CODE" == "204" ]] && ok "reply deleted (204)" || fail "delete returned $HTTP_CODE"

step "9. List again - only top-level remains"
LIST4=$(curl -fsSL "$COMMENT_PATH" -H "$AUTH")
echo "$LIST4" | grep -q "\"id\":\"${TOP_ID}\"" && ok "top-level still present" || fail "top-level disappeared"
echo "$LIST4" | grep -q "\"id\":\"${REPLY_ID}\"" && fail "reply still listed after delete" || ok "reply gone"
echo "$LIST4" | grep -q '"total_replies":0' && ok "reply stat reset to 0" || fail "reply stat wrong"

printf "\n=== Wiki page comments smoke test passed ===\n"
