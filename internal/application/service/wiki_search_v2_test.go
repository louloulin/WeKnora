package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// fakeSearchV2Repo is a programmable in-memory stand-in for
// WikiSearchV2Repo so the service's input-validation + ACL post-filter
// path can be exercised without spinning up PostgreSQL.
type fakeSearchV2Repo struct {
	calls      int
	gotReq     types.WikiSearchV2Request
	gotVisible []string
	gotZh      string
	resp       types.WikiSearchV2Result
	err        error
}

func (f *fakeSearchV2Repo) Search(
	_ context.Context,
	_ uint64,
	_ []string,
	req types.WikiSearchV2Request,
	visibleKBIDs []string,
	zhQuery string,
) (types.WikiSearchV2Result, error) {
	f.calls++
	f.gotReq = req
	f.gotVisible = append([]string(nil), visibleKBIDs...)
	f.gotZh = zhQuery
	return f.resp, f.err
}

// fakeAclService is a minimal WikiAclService stub for the search v2
// service. Only Resolve is exercised; the rest panic to surface any
// accidental dependency.
type fakeAclService struct {
	resolutions map[string]string // key: kb|slug|user → "allow" / "deny_*"
}

func newFakeAclService() *fakeAclService {
	return &fakeAclService{resolutions: map[string]string{}}
}

func (a *fakeAclService) Resolve(_ context.Context, kbID, slug, userID string) (string, error) {
	if v, ok := a.resolutions[kbID+"|"+slug+"|"+userID]; ok {
		return v, nil
	}
	return types.WikiPageAclAllow, nil
}

// ResolveBulk mirrors Resolve for the bulk path; tests with the existing
// resolutions map get identical results whether the service calls one-at-
// a-time or in bulk.
func (a *fakeAclService) ResolveBulk(_ context.Context, items []AclResolveItem, userID string) (map[string]string, error) {
	out := make(map[string]string, len(items))
	for _, it := range items {
		key := it.KBID + ":" + it.Slug
		if v, ok := a.resolutions[it.KBID+"|"+it.Slug+"|"+userID]; ok {
			out[key] = v
			continue
		}
		out[key] = types.WikiPageAclAllow
	}
	return out, nil
}

func (a *fakeAclService) GetAcl(_ context.Context, _, _ string) (*types.WikiPageAcl, error) {
	panic("unused")
}
func (a *fakeAclService) PutAcl(_ context.Context, _, _ string, _ types.WikiPageAclSaveRequest, _, _ string) (*types.WikiPageAcl, error) {
	panic("unused")
}
func (a *fakeAclService) SearchAclCandidates(_ context.Context, _ uint64, _ string, _ int) ([]*types.User, error) {
	panic("unused")
}

