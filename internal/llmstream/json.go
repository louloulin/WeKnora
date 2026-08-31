package llmstream

import "encoding/json"

// jsonEncode is a tiny wrapper around encoding/json so callers
// don't have to handle the error path. It panics on encoding
// failures because every value the SSE formatter passes through
// here is built from our own typed structs and is therefore always
// serialisable.
func jsonEncode(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// Should never happen for the typed values we feed in.
		// Falling back to a literal keeps the wire frame valid.
		return `{"_encoding_error":true}`
	}
	return string(b)
}
