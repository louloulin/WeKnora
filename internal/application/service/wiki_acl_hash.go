package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// HashAcl computes the SHA-256 fingerprint of an ACL payload, truncated
// to 16 hex characters (64 bits). PutAcl compares this fingerprint
// against the previously-stored value (read from
// wiki_pages.acl_snapshot_hash) and skips the Build #24 cache wipe +
// invalidation-log row when they match — see spec D1–D3 of
// docs/comet/changes/weknora-cache-acl-snapshot-hash/specs/acl-snapshot-hash/spec.md.
//
// Why 16 hex chars (64 bits)?
//
// The birthday-bound collision probability at 2^32 stored values is
// ~50%. WeKnora's expected per-KB wiki page count is well under 2^32,
// and the failure mode for a collision is "one wipe we should have done
// was skipped" — which self-heals on the next read (the cache row will
// recompute on miss). 64 bits is the sweet spot between collision risk
// and storage cost.
//
// Why sort the ID slices before hashing?
//
// Two callers that submit the same logical ACL in different slice
// orders (`[a, b, c]` vs `[c, b, a]`) must hash the same value or the
// skip optimization will never fire in practice. The slices are already
// JSON-encoded for the audit row's before_acl / after_acl columns, but
// Go's json.Marshal preserves slice order; we canonicalize first.
//
// Determinism:
//
// The function is intentionally pure: no maps, no time, no randomness,
// no global state. Two callers running the same input on different
// machines or Go versions must produce the same string. TestAclHash_Deterministic
// (B27-B5) exercises this property with a 1000-iteration loop.
func HashAcl(mode string, allowUserIDs, allowGroupIDs []string, denyInherited bool) string {
	// Sort a copy so the caller-owned slices are not mutated. The sort
	// is stable on Go 1.21+ for strings of equal value, which is all
	// we need.
	if allowUserIDs != nil {
		cp := make([]string, len(allowUserIDs))
		copy(cp, allowUserIDs)
		sort.Strings(cp)
		allowUserIDs = cp
	}
	if allowGroupIDs != nil {
		cp := make([]string, len(allowGroupIDs))
		copy(cp, allowGroupIDs)
		sort.Strings(cp)
		allowGroupIDs = cp
	}

	// The hash input is a stable JSON encoding. We marshal into a
	// fixed struct shape rather than building the JSON manually so
	// field order, escaping, and separators all come from
	// encoding/json (which is itself deterministic for a struct).
	buf, _ := json.Marshal(canonicalACL{
		Mode:          mode,
		AllowUserIDs:  allowUserIDs,
		AllowGroupIDs: allowGroupIDs,
		DenyInherited: denyInherited,
	})
	sum := sha256.Sum256(buf)
	full := hex.EncodeToString(sum[:])
	return full[:16]
}

// canonicalACL is the JSON shape the hash reads. The field order here
// defines the JSON key order, which matters only because we want the
// bytes-to-hash to be stable across processes — Go's encoding/json
// emits struct fields in declaration order.
type canonicalACL struct {
	Mode          string   `json:"mode"`
	AllowUserIDs  []string `json:"allow_user_ids"`
	AllowGroupIDs []string `json:"allow_group_ids"`
	DenyInherited bool     `json:"deny_inherited"`
}
