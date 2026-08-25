package handler

import (
	"bytes"
	"errors"
	"testing"
)

func TestParseYFrame_KnownTypes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    []byte
		wantKind yFrameKind
		wantLen  int
		wantErr  bool
	}{
		{"sync", []byte{0x00, 'a'}, yFrameSync, 1, false},
		{"awareness", []byte{0x01, 'b'}, yFrameAwareness, 1, false},
		{"auth_rejected", []byte{0x02, 'c'}, yFrameRejectAuth, 1, false},
		{"query_awareness_rejected", []byte{0x03, 'd'}, yFrameRejectQueryAwareness, 1, false},
		{"unsupported_4_rejected", []byte{0x04, 'e'}, yFrameRejectUnsupported, 1, false},
		{"unsupported_high_rejected", []byte{0x7f, 'f'}, yFrameRejectUnsupported, 1, false},
		{"empty_malformed", []byte{}, yFrameRejectMalformed, 0, true},
		{"unterminated_varint", []byte{0x80}, yFrameRejectMalformed, 0, true},
		{"oversized_varint", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}, yFrameRejectMalformed, 0, true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			k, l, err := parseYFrame(c.input)
			if k != c.wantKind {
				t.Errorf("kind = %d, want %d", k, c.wantKind)
			}
			if l != c.wantLen {
				t.Errorf("varintLen = %d, want %d", l, c.wantLen)
			}
			if (err != nil) != c.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}

func TestParseYFrame_BytesRoundTrip(t *testing.T) {
	t.Parallel()
	// Verify the helper bytesIndex + JSON-walk don't corrupt payload
	// bytes. A round-trip is the strongest test for off-by-one bugs.
	frame := []byte{0x01, '{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 'A', 'l', 'i', 'c', 'e', '"', '}'}
	k, l, err := parseYFrame(frame)
	if err != nil || k != yFrameAwareness || l != 1 {
		t.Fatalf("parseYFrame failed: k=%d l=%d err=%v", k, l, err)
	}
	// The payload after stripping the varint must be the original JSON.
	if !bytes.Equal(frame[l:], []byte(`{"name":"Alice"}`)) {
		t.Fatalf("payload after varint = %q, want %q", frame[l:], `{"name":"Alice"}`)
	}
}

func TestIsAllowedForward(t *testing.T) {
	t.Parallel()
	if !isAllowedForward(yFrameSync) {
		t.Error("sync should be allowed")
	}
	if !isAllowedForward(yFrameAwareness) {
		t.Error("awareness should be allowed")
	}
	if isAllowedForward(yFrameRejectAuth) {
		t.Error("auth must not forward")
	}
	if isAllowedForward(yFrameUnknown) {
		t.Error("unknown must not forward")
	}
}

func TestErrUnknownFrameKind(t *testing.T) {
	t.Parallel()
	_, _, err := parseYFrame([]byte{0x04})
	if err == nil {
		t.Fatal("expected error for messageType=4")
	}
	if !errors.Is(err, ErrUnknownFrameKind) {
		t.Fatalf("err should wrap ErrUnknownFrameKind; got %v", err)
	}
}

func TestParseAwarenessSummary_HappyPath(t *testing.T) {
	t.Parallel()
	// Realistic awareness payload shape (y-protocol CRDT-wrapped JSON).
	frame := []byte{0x01, '{', '"', 'u', '"', ':', '{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 'B', 'o', 'b', '"', ',', '"', 'c', 'o', 'l', 'o', 'r', '"', ':', '"', '#', 'f', 'f', '0', '0', '0', '0', '"', '}', '}'}
	out, err := parseAwarenessSummary(frame)
	if err != nil {
		t.Fatalf("parseAwarenessSummary failed: %v", err)
	}
	if out.DisplayName != "Bob" {
		t.Errorf("display_name = %q, want %q", out.DisplayName, "Bob")
	}
	if out.Color != "#ff0000" {
		t.Errorf("color = %q, want %q", out.Color, "#ff0000")
	}
}

func TestParseAwarenessSummary_MissingFields(t *testing.T) {
	t.Parallel()
	// A garbage awareness frame should still return an entry; the
	// caller (NoteAwareness) treats empty display/color as acceptable.
	frame := []byte{0x01, 'g', 'a', 'r', 'b', 'a', 'g', 'e'}
	out, err := parseAwarenessSummary(frame)
	if err != nil {
		t.Fatalf("missing fields should not error, got %v", err)
	}
	if out.DisplayName != "" || out.Color != "" {
		t.Errorf("missing fields should leave name/color empty; got %+v", out)
	}
}
