package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Build #26 — wiki_backlinks_cache_backref index harness.
//
// The repo's transactional Upsert / Delete / DeleteByKB / FindReferencingSlugs
// are exercised here against a real in-memory SQLite database. SQLite is
// the cheapest dialect to spin up in tests; the same transactional
// semantics apply across PG / MySQL / SQLite for the operations under
// test (no dialect-specific JSON parsing involved — the index is a flat
// relational table).
//
// The eight tests cover A1–A8 of the Build #26 spec:
//   1. Upsert populates the backref index from the cache payload
//   2. Upsert replaces stale backref rows when the referenced-slug set
//      shrinks (drop-all-then-insert semantics)
//   3. Delete removes the matching backref rows
//   4. DeleteByKB removes every backref row for the KB
//   5. FindReferencingSlugs walks the inverted index
//   6. FindReferencingSlugs is KB-scoped (no cross-KB leakage)
//   7. FindReferencingSlugs returns empty when no row references the slug
//   8. Upsert's transaction rolls back atomically when the backref
//      insert fails (cache row is reverted)

// newBackrefRepoForTest wires a real WikiBacklinksCacheRepository against
// an in-memory SQLite database with the Build #21 + #26 schemas
// auto-migrated. The file-name trick (?mode=memory&cache=shared) lets
// multiple connections in the same process share the DB — SQLite's
// `:memory:` alone is per-connection, which breaks GORM's transaction
// manager when it spawns a second handle.
func newBackrefRepoForTest(t *testing.T) (interfaces.WikiBacklinksCacheRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.WikiBacklinksCacheRow{},
		&types.WikiBacklinksCacheBackrefRow{},
	))
	return repository.NewWikiBacklinksCacheRepository(db), db
}

// upsertRow is a small builder that constructs a cache row with a
// sensible payload so each test can focus on one assertion. The
// referenced-slug lists are passed as raw JSON arrays.
func upsertRow(kbID, slug string, direct, indirect, related []string) *types.WikiBacklinksCacheRow {
	mk := func(slugs []string) string {
		if len(slugs) == 0 {
			return "[]"
		}
		// Build a JSON array literal — the payload columns are TEXT
		// (Build #21 schema) so we never need real backref page
		// metadata here, just slug strings.
		out := "["
		for i, s := range slugs {
			if i > 0 {
				out += ","
			}
			out += `"` + s + `"`
		}
		return out + "]"
	}
	return &types.WikiBacklinksCacheRow{
		KbID:         kbID,
		Slug:         slug,
		DirectJSON:   mk(direct),
		IndirectJSON: mk(indirect),
		RelatedJSON:  mk(related),
	}
}

// backrefCount returns the number of backref rows matching the filter.
// Empty filter counts all rows in the table.
func backrefCount(t *testing.T, db *gorm.DB, kbID string) int64 {
	t.Helper()
	q := db.Model(&types.WikiBacklinksCacheBackrefRow{})
	if kbID != "" {
		q = q.Where("kb_id = ?", kbID)
	}
	var n int64
	require.NoError(t, q.Count(&n).Error)
	return n
}

// Test 1 — Upsert populates the backref index from the cache payload.
func TestWikiBacklinksCache_Upsert_PopulatesBackref(t *testing.T) {
	repo, db := newBackrefRepoForTest(t)
	ctx := context.Background()
	row := upsertRow("kb-a", "concept/x", []string{"concept/a", "concept/b"}, []string{"concept/c"}, []string{"concept/d"})
	require.NoError(t, repo.Upsert(ctx, row))

	// 4 unique slugs referenced → 4 backref rows, all owned by concept/x
	require.Equal(t, int64(4), backrefCount(t, db, "kb-a"))

	var got []types.WikiBacklinksCacheBackrefRow
	require.NoError(t, db.Where("kb_id = ? AND owning_slug = ?", "kb-a", "concept/x").
		Find(&got).Error)
	require.Len(t, got, 4)
	refs := map[string]bool{}
	for _, b := range got {
		refs[b.ReferencedSlug] = true
	}
	for _, want := range []string{"concept/a", "concept/b", "concept/c", "concept/d"} {
		require.True(t, refs[want], "missing backref for %s", want)
	}
}

// Test 2 — Upsert replaces stale backref rows when the referenced-slug
// set shrinks. Old refs must not linger after a re-Upsert.
func TestWikiBacklinksCache_Upsert_ReplacesBackref(t *testing.T) {
	repo, db := newBackrefRepoForTest(t)
	ctx := context.Background()

	// First Upsert: refs = {a, b, c}
	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/x", []string{"a", "b", "c"}, nil, nil)))
	require.Equal(t, int64(3), backrefCount(t, db, "kb-a"))

	// Second Upsert: refs = {a, d} — b, c should be gone; a, d should be in
	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/x", []string{"a", "d"}, nil, nil)))
	require.Equal(t, int64(2), backrefCount(t, db, "kb-a"))

	var got []types.WikiBacklinksCacheBackrefRow
	require.NoError(t, db.Where("kb_id = ? AND owning_slug = ?", "kb-a", "concept/x").
		Find(&got).Error)
	refs := map[string]bool{}
	for _, b := range got {
		refs[b.ReferencedSlug] = true
	}
	require.True(t, refs["a"])
	require.True(t, refs["d"])
	require.False(t, refs["b"], "stale backref b should have been dropped")
	require.False(t, refs["c"], "stale backref c should have been dropped")
}

