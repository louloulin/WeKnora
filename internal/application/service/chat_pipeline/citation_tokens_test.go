package chatpipeline

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// TestAttachCitationsPure verifies the pure single-call helper. The
// streaming path uses citationBuilder instead, but attachCitations is
// kept as a focused unit testable surface.
func TestAttachCitationsPure(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)

	in := `See <kb doc="Doc A" chunk_id="chunk-1" kb_id="kb-x" /> and <kb doc="Doc B" chunk_id="chunk-2" /> for details.`
	out, idx := attachCitations(cm, in)
	require.Equal(t,
		"See [[cite:1]] and [[cite:2]] for details.",
		out,
	)
	require.Len(t, idx, 2)
	require.Equal(t, "chunk-1", idx[0].ChunkID)
	require.Equal(t, "kb-x", idx[0].KnowledgeBaseID)
	require.Equal(t, "Doc A", idx[0].Title)
	require.Equal(t, "chunk-2", idx[1].ChunkID)
}

// TestAttachCitationsDisabledIsNoop verifies the citation-enabled=false
// branch: the text must pass through unchanged and no index is built.
func TestAttachCitationsDisabledIsNoop(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(false)

	in := `See <kb doc="Doc A" chunk_id="chunk-1" /> for details.`
	out, idx := attachCitations(cm, in)
	require.Equal(t, in, out, "disabled citations must not rewrite")
	require.Nil(t, idx)
}

// TestAttachCitationsDedupByChunkID verifies that two references to the
// same chunk_id share a single citation number.
func TestAttachCitationsDedupByChunkID(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)

	in := `A <kb doc="D" chunk_id="c1" /> B <kb doc="D" chunk_id="c1" /> C <kb doc="D" chunk_id="c2" />`
	out, idx := attachCitations(cm, in)
	require.Equal(t, "A [[cite:1]] B [[cite:1]] C [[cite:2]]", out)
	require.Len(t, idx, 2)
	require.Equal(t, "c1", idx[0].ChunkID)
	require.Equal(t, "c2", idx[1].ChunkID)
}

// TestAttachCitationsEmptyText verifies the trivial fast path.
func TestAttachCitationsEmptyText(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)
	out, idx := attachCitations(cm, "")
	require.Equal(t, "", out)
	require.Nil(t, idx)
}

// TestAttachCitationsDropsEmptyChunkID verifies that a `<kb>` tag with
// no chunk_id is dropped from the rewritten text and does NOT claim a
// position number — this matches the "garbage in, garbage out" tolerance
// for hallucinated tags.
func TestAttachCitationsDropsEmptyChunkID(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)

	in := `See <kb doc="D" chunk_id="" /> and <kb doc="D" chunk_id="c1" />.`
	out, idx := attachCitations(cm, in)
	require.Equal(t, "See  and [[cite:1]].", out)
	require.Len(t, idx, 1)
	require.Equal(t, "c1", idx[0].ChunkID)
}

// TestAttachCitationsAttributeOrderIndependent verifies that the regex
// extracts chunk_id / title / kb_id regardless of attribute order. The
// modelcontext expander emits `<kb doc="..." chunk_id="..." />` but the
// Forge may flip the order in future revisions, so the rewrite pass must
// not be order-sensitive.
func TestAttachCitationsAttributeOrderIndependent(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)

	cases := []string{
		`<kb doc="Doc A" chunk_id="c1" kb_id="kb-x" />`,
		`<kb chunk_id="c1" doc="Doc A" kb_id="kb-x" />`,
		`<kb kb_id="kb-x" chunk_id="c1" doc="Doc A" />`,
		`<kb chunk_id="c1" />`,
		`<kb title="T" chunk_id="c2" />`,
	}
	for _, in := range cases {
		out, idx := attachCitations(cm, in)
		require.Equal(t, "[[cite:1]]", out, "in=%s", in)
		if in == `<kb chunk_id="c1" />` {
			require.Equal(t, "c1", idx[0].ChunkID)
			require.Equal(t, "", idx[0].KnowledgeBaseID)
			require.Equal(t, "", idx[0].Title)
			continue
		}
		if in == `<kb title="T" chunk_id="c2" />` {
			require.Equal(t, "c2", idx[0].ChunkID)
			require.Equal(t, "T", idx[0].Title)
			require.Equal(t, "", idx[0].KnowledgeBaseID)
			continue
		}
		require.Equal(t, "c1", idx[0].ChunkID, "in=%s", in)
		require.Equal(t, "Doc A", idx[0].Title, "in=%s", in)
		require.Equal(t, "kb-x", idx[0].KnowledgeBaseID, "in=%s", in)
	}
}

