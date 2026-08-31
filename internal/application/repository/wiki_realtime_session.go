package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	)

// wikiRealtimeSessionRepository owns presence rows for the Yjs awareness
// protocol. Sessions are heartbeat-driven and short-lived; the application
// layer schedules the sweeper that calls SweepOlderThan.
type wikiRealtimeSessionRepository struct {
	db *gorm.DB
}

// NewWikiRealtimeSessionRepository returns the GORM-backed presence repo.
func NewWikiRealtimeSessionRepository(db *gorm.DB) interfaces.WikiRealtimeSessionRepository {
	return &wikiRealtimeSessionRepository{db: db}
}

func (r *wikiRealtimeSessionRepository) Upsert(ctx context.Context, s *types.WikiRealtimeSession) error {
	if s == nil || s.TenantID == 0 || s.PageID == "" {
		return fmt.Errorf("session upsert: missing required fields")
	}
	if s.ID == "" {
		s.ID = newSessionID()
	}
	if s.JoinedAt.IsZero() {
		s.JoinedAt = time.Now().UTC()
	}
	s.LastHeartbeat = time.Now().UTC()
	dialect := r.db.Dialector.Name()
	var err error
	if dialect == "sqlite" {
		err = r.db.WithContext(ctx).Exec(
			`INSERT OR REPLACE INTO wiki_realtime_sessions
			 (id, tenant_id, page_id, user_id, client_id, color, display_name, last_heartbeat, joined_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.TenantID, s.PageID, s.UserID, s.ClientID, s.Color, s.DisplayName, s.LastHeartbeat, s.JoinedAt,
		).Error
	} else {
		err = r.db.WithContext(ctx).Exec(
			`INSERT INTO wiki_realtime_sessions (id, tenant_id, page_id, user_id, client_id, color, display_name, last_heartbeat, joined_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (tenant_id, page_id, client_id)
			 DO UPDATE SET color = EXCLUDED.color, display_name = EXCLUDED.display_name, last_heartbeat = EXCLUDED.last_heartbeat`,
			s.ID, s.TenantID, s.PageID, s.UserID, s.ClientID, s.Color, s.DisplayName, s.LastHeartbeat, s.JoinedAt,
		).Error
	}
	if err != nil {
		logger.Errorf(ctx, "wiki realtime session upsert failed: tenant=%d page=%s client=%d err=%v",
			s.TenantID, s.PageID, s.ClientID, err)
		return fmt.Errorf("upsert session: %w", err)
	}
	return nil
}

func (r *wikiRealtimeSessionRepository) ListByPage(ctx context.Context, tenantID uint64, pageID string, since time.Time) ([]*types.WikiRealtimeSession, error) {
	var rows []*types.WikiRealtimeSession
	q := `SELECT id, tenant_id, page_id, user_id, client_id, color, display_name, last_heartbeat, joined_at
		FROM wiki_realtime_sessions WHERE tenant_id = ? AND page_id = ?`
	args := []interface{}{tenantID, pageID}
	if !since.IsZero() {
		q += ` AND last_heartbeat >= ?`
		args = append(args, since)
	}
	q += ` ORDER BY last_heartbeat DESC`
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return rows, nil
}

func (r *wikiRealtimeSessionRepository) DeleteByClient(ctx context.Context, tenantID uint64, pageID string, clientID uint64) error {
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM wiki_realtime_sessions WHERE tenant_id = ? AND page_id = ? AND client_id = ?`,
		tenantID, pageID, clientID,
	)
	if res.Error != nil {
		return fmt.Errorf("delete session: %w", res.Error)
	}
	return nil
}

func (r *wikiRealtimeSessionRepository) SweepOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM wiki_realtime_sessions WHERE last_heartbeat < ?`, cutoff,
	)
	if res.Error != nil {
		return 0, fmt.Errorf("sweep sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// newSessionID returns a UUID-style identifier for the session primary key.
// The realtime package prefers a lightweight random string over importing
// google/uuid to keep the runtime dependency surface small.
func newSessionID() string {
	return fmt.Sprintf("wrs_%d_%d", time.Now().UnixNano(), fastRand())
}

// fastRand returns a quick pseudo-random int for ID uniqueness. Not crypto
// safe, but adequate for a presence key. Real entropy lives in client_id.
func fastRand() int {
	n := time.Now().UnixNano()
	return int((n>>16) ^ n)
}

var _ interfaces.WikiRealtimeSessionRepository = (*wikiRealtimeSessionRepository)(nil)
