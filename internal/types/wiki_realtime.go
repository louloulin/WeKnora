package types

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// WikiRealtimeSnapshot is the durable Yjs document state for one wiki page.
// WeKnora stores the binary Y.encodeStateAsUpdate output plus a vector clock
// so a fresh server can re-hydrate the Yjs doc without recomputing history.
//
// The struct is JSON-marshallable for REST read endpoints, but the binary
// fields use base64 so they survive JSON transports (e.g. the management
// "force snapshot" endpoint and the snapshot audit log).
type WikiRealtimeSnapshot struct {
	ID          uint64    `json:"id"`
	TenantID    uint64    `json:"tenant_id" gorm:"index"`
	KBID        string    `json:"kb_id" gorm:"type:varchar(36);index"`
	PageID      string    `json:"page_id" gorm:"type:varchar(36);index"`
	YDocState   []byte    `json:"ydoc_state_base64" gorm:"column:ydoc_state;type:bytea"`
	VectorClock []byte    `json:"vector_clock_base64" gorm:"column:vector_clock;type:bytea"`
	Version     int64     `json:"version" gorm:"default:1"`
	SizeBytes   int       `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MarshalJSON encodes the binary fields as base64 so JSON callers can
// round-trip without extra plumbing.
func (s WikiRealtimeSnapshot) MarshalJSONBytes() ([]byte, error) {
	type alias WikiRealtimeSnapshot
	ydoc := base64.StdEncoding.EncodeToString(s.YDocState)
	vc := base64.StdEncoding.EncodeToString(s.VectorClock)
	return json.Marshal(struct {
		alias
		YDocStateBase64 string `json:"ydoc_state"`
		VectorClockBase string `json:"vector_clock"`
	}{
		alias:           alias(s),
		YDocStateBase64: ydoc,
		VectorClockBase: vc,
	})
}

// WikiRealtimeSession is one live presence row. (user_id, client_id) is
// unique per page; the server upserts on join and refreshes last_heartbeat
// every ~10s while the WebSocket is alive. The Color and DisplayName are
// denormalized so the presence panel can render without joining to the user
// table on every snapshot read.
type WikiRealtimeSession struct {
	ID            string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID      uint64    `json:"tenant_id" gorm:"index"`
	PageID        string    `json:"page_id" gorm:"type:varchar(36);index"`
	UserID        uint64    `json:"user_id" gorm:"index"`
	ClientID      uint64    `json:"client_id"`
	Color         string    `json:"color" gorm:"type:varchar(16)"`
	DisplayName   string    `json:"display_name" gorm:"type:varchar(128)"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	JoinedAt      time.Time `json:"joined_at"`
}

// WikiRealtimeSnapshotUpsert is the shape used by the snapshot service when
// saving a freshly compacted Yjs document. The service is responsible for
// computing size_bytes and the new vector clock before calling the repo.
type WikiRealtimeSnapshotUpsert struct {
	TenantID    uint64
	KBID        string
	PageID      string
	YDocState   []byte
	VectorClock []byte
	SizeBytes   int
}

// Validate enforces the non-empty invariants the snapshot repo relies on.
// Returns a typed error so callers can map to HTTP 400 without string matching.
func (u WikiRealtimeSnapshotUpsert) Validate() error {
	if u.TenantID == 0 {
		return ErrWikiRealtimeInvalid("tenant_id is required")
	}
	if u.KBID == "" {
		return ErrWikiRealtimeInvalid("kb_id is required")
	}
	if u.PageID == "" {
		return ErrWikiRealtimeInvalid("page_id is required")
	}
	if len(u.YDocState) == 0 {
		return ErrWikiRealtimeInvalid("ydoc_state is required")
	}
	if u.SizeBytes <= 0 {
		return ErrWikiRealtimeInvalid("size_bytes must be positive")
	}
	return nil
}


// TableName tells GORM to use the wiki_doc_snapshots table for this model.
func (WikiRealtimeSnapshot) TableName() string { return "wiki_doc_snapshots" }


// TableName tells GORM to use the wiki_realtime_sessions table for this model.
func (WikiRealtimeSession) TableName() string { return "wiki_realtime_sessions" }

// ErrWikiRealtimeInvalid is the typed validation error used by the realtime
// package. Service layer wraps this with fmt.Errorf so HTTP handlers can
// detect it via errors.Is and return 400.
type ErrWikiRealtimeInvalid string

func (e ErrWikiRealtimeInvalid) Error() string {
	return fmt.Sprintf("wiki realtime invalid: %s", string(e))
}