// TestCitationBuilderStreamsDedupAcrossChunks is the streaming-aware
// behaviour: chunk 1 cites c1, chunk 2 cites c1 again and adds c2 — the
// second c1 reference must keep position 1, not claim position 2.
func TestCitationBuilderStreamsDedupAcrossChunks(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)
	b := newCitationBuilder(cm)

	chunk1 := b.Rewrite(`First <kb doc="Doc A" chunk_id="c1" />.`)
	chunk2 := b.Rewrite(`Again <kb doc="Doc A" chunk_id="c1" /> plus <kb doc="Doc B" chunk_id="c2" />.`)

	require.Equal(t, "First [[cite:1]].", chunk1)
	require.Equal(t, "Again [[cite:1]] plus [[cite:2]].", chunk2)

	idx := b.Index()
	require.Len(t, idx, 2)
	require.Equal(t, "c1", idx[0].ChunkID)
	require.Equal(t, "Doc A", idx[0].Title)
	require.Equal(t, "c2", idx[1].ChunkID)
	require.Equal(t, "Doc B", idx[1].Title)
}

// TestCitationBuilderDisabledIsIdentity verifies the streaming builder
// respects CitationsEnabled(): when off, Rewrite is a passthrough and
// Index returns nil. This is the gate the goroutine relies on.
func TestCitationBuilderDisabledIsIdentity(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(false)
	b := newCitationBuilder(cm)

	in := `See <kb doc="D" chunk_id="c1" /> for details.`
	require.Equal(t, in, b.Rewrite(in))
	require.Nil(t, b.Index(), "disabled builder must not produce an index")
}

// TestCitationBuilderEmptyText verifies the no-op fast path on empty
// input — important because streaming chunks may carry only whitespace
// or an empty Done=true payload.
func TestCitationBuilderEmptyText(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)
	b := newCitationBuilder(cm)
	require.Equal(t, "", b.Rewrite(""))
	require.Nil(t, b.Index())
}

// TestCitationBuilderIndexIsCopy verifies the Index snapshot is
// defensive: callers can mutate the returned slice without disturbing
// the builder's internal state.
func TestCitationBuilderIndexIsCopy(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)
	b := newCitationBuilder(cm)
	b.Rewrite(`<kb doc="D" chunk_id="c1" />`)

	idx := b.Index()
	require.Len(t, idx, 1)
	idx[0].ChunkID = "tampered"

	fresh := b.Index()
	require.Equal(t, "c1", fresh[0].ChunkID, "Index must return a defensive copy")
}

// TestCitationBuilderIndexEmptyWhenNoCitations verifies the builder
// does not emit an empty index when the answer had no `<kb>` tags.
func TestCitationBuilderIndexEmptyWhenNoCitations(t *testing.T) {
	cm := &types.ChatManage{}
	cm.CitationEnabled = boolPtr(true)
	b := newCitationBuilder(cm)
	b.Rewrite(`Just plain prose with no citations.`)
	require.Nil(t, b.Index(), "no chunks cited → no index emitted")
}

// boolPtr is a tiny helper for tests; pulled out so each test can pin
// the bool without re-importing the strconv-free idiom in every case.
func boolPtr(b bool) *bool { return &b }