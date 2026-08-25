package types

// FeaturesFlags is the per-flag map returned by GET /api/v1/features.
// Each field is a flat boolean — the frontend reads flags.wiki_wysiwyg
// to decide whether to render the Tiptap editor (Build #2b) or fall
// back to the existing textarea (Build #2a behavior).
type FeaturesFlags struct {
	WikiWysiwyg bool `json:"wiki_wysiwyg"`
}

// FeaturesResponse is the `data` payload for GET /api/v1/features.
// Wrapped at the envelope layer by the handler (code/msg/data), so
// this type only describes the inner shape.
type FeaturesResponse struct {
	Flags FeaturesFlags `json:"flags"`
}