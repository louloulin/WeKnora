package repository

import (
	"encoding/json"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// BackrefRowsFromCachePayload builds the set of backref rows for a
// cache row's payload. The repo writes one row per unique referenced
// slug across direct_json + indirect_json + related_json — direction
// is dropped (Build #26 decided the ACL→cache hook doesn't care which
// section a slug appeared in).
//
// Returns nil when the payload has no references (empty arrays). The
// caller treats nil and an empty slice identically — no insert is
// issued in either case. We return []WikiBacklinksCacheBackrefRow{}
// only when at least one row was materialised so the caller can
// observe the diff between "no payload" and "payload with refs".
//
// Defensive parsing: malformed JSON in any of the three columns is
// logged-skipped, not fatal. The cache layer is read-mostly and a
// poison-pill row should not stop an Upsert. The validation step
// upstream (wikiPageService.ListBacklinkGraph → marshal) writes only
// canonical JSON, so a parse failure here means the row was hand-
// edited or imported from a pre-Build #21 migration — best-effort
// recovery is fine.
func BackrefRowsFromCachePayload(kbID, owningSlug string, now time.Time,
	directJSON, indirectJSON, relatedJSON string,
) []types.WikiBacklinksCacheBackrefRow {
	refs := unionReferencedSlugs(directJSON, indirectJSON, relatedJSON)
	if len(refs) == 0 {
		return nil
	}
	rows := make([]types.WikiBacklinksCacheBackrefRow, 0, len(refs))
	for _, slug := range refs {
		rows = append(rows, types.WikiBacklinksCacheBackrefRow{
			KbID:           kbID,
			ReferencedSlug: slug,
			OwningSlug:     owningSlug,
			UpdatedAt:      now,
		})
	}
	return rows
}

// unionReferencedSlugs flattens the three payload JSON arrays into a
// deduplicated, sorted slice of slug strings. Each column is a JSON
// array of strings (e.g. `["a","b"]`). Malformed JSON in any column
// is silently skipped — the cache row's payload comes from a
// marshalled list in production and a parse error indicates hand-edited
// data, which the Build #26 backfill migration also handles
// defensively (see migrations/versioned/000101_*.up.sql).
func unionReferencedSlugs(directJSON, indirectJSON, relatedJSON string) []string {
	all := make([]string, 0, 16)
	all = appendDecodedStrings(all, directJSON)
	all = appendDecodedStrings(all, indirectJSON)
	all = appendDecodedStrings(all, relatedJSON)
	if len(all) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(all))
	uniq := make([]string, 0, len(all))
	for _, s := range all {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		uniq = append(uniq, s)
	}
	if len(uniq) == 0 {
		return nil
	}
	return uniq
}

// appendDecodedStrings decodes a JSON string array into the accumulator.
// Malformed JSON returns the accumulator unchanged. json.Unmarshal is
// strict about the input shape — passing a non-array value returns an
// error and we skip it. Empty string (zero-value from a never-updated
// column) also returns unchanged.
func appendDecodedStrings(acc []string, raw string) []string {
	if raw == "" {
		return acc
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return acc
	}
	return append(acc, out...)
}