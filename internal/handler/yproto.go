package handler

import (
	"errors"
	"fmt"
)

// y-protocol message types (lib0/protocol/message-type).
//
// Reference: https://github.com/yjs/y-protocols/blob/master/protocol.js
//
//	y-protocol frames are: [varint messageType, payload...]
//	messageType is a single varint (typically 0-3 for our usage). We
//	parse it defensively because a malicious client can send a
//	multi-byte varint to confuse a naive "first byte" check.
const (
	yMessageSync         byte = 0
	yMessageAwareness    byte = 1
	yMessageAuth         byte = 2
	yMessageQueryAwareness byte = 3
)

// yFrameKind classifies a frame the hub just read off the wire.
//
//	We do NOT parse the payload — the hub forwards frames opaquely
//	for sync + awareness and drops everything else. The only thing we
//	need is the messageType to decide whether to forward, persist, or
//	reject.
type yFrameKind int

const (
	yFrameUnknown yFrameKind = iota
	yFrameSync
	yFrameAwareness
	// Rejection reasons use distinct values so we can log structured
	// counters in observability without parsing freeform strings.
	yFrameRejectAuth
	yFrameRejectQueryAwareness
	yFrameRejectMalformed
	yFrameRejectUnsupported
)

// ErrUnknownFrameKind is returned by parseYFrame when the varint is
// outside the range we know about (any byte ≥ yMaxKnownMessageType
// plus a small extension window for future y-protocol additions).
// The caller treats this as a hard reject — the same path as
// yFrameRejectUnsupported.
var ErrUnknownFrameKind = errors.New("y-protocol: unknown messageType")

// yMaxKnownMessageType caps the varint range we accept. y-protocol
// currently uses 0-3; we allow up to 7 so future versions of the spec
// don't immediately disconnect on an unknown but valid extension. We
// still treat anything > 3 as yFrameRejectUnsupported, not allowed
// forward, so the door stays closed today while the protocol can
// evolve tomorrow.
const yMaxKnownMessageType byte = 7

// parseYFrame parses the leading varint and returns the classified
// kind plus the byte length of the varint itself so the caller can
// strip it before forwarding the payload.
//
//	y-frames are varint-prefixed; the encoder uses one byte for values
//	0-127, which covers every type currently defined. We support up to
//	8 bytes of varint to be safe (yjs varints are arbitrary width).
//
// The function is intentionally allocation-free — it operates on the
// input byte slice and returns a (kind, varintLen) pair.
func parseYFrame(data []byte) (yFrameKind, int, error) {
	if len(data) == 0 {
		return yFrameRejectMalformed, 0, fmt.Errorf("y-protocol: empty frame")
	}
	// y-protocol uses lib0's writeVarUint which is LE 7-bit continuation.
	// For known messageTypes (0-3) this is a single byte; we still
	// walk the multi-byte form to avoid a denial-of-service where a
	// peer sends a giant varint to exhaust the parser.
	var (
		value uint64
		shift uint
	)
	varLen := 0
	for i := 0; i < len(data) && i < 8; i++ {
		b := data[i]
		varLen++
		value |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			goto decoded
		}
		shift += 7
	}
	return yFrameRejectMalformed, 0, fmt.Errorf("y-protocol: varint too long or unterminated")
decoded:
	if value > uint64(yMaxKnownMessageType) {
		return yFrameRejectUnsupported, varLen, fmt.Errorf("%w: %d", ErrUnknownFrameKind, value)
	}
	switch byte(value) {
	case yMessageSync:
		return yFrameSync, varLen, nil
	case yMessageAwareness:
		return yFrameAwareness, varLen, nil
	case yMessageAuth:
		return yFrameRejectAuth, varLen, nil
	case yMessageQueryAwareness:
		return yFrameRejectQueryAwareness, varLen, nil
	default:
		return yFrameRejectUnsupported, varLen, ErrUnknownFrameKind
	}
}

// isAllowedForward returns true for kinds the hub will rebroadcast to
// the rest of the room. Sync and Awareness are forwarded; the reject
// kinds and Unknown are dropped (and counted so the readLoop can
// decide whether to disconnect a noisy peer).
func isAllowedForward(k yFrameKind) bool {
	return k == yFrameSync || k == yFrameAwareness
}

// isAwareness returns true for the awareness kind — the persistence
// path in wiki_collab.go uses this to decide whether to upsert the
// payload into wiki_collab_awareness.
func isAwareness(k yFrameKind) bool {
	return k == yFrameAwareness
}