func TestWikiSearchV2_EmptyQueryShortCircuits(t *testing.T) {
	repo := &fakeSearchV2Repo{}
	svc := NewWikiSearchV2Service(WikiSearchV2ServiceParams{
		Repo: repo,
		KB:   nil,
		ACL:  newFakeAclService(),
	})

	res, err := svc.Search(context.Background(), 1, "u1",
		types.WikiSearchV2Request{Query: ""},
		[]string{"kbA", "kbB"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("expected empty hits, got %d", len(res.Hits))
	}
	if res.Total != 0 {
		t.Fatalf("expected total=0, got %d", res.Total)
	}
	if repo.calls != 0 {
		t.Fatalf("repo.Search should not be called for empty query, called %d times", repo.calls)
	}
}

func TestWikiSearchV2_ACLPostFilter_DropsPrivateAndAllowListDenied(t *testing.T) {
	acl := newFakeAclService()
	acl.resolutions["kbA|secret|u1"] = types.WikiPageAclDenyPrivate
	acl.resolutions["kbA|need-allow|u1"] = types.WikiPageAclDenyAllowList
	acl.resolutions["kbA|public|u1"] = types.WikiPageAclAllow

	repo := &fakeSearchV2Repo{
		resp: types.WikiSearchV2Result{
			Hits: []types.WikiSearchV2Hit{
				{Slug: "secret", Title: "secret", KBID: "kbA", Snippet: "snippet"},
				{Slug: "need-allow", Title: "need-allow", KBID: "kbA", Snippet: "snippet"},
				{Slug: "public", Title: "public", KBID: "kbA", Snippet: "snippet"},
			},
		},
	}

	svc := NewWikiSearchV2Service(WikiSearchV2ServiceParams{
		Repo: repo,
		KB:   nil,
		ACL:  acl,
	})

	res, err := svc.Search(context.Background(), 1, "u1",
		types.WikiSearchV2Request{Query: "finance"},
		[]string{"kbA"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("expected 1 hit after ACL filter, got %d", len(res.Hits))
	}
	if res.Hits[0].Slug != "public" {
		t.Fatalf("expected only 'public' to survive, got %q", res.Hits[0].Slug)
	}
	if res.Total != 1 {
		t.Fatalf("expected total=1, got %d", res.Total)
	}
}

func TestWikiSearchV2_NormalizesKBAndPageTypes(t *testing.T) {
	repo := &fakeSearchV2Repo{
		resp: types.WikiSearchV2Result{Hits: []types.WikiSearchV2Hit{}},
	}
	svc := NewWikiSearchV2Service(WikiSearchV2ServiceParams{
		Repo: repo,
		KB:   nil,
		ACL:  newFakeAclService(),
	})

	_, err := svc.Search(context.Background(), 1, "u1",
		types.WikiSearchV2Request{
			Query:     "finance",
			KBIDs:     []string{"kbA", "kbA", "kbB", "  "},
			PageTypes: []string{"concept", "concept", "summary", ""},
			Limit:     0, // default → 20
			Offset:    -5, // clamp → 0
		},
		nil, // visibleKBIDs nil → unrestricted
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.calls != 1 {
		t.Fatalf("repo.Search should be called once, was %d", repo.calls)
	}
	if got := repo.gotReq.KBIDs; len(got) != 2 || got[0] != "kbA" || got[1] != "kbB" {
		t.Fatalf("deduped KBIDs = %v", got)
	}
	if got := repo.gotReq.PageTypes; len(got) != 2 || got[0] != "concept" || got[1] != "summary" {
		t.Fatalf("deduped PageTypes = %v", got)
	}
	if repo.gotReq.Limit != 20 {
		t.Fatalf("default limit should be 20, got %d", repo.gotReq.Limit)
	}
	if repo.gotReq.Offset != 0 {
		t.Fatalf("negative offset should clamp to 0, got %d", repo.gotReq.Offset)
	}
}

func TestWikiSearchV2_LimitClampedAt100(t *testing.T) {
	repo := &fakeSearchV2Repo{resp: types.WikiSearchV2Result{Hits: []types.WikiSearchV2Hit{}}}
	svc := NewWikiSearchV2Service(WikiSearchV2ServiceParams{
		Repo: repo,
		KB:   nil,
		ACL:  newFakeAclService(),
	})

	_, err := svc.Search(context.Background(), 1, "u1",
		types.WikiSearchV2Request{Query: "x", Limit: 999},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotReq.Limit != 100 {
		t.Fatalf("limit should clamp to 100, got %d", repo.gotReq.Limit)
	}
}

func TestWikiSearchV2_EmptyOverlapShortCircuits(t *testing.T) {
	// visibleKBIDs is restricted; requested kb_ids[] do not intersect.
	// Service must return empty hits without calling the repo.
	repo := &fakeSearchV2Repo{}
	svc := NewWikiSearchV2Service(WikiSearchV2ServiceParams{
		Repo: repo,
		KB:   nil,
		ACL:  newFakeAclService(),
	})

	res, err := svc.Search(context.Background(), 1, "u1",
		types.WikiSearchV2Request{
			Query: "finance",
			KBIDs: []string{"kbX", "kbY"},
		},
		[]string{"kbA"}, // only kbA is visible
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("expected empty hits, got %d", len(res.Hits))
	}
	if repo.calls != 0 {
		t.Fatalf("repo.Search must not run on empty overlap, called %d times", repo.calls)
	}
}

func TestWikiSearchV2_TenantMissingIsError(t *testing.T) {
	repo := &fakeSearchV2Repo{}
	svc := NewWikiSearchV2Service(WikiSearchV2ServiceParams{
		Repo: repo,
		KB:   nil,
		ACL:  newFakeAclService(),
	})

	_, err := svc.Search(context.Background(), 0, "u1",
		types.WikiSearchV2Request{Query: "x"},
		nil,
	)
	if err == nil {
		t.Fatalf("expected error when tenant id is 0")
	}
	if repo.calls != 0 {
		t.Fatalf("repo.Search must not run with bad tenant, called %d times", repo.calls)
	}
}