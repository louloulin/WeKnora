// Wiki tag types (Build #17 / P1.1).
//
// Mirrors the backend shapes defined in `internal/types/wiki_tag.go`:
// every type here corresponds to one server-side struct so the API
// client and the Pinia store can stay free of `any`. The wire format
// is plain JSON; the request utilities unwrap the axios envelope
// before these types are observed.

// WikiTagColor is the palette of user-selectable tag colors. Hard-
// coded on both ends so the backend can reject anything outside this
// set at the request level and the frontend can render the chip
// without a CSS lookup table.
export type WikiTagColor =
  | 'blue'
  | 'green'
  | 'orange'
  | 'red'
  | 'purple'
  | 'teal'
  | 'gray'
  | 'gold'

// WikiTagPalette is the ordered list of valid colors. Kept as a
// constant rather than a derived union so the iteration order (used
// by the picker UI) is stable and matches the backend's acceptance
// order in `types.IsValidWikiTagColor`.
export const WikiTagPalette: WikiTagColor[] = [
  'blue',
  'green',
  'orange',
  'red',
  'purple',
  'teal',
  'gray',
  'gold',
]

// WikiTag is one row in the wiki_tags table.
export interface WikiTag {
  id: string
  tenant_id: number
  knowledge_base_id: string
  name: string
  color: WikiTagColor
  sort_order: number
  created_at: string
  updated_at: string
}

// WikiTagWithCount is the List response — same shape as WikiTag plus
// the current page_count from a LEFT JOIN + GROUP BY.
export interface WikiTagWithCount extends WikiTag {
  page_count: number
}

// WikiTagCreateRequest is the POST body. Color is optional — the
// backend defaults to 'blue' when the field is omitted.
export interface WikiTagCreateRequest {
  name: string
  color?: WikiTagColor
}

// WikiTagUpdateRequest is the PUT body. All fields optional via the
// Omit<T, K> + Partial<T> pattern: undefined means "leave unchanged".
// The backend ignores nil pointers.
export interface WikiTagUpdateRequest {
  name?: string
  color?: WikiTagColor
  sort_order?: number
}

// WikiTagSetPageRequest is the PUT /pages/:slug/tags body. The
// service replaces the existing associations atomically.
export interface WikiTagSetPageRequest {
  tag_ids: string[]
}

// WikiBatchTagBody is the POST /pages/batch-tag body. Op is 'add' or
// 'remove'; the backend maps any other value to HTTP 400.
export interface WikiBatchTagBody {
  slugs: string[]
  tag_id: string
  op: 'add' | 'remove'
}

// WikiBatchTagOpAdd / WikiBatchTagOpRemove mirror the backend constants
// so callers don't sprinkle string literals.
export const WikiBatchTagOpAdd = 'add' as const
export const WikiBatchTagOpRemove = 'remove' as const

// WikiTagListResponse wraps the GET /tags response. Empty arrays must
// be returned as `[]`, never omitted — the store normalises both.
export interface WikiTagListResponse {
  tags: WikiTagWithCount[]
}

// WikiPageTagsResponse wraps GET /pages/:slug/tags.
export interface WikiPageTagsResponse {
  tags: WikiTag[]
}