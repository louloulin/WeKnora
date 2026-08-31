package connector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- in-memory fakes ---

type stubConnectorRepo struct {
	rows map[uint64]*types.IngestConnector
	seq  uint64
}

func newStubConnectorRepo() *stubConnectorRepo {
	return &stubConnectorRepo{rows: map[uint64]*types.IngestConnector{}}
}

func (s *stubConnectorRepo) Create(_ context.Context, c *types.IngestConnector) error {
	s.seq++
	c.ID = s.seq
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	s.rows[c.ID] = c
	return nil
}
func (s *stubConnectorRepo) Update(_ context.Context, c *types.IngestConnector) error {
	c.UpdatedAt = time.Now()
	s.rows[c.ID] = c
	return nil
}
func (s *stubConnectorRepo) Get(_ context.Context, tenantID string, id uint64) (*types.IngestConnector, error) {
	if v, ok := s.rows[id]; ok && v.TenantID == tenantID {
		return v, nil
	}
	return nil, nil
}
func (s *stubConnectorRepo) List(_ context.Context, tenantID string, limit, offset int) ([]*types.IngestConnector, int, error) {
	out := []*types.IngestConnector{}
	for _, v := range s.rows {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	return out, len(out), nil
}
func (s *stubConnectorRepo) Delete(_ context.Context, tenantID string, id uint64) error {
	if v, ok := s.rows[id]; ok && v.TenantID == tenantID {
		delete(s.rows, id)
		return nil
	}
	return errors.New("not found")
}
func (s *stubConnectorRepo) TouchSync(_ context.Context, id uint64, lastSyncAt time.Time, lastErr string) error {
	if v, ok := s.rows[id]; ok {
		v.LastSyncAt = &lastSyncAt
		v.LastError = lastErr
		v.UpdatedAt = lastSyncAt
	}
	return nil
}

type stubJobRepo struct {
	rows map[uint64]*types.IngestJob
	seq  uint64
}

func newStubJobRepo() *stubJobRepo {
	return &stubJobRepo{rows: map[uint64]*types.IngestJob{}}
}

func (s *stubJobRepo) Create(_ context.Context, job *types.IngestJob) error {
	s.seq++
	job.ID = s.seq
	job.CreatedAt = time.Now()
	s.rows[job.ID] = job
	return nil
}
func (s *stubJobRepo) UpdateJob(_ context.Context, job *types.IngestJob) error {
	s.rows[job.ID] = job
	return nil
}
func (s *stubJobRepo) Get(_ context.Context, id uint64) (*types.IngestJob, error) {
	if v, ok := s.rows[id]; ok {
		return v, nil
	}
	return nil, nil
}
func (s *stubJobRepo) ListByConnector(_ context.Context, tenantID string, connectorID uint64, limit, offset int) ([]*types.IngestJob, int, error) {
	out := []*types.IngestJob{}
	for _, v := range s.rows {
		if v.TenantID == tenantID && v.ConnectorID == connectorID {
			out = append(out, v)
		}
	}
	return out, len(out), nil
}
func (s *stubJobRepo) ListByTenant(_ context.Context, tenantID string, limit, offset int) ([]*types.IngestJob, int, error) {
	out := []*types.IngestJob{}
	for _, v := range s.rows {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	return out, len(out), nil
}

type stubIngester struct {
	items []string
}

func (s *stubIngester) Ingest(_ context.Context, _, _, title, content, _, _ string, _ time.Time) error {
	s.items = append(s.items, title+"|"+content)
	return nil
}

// --- tests ---

func TestService_Create_HappyPath(t *testing.T) {
	svc := newServiceForTest()
	svc.Register(&SlackConnector{})

	conn := &types.IngestConnector{
		TenantID: "t1", Name: "team-eng",
		Kind: types.ConnectorSlack,
		Config: `{"channel":"C01234567","messages":[]}`,
		CreatedBy: "u1",
	}
	if err := svc.Create(context.Background(), conn); err != nil {
		t.Fatalf("create: %v", err)
	}
	if conn.ID == 0 {
		t.Error("expected non-zero id")
	}
}

func TestService_Create_RejectsUnknownKind(t *testing.T) {
	svc := newServiceForTest()
	err := svc.Create(context.Background(), &types.IngestConnector{
		TenantID: "t1", Name: "x",
		Kind: "not-a-real-kind",
	})
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("expected ErrUnknownKind, got %v", err)
	}
}

func TestService_Create_RejectsEmptyName(t *testing.T) {
	svc := newServiceForTest()
	svc.Register(&SlackConnector{})
	err := svc.Create(context.Background(), &types.IngestConnector{
		TenantID: "t1", Name: "  ",
		Kind: types.ConnectorSlack,
	})
	if !errors.Is(err, ErrEmptyName) {
		t.Fatalf("expected ErrEmptyName, got %v", err)
	}
}

func TestService_Trigger_FetchesSlackMessages(t *testing.T) {
	svc, ing := newServiceForTestWithIngester()
	svc.Register(&SlackConnector{})
	conn := &types.IngestConnector{
		TenantID: "t1", Name: "team", Kind: types.ConnectorSlack,
		Config: `{"channel":"C01234567","messages":[{"ts":"1700000001.000100","text":"hello","user":"U123"}]}`,
		CreatedBy: "u1", Enabled: true,
	}
	if err := svc.Create(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	job, err := svc.Trigger(context.Background(), "t1", conn.ID)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if job.ResultCount != 1 {
		t.Errorf("result_count = %d, want 1", job.ResultCount)
	}
	if job.Status != types.IngestJobSucceeded {
		t.Errorf("status = %s, want succeeded", job.Status)
	}
	if len(ing.items) != 1 {
		t.Errorf("ingester received %d items, want 1", len(ing.items))
	}
}

func TestService_Trigger_FetchesEmailMessages(t *testing.T) {
	svc, ing := newServiceForTestWithIngester()
	svc.Register(EmailConnector{})
	conn := &types.IngestConnector{
		TenantID: "t1", Name: "support", Kind: types.ConnectorEmail,
		Config: `{"mailbox":"support@example.com","messages":[{"message_id":"<abc@example.com>","from":"alice@example.com","subject":"Login","date":"2026-08-30T12:00:00Z","body":"help"}]}`,
		CreatedBy: "u1", Enabled: true,
	}
	if err := svc.Create(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	job, err := svc.Trigger(context.Background(), "t1", conn.ID)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if job.ResultCount != 1 {
		t.Errorf("result_count = %d, want 1", job.ResultCount)
	}
	if len(ing.items) != 1 {
		t.Errorf("ingester items = %d, want 1", len(ing.items))
	}
}

func TestService_Trigger_FailsOnDisabled(t *testing.T) {
	svc, _ := newServiceForTestWithIngester()
	svc.Register(&SlackConnector{})
	conn := &types.IngestConnector{
		TenantID: "t1", Name: "x", Kind: types.ConnectorSlack,
		Config: `{}`, CreatedBy: "u1", Enabled: false,
	}
	_ = svc.Create(context.Background(), conn)
	_, err := svc.Trigger(context.Background(), "t1", conn.ID)
	if !errors.Is(err, ErrConnectorDisabled) {
		t.Fatalf("expected ErrConnectorDisabled, got %v", err)
	}
}

func TestService_Trigger_RecordsFetchError(t *testing.T) {
	svc, _ := newServiceForTestWithIngester()
	svc.Register(&SlackConnector{})
	conn := &types.IngestConnector{
		TenantID: "t1", Name: "x", Kind: types.ConnectorSlack,
		Config: `{`, // invalid JSON
		CreatedBy: "u1", Enabled: true,
	}
	_ = svc.Create(context.Background(), conn)
	job, err := svc.Trigger(context.Background(), "t1", conn.ID)
	if err != nil {
		t.Fatalf("trigger should return job even on fetch err: %v", err)
	}
	if job.Status != types.IngestJobFailed {
		t.Errorf("status = %s, want failed", job.Status)
	}
	if job.Error == "" {
		t.Error("expected error message on job")
	}
}

func TestService_Kinds_ReflectRegisteredConnectors(t *testing.T) {
	svc := newServiceForTest()
	svc.Register(&SlackConnector{})
	svc.Register(EmailConnector{})
	kinds := svc.Kinds()
	if len(kinds) != 2 {
		t.Errorf("kinds count = %d, want 2", len(kinds))
	}
}

func TestSlackConnector_RejectsEmptyConfig(t *testing.T) {
	slack := &SlackConnector{}
	_, err := slack.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{})
	if err == nil {
		t.Error("expected error on empty config")
	}
}

func TestSlackConnector_ParsesMessages(t *testing.T) {
	cfg := `{"channel":"C01234567","messages":[{"ts":"1700000001.000100","text":"hello world","user":"U123"}]}`
	slack := &SlackConnector{}
	got, err := slack.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorSlack, ConfigJSON: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].ID != "1700000001.000100" {
		t.Errorf("id = %q", got[0].ID)
	}
	if got[0].Author != "U123" {
		t.Errorf("author = %q, want U123", got[0].Author)
	}
}

func TestEmailConnector_ParsesMessages(t *testing.T) {
	cfg := `{"mailbox":"help@example.com","messages":[{"message_id":"<x@y>","from":"alice@example.com","subject":"Login","date":"2026-08-30T12:00:00Z","body":"hi"}]}`
	got, err := EmailConnector{}.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorEmail, ConfigJSON: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Title != "Email — Login" {
		t.Errorf("title = %q", got[0].Title)
	}
	if got[0].Author != "alice@example.com" {
		t.Errorf("author = %q", got[0].Author)
	}
}

func TestWebhookConnector_ParsesItems(t *testing.T) {
	cfg := `{"token":"abc","items":[{"id":"e1","title":"Deploy","content":"v1.2","author":"ci"}]}`
	wh := &WebhookConnector{}
	got, err := wh.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorWebhook, ConfigJSON: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
}

// --- helpers ---

func newServiceForTest() *Service {
	return NewService(newStubConnectorRepo(), newStubJobRepo(), nil)
}

func newServiceForTestWithIngester() (*Service, *stubIngester) {
	ing := &stubIngester{}
	return NewService(newStubConnectorRepo(), newStubJobRepo(), ing), ing
}

// interfaces compliance
var (
	_ interfaces.IngestConnectorRepository = (*stubConnectorRepo)(nil)
	_ interfaces.IngestJobRepository       = (*stubJobRepo)(nil)
)