// Test 3 — Delete removes the matching backref rows. The cache row +
// its backref rows disappear together.
func TestWikiBacklinksCache_Delete_RemovesBackref(t *testing.T) {
	repo, db := newBackrefRepoForTest(t)
	ctx := context.Background()

	// Seed two cache rows in the same KB so we can verify Delete is
	// scoped to the slugs it was asked to wipe.
	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/x", []string{"a", "b"}, nil, nil)))
	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/y", []string{"a"}, nil, nil)))
	require.Equal(t, int64(3), backrefCount(t, db, "kb-a"))

	// Delete only concept/x — its 2 backrefs go; concept/y's 1 stays.
	affected, err := repo.Delete(ctx, "kb-a", []string{"concept/x"})
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
	require.Equal(t, int64(1), backrefCount(t, db, "kb-a"))

	var row types.WikiBacklinksCacheRow
	require.Error(t, db.Where("kb_id = ? AND slug = ?", "kb-a", "concept/x").
		First(&row).Error, "cache row concept/x should be gone")
	require.NoError(t, db.Where("kb_id = ? AND slug = ?", "kb-a", "concept/y").
		First(&row).Error, "cache row concept/y should still exist")
}

// Test 4 — DeleteByKB removes every backref row for the KB across all
// owning slugs.
func TestWikiBacklinksCache_DeleteByKB_RemovesAllBackrefs(t *testing.T) {
	repo, db := newBackrefRepoForTest(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/x", []string{"a", "b"}, nil, nil)))
	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/y", []string{"c"}, nil, nil)))
	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-b", "concept/z", []string{"d"}, nil, nil)))
	require.Equal(t, int64(3), backrefCount(t, db, "kb-a"))
	require.Equal(t, int64(1), backrefCount(t, db, "kb-b"))

	affected, err := repo.DeleteByKB(ctx, "kb-a")
	require.NoError(t, err)
	require.Equal(t, int64(2), affected)

	require.Equal(t, int64(0), backrefCount(t, db, "kb-a"))
	require.Equal(t, int64(1), backrefCount(t, db, "kb-b"),
		"kb-b backref rows must survive a kb-a wipe")
}

// Test 5 — FindReferencingSlugs walks the inverted index. Two cache
// rows in the same KB both reference concept/x — the lookup returns
// both, deduplicated.
func TestWikiBacklinksCache_FindReferencingSlugs_IndexPath(t *testing.T) {
	repo, _ := newBackrefRepoForTest(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/y", []string{"concept/x"}, nil, nil)))
	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/z", []string{"concept/x", "concept/x", "concept/w"}, nil, nil)))

	refs, err := repo.FindReferencingSlugs(ctx, "kb-a", "concept/x")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"concept/y", "concept/z"}, refs)

	// concept/w is only referenced by z; lookup returns just z.
	refs, err = repo.FindReferencingSlugs(ctx, "kb-a", "concept/w")
	require.NoError(t, err)
	require.Equal(t, []string{"concept/z"}, refs)
}

// Test 6 — FindReferencingSlugs is KB-scoped. Two KBs each have a
// cache row that references the same slug; the lookup for KB-A must
// NOT surface KB-B's owning slug.
func TestWikiBacklinksCache_FindReferencingSlugs_KBIsolation(t *testing.T) {
	repo, _ := newBackrefRepoForTest(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/y", []string{"concept/x"}, nil, nil)))
	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-b", "concept/z", []string{"concept/x"}, nil, nil)))

	refs, err := repo.FindReferencingSlugs(ctx, "kb-a", "concept/x")
	require.NoError(t, err)
	require.Equal(t, []string{"concept/y"}, refs,
		"cross-KB leakage: kb-b/concept/z appeared in kb-a lookup")
}

// Test 7 — FindReferencingSlugs returns empty when no row references
// the slug. Distinct from "lookup errored" — the read path treats
// empty as cache miss for the reverse-lookup.
func TestWikiBacklinksCache_FindReferencingSlugs_EmptyHit(t *testing.T) {
	repo, _ := newBackrefRepoForTest(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx,
		upsertRow("kb-a", "concept/y", []string{"concept/a"}, nil, nil)))

	refs, err := repo.FindReferencingSlugs(ctx, "kb-a", "concept/never-referenced")
	require.NoError(t, err)
	require.Empty(t, refs)
}

// Test 8 — Upsert's transaction rolls back atomically when the backref
// insert fails. The cache row must NOT be visible after the failure.
func TestWikiBacklinksCache_Upsert_TransactionRollback(t *testing.T) {
	repo, db := newBackrefRepoForTest(t)
	ctx := context.Background()

	// Inject a fault on the backref insert via a GORM create callback.
	// Any Create on the backref model returns an error, but the cache
	// row insert (the first Create in the tx) still completes inside
	// the transaction. The tx should roll back, leaving the cache row
	// unwritten.
	injected := errors.New("injected backref insert failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").
		Register("backref_fault_inject", func(tx *gorm.DB) {
			if tx.Statement.Schema != nil &&
				tx.Statement.Schema.Name == "WikiBacklinksCacheBackrefRow" {
				tx.AddError(injected)
			}
		}))

	row := upsertRow("kb-a", "concept/x", []string{"a", "b"}, nil, nil)
	err := repo.Upsert(ctx, row)
	require.Error(t, err)
	require.ErrorIs(t, err, injected, "Upsert must propagate the injected backref fault")

	// Cache row must NOT be visible — the transaction rolled back.
	var count int64
	require.NoError(t, db.Model(&types.WikiBacklinksCacheRow{}).
		Where("kb_id = ? AND slug = ?", "kb-a", "concept/x").
		Count(&count).Error)
	require.Equal(t, int64(0), count, "cache row should be rolled back along with backref failure")

	// Backref rows must also be empty.
	require.Equal(t, int64(0), backrefCount(t, db, "kb-a"))
}