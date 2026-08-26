package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// stubWikiAclRepo is a minimal in-memory WikiAclRepo used by the unit
// tests below. It exercises only the methods WikiAclService actually
// touches; the rest of WikiPageRepository is irrelevant to ACL.
type stubWikiAclRepo struct {
	acl        *types.WikiPageAcl
	owner      string
	adminFor   map[string]bool // userID -> isAdmin
	groups     map[string][]string // groupID -> []userID
	storedRev  int64
	updateErr  error
}

func newStubRepo(acl *types.WikiPageAcl, owner string) *stubWikiAclRepo {
	return &stubWikiAclRepo{acl: acl, owner: owner, adminFor: map[string]bool{}, groups: map[string][]string{}}
}

func (s *stubWikiAclRepo) GetAclBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPageAcl, error) {
	if s.acl == nil {
		return nil, nil
	}
	copy := *s.acl
	return &copy, nil
}

func (s *stubWikiAclRepo) UpdateAclWithRevision(ctx context.Context, kbID string, slug string,
	newAcl types.WikiPageAcl, expectedRevision int64, snapshotHash string,
	actorUserID string, actorRole string, action string) (*types.WikiPageAcl, error) {
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.acl != nil && s.acl.Revision != expectedRevision {
		return nil, types.ErrWikiPageAclRevisionConflict
	}
	s.storedRev = expectedRevision + 1
	merged := newAcl
	merged.Revision = s.storedRev
	// Build #27 — propagate the snapshot hash onto the stored value so
	// the next PutAcl can compare-and-skip via GetAclBySlug returning
	// this value back.
	merged.SnapshotHash = snapshotHash
	s.acl = &merged
	return &merged, nil
}

func (s *stubWikiAclRepo) PageOwnerAndAdmin(ctx context.Context, kbID string, slug string, callerUserID string) (string, bool, error) {
	return s.owner, s.adminFor[callerUserID], nil
}

func (s *stubWikiAclRepo) GroupMembers(ctx context.Context, tenantID uint64, groupIDs []string) ([]string, error) {
	out := []string{}
	for _, gid := range groupIDs {
		out = append(out, s.groups[gid]...)
	}
	return out, nil
}

// stubUserService returns canned users for SearchUsers. The ACL service
// only uses SearchUsers via SearchAclCandidates, so the rest of the
// UserService surface is left as zero values.
type stubUserService struct {
	users []*types.User
}

func (s *stubUserService) SearchUsers(ctx context.Context, query string, limit int) ([]*types.User, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if len(s.users) <= limit {
		out := make([]*types.User, len(s.users))
		copy(out, s.users)
		return out, nil
	}
	return s.users[:limit], nil
}

func withUserAndTenant(ctx context.Context, userID string, tenantID uint64) context.Context {
	ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
	ctx = context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	return ctx
}

// TestResolve_Inherit: legacy NULL row grants every KB member allow.
func TestResolve_Inherit(t *testing.T) {
	repo := newStubRepo(nil, "owner-1")
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "alice", 1)
	got, err := svc.Resolve(ctx, "kb1", "slug1", "alice")
	if err != nil {
		t.Fatalf("Resolve(inherit): %v", err)
	}
	if got != types.WikiPageAclAllow {
		t.Fatalf("Resolve(inherit) = %q, want allow", got)
	}
}

// TestResolve_Private_NonOwner: private mode denies a non-owner, non-admin.
func TestResolve_Private_NonOwner(t *testing.T) {
	repo := newStubRepo(&types.WikiPageAcl{
		Mode:     types.WikiPageAclModePrivate,
		Revision: 1,
	}, "owner-1")
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "alice", 1)
	got, err := svc.Resolve(ctx, "kb1", "slug1", "alice")
	if err != nil {
		t.Fatalf("Resolve(private,non-owner): %v", err)
	}
	if got != types.WikiPageAclDenyPrivate {
		t.Fatalf("got %q, want deny_private", got)
	}
}

// TestResolve_Private_Owner: page owner is always allowed regardless of mode.
func TestResolve_Private_Owner(t *testing.T) {
	repo := newStubRepo(&types.WikiPageAcl{
		Mode:     types.WikiPageAclModePrivate,
		Revision: 1,
	}, "alice")
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "alice", 1)
	got, err := svc.Resolve(ctx, "kb1", "slug1", "alice")
	if err != nil {
		t.Fatalf("Resolve(private,owner): %v", err)
	}
	if got != types.WikiPageAclAllow {
		t.Fatalf("got %q, want allow", got)
	}
}

// TestResolve_AllowList_UserInList: direct user id in allow_user_ids → allow.
func TestResolve_AllowList_UserInList(t *testing.T) {
	repo := newStubRepo(&types.WikiPageAcl{
		Mode:         types.WikiPageAclModeAllowList,
		AllowUserIDs: []string{"bob", "carol"},
		Revision:     2,
	}, "owner-1")
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "bob", 1)
	got, err := svc.Resolve(ctx, "kb1", "slug1", "bob")
	if err != nil {
		t.Fatalf("Resolve(allow_list,in): %v", err)
	}
	if got != types.WikiPageAclAllow {
		t.Fatalf("got %q, want allow", got)
	}
}

// TestResolve_AllowList_UserNotInList: user not in either list → deny.
func TestResolve_AllowList_UserNotInList(t *testing.T) {
	repo := newStubRepo(&types.WikiPageAcl{
		Mode:         types.WikiPageAclModeAllowList,
		AllowUserIDs: []string{"bob"},
		Revision:     2,
	}, "owner-1")
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "alice", 1)
	got, err := svc.Resolve(ctx, "kb1", "slug1", "alice")
	if err != nil {
		t.Fatalf("Resolve(allow_list,out): %v", err)
	}
	if got != types.WikiPageAclDenyAllowList {
		t.Fatalf("got %q, want deny_allow_list", got)
	}
}

