package types

import (
	"strings"
	"testing"
)

func TestMindMapValidate(t *testing.T) {
	cases := []struct {
		name string
		in   MindMap
		ok   bool
	}{
		{"valid", MindMap{TenantID: 1, Title: "x", Layout: MindMapLayoutTree, RootNodeID: "r", OwnerUserID: 42}, true},
		{"missing tenant", MindMap{Title: "x", Layout: MindMapLayoutTree, RootNodeID: "r", OwnerUserID: 42}, false},
		{"missing title", MindMap{TenantID: 1, Layout: MindMapLayoutTree, RootNodeID: "r", OwnerUserID: 42}, false},
		{"bad layout", MindMap{TenantID: 1, Title: "x", Layout: "bogus", RootNodeID: "r", OwnerUserID: 42}, false},
		{"missing root", MindMap{TenantID: 1, Title: "x", Layout: MindMapLayoutTree, OwnerUserID: 42}, false},
		{"missing owner", MindMap{TenantID: 1, Title: "x", Layout: MindMapLayoutTree, RootNodeID: "r"}, false},
	}
	for _, tc := range cases {
		err := tc.in.Validate()
		got := err == nil
		if got != tc.ok {
			t.Errorf("%s: got ok=%v want ok=%v (err=%v)", tc.name, got, tc.ok, err)
		}
	}
}

func TestMindMapNodeValidate(t *testing.T) {
	cases := []struct {
		name string
		in   MindMapNode
		ok   bool
	}{
		{"valid text", MindMapNode{TenantID: 1, MapID: "m", Label: "x", NodeType: MindMapNodeText}, true},
		{"missing tenant", MindMapNode{MapID: "m", Label: "x", NodeType: MindMapNodeText}, false},
		{"missing map", MindMapNode{TenantID: 1, Label: "x", NodeType: MindMapNodeText}, false},
		{"missing label", MindMapNode{TenantID: 1, MapID: "m", NodeType: MindMapNodeText}, false},
		{"bad node type", MindMapNode{TenantID: 1, MapID: "m", Label: "x", NodeType: "bogus"}, false},
	}
	for _, tc := range cases {
		err := tc.in.Validate()
		got := err == nil
		if got != tc.ok {
			t.Errorf("%s: got ok=%v want ok=%v (err=%v)", tc.name, got, tc.ok, err)
		}
	}
}

func TestValidMindMapLayoutsClosed(t *testing.T) {
	for _, l := range []MindMapLayout{MindMapLayoutTree, MindMapLayoutFree, MindMapLayoutFishbone, MindMapLayoutTimeline, MindMapLayoutRadial} {
		if !ValidMindMapLayouts[l] {
			t.Errorf("missing layout: %s", l)
		}
	}
}

func TestValidMindMapNodeTypesClosed(t *testing.T) {
	for _, n := range []MindMapNodeType{MindMapNodeText, MindMapNodeImage, MindMapNodeLink, MindMapNodeDocRef, MindMapNodeTask, MindMapNodeFormula} {
		if !ValidMindMapNodeTypes[n] {
			t.Errorf("missing node type: %s", n)
		}
	}
}

func TestValidExportFormats(t *testing.T) {
	for _, f := range []ExportFormat{ExportFormatPNG, ExportFormatSVG, ExportFormatMD, ExportFormatOPML, ExportFormatXMIND} {
		if !ValidExportFormats[f] {
			t.Errorf("missing format: %s", f)
		}
	}
}

func TestErrMindMapInvalidImplementsError(t *testing.T) {
	var err error = ErrMindMapInvalid("test")
	if !strings.Contains(err.Error(), "test") {
		t.Errorf("error string mismatch: %v", err)
	}
	if !IsMindMapInvalid(err) {
		t.Errorf("expected IsMindMapInvalid to match")
	}
	if IsMindMapInvalid(nil) {
		t.Errorf("nil should not match")
	}
}
