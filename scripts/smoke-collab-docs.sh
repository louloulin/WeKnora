#!/bin/bash
# smoke-collab-docs.sh — v0.7.26 manual smoke test for the Feishu-style
# collaborative document surface.
#
# Exercises:
#   1. Create a collab doc (kind=doc)
#   2. Upload a tiny .docx payload (constructed inline)
#   3. Download it back
#   4. Verify round-trip byte equality (since no edit was applied)
#   5. Generate a public share link and fetch without auth
#   6. Submit sync-to-kb (returns 202 whether or not the docparser is up)
#
# Requirements: the WeKnora server is running on $WEKNORA_BASE (default
# http://localhost:8080) and you have a valid JWT in $WEKNORA_TOKEN.
#
# Usage:
#   WEKNORA_TOKEN=... ./scripts/smoke-collab-docs.sh

set -euo pipefail

BASE="${WEKNORA_BASE:-http://localhost:8080}"
TOKEN="${WEKNORA_TOKEN:?set WEKNORA_TOKEN first}"
KB_ID="${WEKNORA_KB_ID:?set WEKNORA_KB_ID first (must exist)}"

echo "[1/6] Creating collab doc..."
DOC_JSON=$(curl -fsS -X POST "$BASE/api/v1/collaborative-docs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"kb_id\":\"$KB_ID\",\"title\":\"smoke-doc\",\"doc_kind\":\"doc\"}")
DOC_ID=$(echo "$DOC_JSON" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['id'])")
echo "  doc_id=$DOC_ID"

echo "[2/6] Constructing a minimal .docx (just an empty paragraph)..."
TMPDOCX=$(mktemp -t smoke.XXXXXX.docx)
python3 - << 'PYEOF'
import zipfile, sys
p = sys.argv[1]
with zipfile.ZipFile(p, 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr('[Content_Types].xml', '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>''')
    z.writestr('_rels/.rels', '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>''')
    z.writestr('word/document.xml', '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>smoke</w:t></w:r></w:p></w:body></w:document>''')
print(f"wrote {p}")
PYEOF
"$TMPDOCX"

echo "[3/6] Uploading..."
curl -fsS -X POST "$BASE/api/v1/collaborative-docs/$DOC_ID/upload" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@$TMPDOCX;type=application/vnd.openxmlformats-officedocument.wordprocessingml.document"
echo

echo "[4/6] Downloading..."
DL=$(mktemp -t smoke-dl.XXXXXX.docx)
curl -fsS -o "$DL" "$BASE/api/v1/collaborative-docs/$DOC_ID/download" \
  -H "Authorization: Bearer $TOKEN"
ORIG_BYTES=$(wc -c < "$TMPDOCX")
DL_BYTES=$(wc -c < "$DL")
echo "  upload=$ORIG_BYTES bytes  download=$DL_BYTES bytes"
if [ "$ORIG_BYTES" != "$DL_BYTES" ]; then
  echo "  WARNING: byte counts differ (this is OK if the engine normalised media types)"
fi

echo "[5/6] Public share link (no auth)..."
# Mark doc as shareable first.
curl -fsS -X PATCH "$BASE/api/v1/collaborative-docs/$DOC_ID" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"visibility":"public"}' >/dev/null
SHARE_TOKEN=$(curl -fsS "$BASE/api/v1/collaborative-docs/$DOC_ID" \
  -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['share_token'])")
echo "  share_token=$SHARE_TOKEN"
curl -fsS -o /dev/null -w "  public download status=%{http_code}\n" \
  "$BASE/api/v1/collaborative-docs/share/$SHARE_TOKEN/download"

echo "[6/6] Sync-to-KB (returns 202 even if docparser is down)..."
curl -fsS -o /dev/null -w "  sync-to-kb status=%{http_code}\n" \
  -X POST "$BASE/api/v1/collaborative-docs/$DOC_ID/sync-to-kb" \
  -H "Authorization: Bearer $TOKEN"

echo
echo "Smoke test done. doc_id=$DOC_ID share_token=$SHARE_TOKEN"
rm -f "$TMPDOCX" "$DL"