// TestResolve_AllowList_GroupMember: caller reaches the allow list via a
// group membership rather than a direct user id.
func TestResolve_AllowList_GroupMember(t *testing.T) {
	repo := newStubRepo(&types.WikiPageAcl{
		Mode:          types.WikiPageAclModeAllowList,
		AllowGroupIDs: []string{"team-x"},
		Revision:      3,
	}, "owner-1")
	repo.groups["team-x"] = []string{"alice", "bob"}
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "alice", 1)
	got, err := svc.Resolve(ctx, "kb1", "slug1", "alice")
	if err != nil {
		t.Fatalf("Resolve(allow_list,group): %v", err)
	}
	if got != types.WikiPageAclAllow {
		t.Fatalf("got %q, want allow", got)
	}
}

// TestPutAcl_RevisionConflict: stale BaseRevision → ErrWikiPageAclRevisionConflict.
func TestPutAcl_RevisionConflict(t *testing.T) {
	repo := newStubRepo(&types.WikiPageAcl{
		Mode:     types.WikiPageAclModeAllowList,
		Revision: 5,
	}, "owner-1")
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "owner-1", 1)
	_, err := svc.PutAcl(ctx, "kb1", "slug1", types.WikiPageAclSaveRequest{
		Mode:         types.WikiPageAclModePrivate,
		BaseRevision: 3, // stale
	}, "owner-1", "owner")
	if !errors.Is(err, types.ErrWikiPageAclRevisionConflict) {
		t.Fatalf("PutAcl(stale) = %v, want ErrWikiPageAclRevisionConflict", err)
	}
}

// TestPutAcl_RevisionMatch: matching BaseRevision → revision+1 and new ACL returned.
func TestPutAcl_RevisionMatch(t *testing.T) {
	repo := newStubRepo(&types.WikiPageAcl{
		Mode:     types.WikiPageAclModeInherit,
		Revision: 7,
	}, "owner-1")
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "owner-1", 1)
	updated, err := svc.PutAcl(ctx, "kb1", "slug1", types.WikiPageAclSaveRequest{
		Mode:         types.WikiPageAclModeAllowList,
		AllowUserIDs: []string{"alice"},
		BaseRevision: 7,
	}, "owner-1", "owner")
	if err != nil {
		t.Fatalf("PutAcl(match): %v", err)
	}
	if updated.Revision != 8 {
		t.Fatalf("updated.Revision = %d, want 8", updated.Revision)
	}
	if updated.Mode != types.WikiPageAclModeAllowList {
		t.Fatalf("updated.Mode = %q, want allow_list", updated.Mode)
	}
}

// TestCache_InvalidationOnPut: after a successful PUT the next Resolve picks
// up the new ACL even within the 60s TTL window.
func TestCache_InvalidationOnPut(t *testing.T) {
	repo := newStubRepo(&types.WikiPageAcl{
		Mode:     types.WikiPageAclModeAllowList,
		Revision: 1,
		AllowUserIDs: []string{"bob"},
	}, "owner-1")
	svc := NewWikiAclService(repo, &stubUserService{})
	ctx := withUserAndTenant(context.Background(), "alice", 1)
	// First call: alice is NOT in allow_user_ids → deny_allow_list, cached.
	got, _ := svc.Resolve(ctx, "kb1", "slug1", "alice")
	if got != types.WikiPageAclDenyAllowList {
		t.Fatalf("pre-PUT got %q, want deny_allow_list", got)
	}
	// Put: switch mode to private + add alice to allow_user_ids. Revision
	// bumps from 1 → 2.
	_, err := svc.PutAcl(ctx, "kb1", "slug1", types.WikiPageAclSaveRequest{
		Mode:         types.WikiPageAclModePrivate,
		AllowUserIDs: []string{"alice"},
		BaseRevision: 1,
	}, "owner-1", "owner")
	if err != nil {
		t.Fatalf("PutAcl: %v", err)
	}
	// Second call: must observe the new ACL (not the cached deny).
	got, _ = svc.Resolve(ctx, "kb1", "slug1", "alice")
	if got != types.WikiPageAclAllow {
		t.Fatalf("post-PUT got %q, want allow (cache stale?)", got)
	}
}

// TestSearchAclCandidates: SearchAclCandidates delegates to userSvc.SearchUsers.
func TestSearchAclCandidates(t *testing.T) {
	userSvc := &stubUserService{users: []*types.User{
		{ID: "u-1", Username: "alice"},
		{ID: "u-2", Username: "alex"},
	}}
	repo := newStubRepo(nil, "owner-1")
	svc := NewWikiAclService(repo, userSvc)
	got, err := svc.SearchAclCandidates(context.Background(), 1, "al", 5)
	if err != nil {
		t.Fatalf("SearchAclCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d users, want 2", len(got))
	}
}

// TestGetAcl_NullRow: GetAcl on a legacy NULL row returns inherit/rev=0
// (not nil, not error).
func TestGetAcl_NullRow(t *testing.T) {
	repo := newStubRepo(nil, "owner-1")
	svc := NewWikiAclService(repo, &stubUserService{})
	got, err := svc.GetAcl(context.Background(), "kb1", "slug1")
	if err != nil {
		t.Fatalf("GetAcl(NULL): %v", err)
	}
	if got == nil || got.Mode != types.WikiPageAclModeInherit || got.Revision != 0 {
		t.Fatalf("GetAcl(NULL) = %+v, want inherit/rev=0", got)
	}
}