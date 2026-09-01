package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// shared.go holds the helpers used by every per-type Tencent Docs connector
// (doc/, sheet/, slide/, form/): error classification, stream-Checkpoint
// tuning, the fetch tally, and the cursor wire format. Anything specific to
// one connector (e.g. docx blocks conversion) stays in that connector's own
// file - this package is the common toolkit only.

// TencentDocsStreamCheckpointInterval is how many processed documents pass
// between cursor checkpoints during a streaming fetch. Small enough that a
// timed-out sync loses little work on resume, large enough that Checkpoint
// persistence (a DB write) does not dominate. Overridable in tests.
var TencentDocsStreamCheckpointInterval = 50

// TencentDocsStreamCheckpointMaxInterval bounds checkpointing by wall-clock
// time as well as document count. Without it, a sync of fewer than
// TencentDocsStreamCheckpointInterval very slow (rate-limited) exports could
// reach the task timeout having never checkpointed, and resume from scratch
// every retry - the same #2136 scenario the Feishu engine guards against.
var TencentDocsStreamCheckpointMaxInterval = 30 * time.Second

// FetchTally accumulates the outcome of fetching a document set so the
// connector can emit a single actionable summary. Without it, unsupported
// documents (forms / mindmaps) vanish with no item, no error and no log.
type FetchTally struct {
	discovered    int
	fetched       int
	failed        int
	skippedByType map[string]int
}

func NewFetchTally(discovered int) *FetchTally {
	return &FetchTally{discovered: discovered, skippedByType: map[string]int{}}
}

// newFetchTally is the package-internal alias used by the copied engine
// (which was written against feishu/core's lowercase symbol). Keeping both
// names means future engine tweaks can switch to the exported form without
// a renames pass.
func newFetchTally(discovered int) *FetchTally { return NewFetchTally(discovered) }

func (t *FetchTally) Fetch()              { t.fetched++ }
func (t *FetchTally) Fail()               { t.failed++ }
func (t *FetchTally) Skip(docType string) { t.skippedByType[docType]++ }

// Lowercase aliases consumed by the copied engine (engine.go was written
// against feishu/core's lowercase symbols). Keeping both names means the
// public API stays idiomatic Go while the internal engine needs no edits.
func (t *FetchTally) fetch()               { t.Fetch() }
func (t *FetchTally) fail()                { t.Fail() }
func (t *FetchTally) summary() string      { return t.Summary() }

func (t *FetchTally) Skipped() int {
	n := 0
	for _, c := range t.skippedByType {
		n += c
	}
	return n
}

func (t *FetchTally) Summary() string {
	return fmt.Sprintf("discovered=%d fetched=%d failed=%d skipped_unsupported=%d by_type=%v",
		t.discovered, t.fetched, t.failed, t.Skipped(), t.skippedByType)
}

// Failure classifies a raw connector/API error into a stable i18n code, an
// optional numeric error code for interpolation, and an English fallback
// message for clients without the i18n key. The raw status/JSON body is
// never returned here - it stays in the server logs. Mirrors
// feishu/core.feishuFailure so the two connectors present a uniform error
// vocabulary to the frontend.
func Failure(err error) (code, codeValue, fallback string) {
	if err == nil {
		return "sync_failed", "", "Sync failed; will retry on the next sync"
	}
	s := strings.ToLower(err.Error())

	switch {
	case strings.Contains(s, "auth error"),
		strings.Contains(s, "invalid access token"),
		strings.Contains(s, "permission"),
		strings.Contains(s, "forbidden"),
		strings.Contains(s, "status=403"):
		return "tencent_docs_auth_or_permission", "", "Authentication or permission error; check client_id / client_secret and scopes"
	case strings.Contains(s, "rate limited"), strings.Contains(s, "status=429"):
		return "tencent_docs_rate_limited", "", "Tencent Docs API rate limited; will retry on the next sync"
	case strings.Contains(s, "timed out"),
		strings.Contains(s, "timeout"),
		strings.Contains(s, "deadline exceeded"):
		return "tencent_docs_timeout", "", "Export or request timed out; will retry on the next sync"
	case strings.Contains(s, "server error"):
		return "tencent_docs_server_unavailable", "", "Tencent Docs service temporarily unavailable; will retry on the next sync"
	case strings.Contains(s, "api error"),
		strings.Contains(s, "download failed"):
		return "tencent_docs_api_error", "", "Tencent Docs API error; will retry on the next sync"
	default:
		return "tencent_docs_sync_failed", "", "Sync failed; will retry on the next sync"
	}
}

// ErrorItemMeta wraps a connector/API error into the FetchedItem metadata
// shape the rest of the pipeline expects. Mirrors feishu/core.FeishuErrorItemMeta
// so downstream consumers (frontend i18n, sync log rendering) can treat
// Tencent Docs and Feishu errors with the same code paths.
func ErrorItemMeta(err error, extra map[string]string) map[string]string {
	code, codeValue, fallback := Failure(err)
	meta := map[string]string{
		"error_code":    code,
		"error_message": fallback,
	}
	if codeValue != "" {
		meta["error_code_value"] = codeValue
	}
	for k, v := range extra {
		meta[k] = v
	}
	return meta
}

// Cursor is the wire-format persisted by the engine to enable resumable
// incremental sync. It records the per-resource (drive / folder) last-seen
// edit time for every document token, plus the wall-clock last sync time.
//
// Same shape as feishu/core.FeishuCursor so the service-layer storage and
// the existing snapshot-isolation logic apply unchanged.
type Cursor struct {
	LastSyncTime time.Time                  `json:"last_sync_time"`
	ResourceTimes map[string]map[string]string `json:"resource_times"`
}

// MarshalToMap encodes the cursor into the generic map[string]interface{}
// the engine expects. JSON round-trip is intentional: it matches the Feishu
// pattern, isolates the wire format from runtime mutations, and makes the
// snapshot safe to keep mutating after Checkpoint returns.
func (c *Cursor) MarshalToMap() map[string]interface{} {
	m := make(map[string]interface{})
	b, _ := json.Marshal(c)
	_ = json.Unmarshal(b, &m)
	return m
}

// DecodeCursorTimes parses the generic map back into a Cursor. Returns nil
// when the map is absent or empty (caller treats nil as "no prior cursor").
func DecodeCursorTimes(m map[string]interface{}) map[string]map[string]string {
	if len(m) == 0 {
		return nil
	}
	var cur Cursor
	b, _ := json.Marshal(m)
	if err := json.Unmarshal(b, &cur); err != nil {
		return nil
	}
	return cur.ResourceTimes
}

// EncodeCursor wraps an engine-internal times map + lastSync into a
// types.SyncCursor ready for persistence. Mirrors feishu/wiki.wikiOps.EncodeCursor.
func EncodeCursor(times map[string]map[string]string, lastSync time.Time) *types.SyncCursor {
	cur := Cursor{LastSyncTime: lastSync, ResourceTimes: times}
	return &types.SyncCursor{
		LastSyncTime:     lastSync,
		ConnectorCursor: cur.MarshalToMap(),
	}
}

// WebDocURL builds the user-facing link to a Tencent Docs document on the
// main web origin. Doc IDs are type-prefixed (D.../S.../B.../F...) so we
// link by ID directly.
func WebDocURL(docID string) string {
	return tencentDocsWebBaseURL + "/doc/" + docID
}
