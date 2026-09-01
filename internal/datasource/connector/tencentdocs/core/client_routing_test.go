package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchAnyContent_RoutesByType verifies the central content dispatcher
// picks the right endpoint per document type. Without this, the picker could
// silently route sheet / slide / form through the doc endpoint and the user
// would see garbled content.
func TestFetchAnyContent_RoutesByType(t *testing.T) {
	cases := []struct {
		name, docType, wantPath, wantMime string
	}{
		{"doc", "doc", "/doc/v3/", "text/markdown"},
		{"sheet", "sheet", "/sheet/v3/", "text/csv"},
		{"slide", "slide", "/slide/v3/", "text/markdown"},
		{"form", "form", "/form/v3/", "application/json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hit string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/oauth/v2/token") {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":7200}`))
					return
				}
				hit = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"errcode":0,"data":"hello-%s","content_type":%q}`,
					tc.name, tc.wantMime)))
			}))
			defer srv.Close()

			cli := &Client{
				baseURL:    srv.URL,
				cfg:        &Config{AccessToken: "tok"},
				httpClient: srv.Client(),
			}
			d := Document{ID: "D-1", Type: tc.docType}
			body, mime, err := cli.FetchAnyContent(context.Background(), d)
			if err != nil {
				t.Fatalf("FetchAnyContent: %v", err)
			}
			if !strings.HasPrefix(hit, tc.wantPath) {
				t.Errorf("routed to %q, want prefix %q", hit, tc.wantPath)
			}
			if !strings.Contains(body, "hello-"+tc.name) {
				t.Errorf("body = %q, want contains hello-%s", body, tc.name)
			}
			if mime != tc.wantMime {
				t.Errorf("mime = %q, want %q", mime, tc.wantMime)
			}
		})
	}
}

// TestAllDocTypes verifies the helper that drives ListAllDriveFiles.
// Order matters because the engine reads docs in the order the picker shows
// them; if this changed shape the cursor compare logic in production could
// silently drift.
func TestAllDocTypes(t *testing.T) {
	want := []string{"doc", "sheet", "slide", "form"}
	got := AllDocTypes()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// TestIsSingleDocumentID exercises the exported alias that doc/ uses to
// decide between "fetch this one doc" and "list the whole library".
func TestIsSingleDocumentID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Dabc123def456ghi789", true},
		{"Sxyz000aaa111bbb222", true},
		{"short", false},
		{"contains spaces here123", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := IsSingleDocumentID(tc.in); got != tc.want {
				t.Errorf("IsSingleDocumentID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
