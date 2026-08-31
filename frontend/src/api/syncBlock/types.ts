// v0.7.20 — wiki synced blocks (Notion Synced Blocks / 飞书同步块 parity)
//
// Type definitions for the synced-block API surface. Mirrors the Go
// structs in internal/types/wiki_sync_block.go exactly — every field
// round-trips through JSON without coercion.

// WikiSyncBlock is the canonical source for a reusable content block.
// Edit it once, every embedded reference re-renders to the latest
// version automatically.
export interface WikiSyncBlock {
  id: number;
  tenant_id: number;
  kb_id: string;
  block_id: string;
  title: string;
  content_json: string; // JSON-encoded string from server; parse with JSON.parse when needed
  content_md: string;
  version: number;
  owner_id: number;
  created_at: string;
  updated_at: string;
}

// WikiSyncBlockRef is one embedded reference to a synced block. The
// `content_version` lets the renderer know whether the embedded copy
// is fresh or stale relative to the canonical block.
export interface WikiSyncBlockRef {
  id: number;
  tenant_id: number;
  kb_id: string;
  block_id: string;
  page_id: string;
  anchor_slug: string;
  content_version: number;
  rendered_at: string;
  created_at: string;
}

// WikiSyncBlockRefStats summarises fan-out reach for a single block.
export interface WikiSyncBlockRefStats {
  block_id: string;
  ref_count: number;
  pages_count: number;
  stale_ref_count: number;
  current_version: number;
}

// WikiSyncBlockUpsertRequest is the create / replace payload.
export interface WikiSyncBlockUpsertRequest {
  block_id: string;
  title: string;
  content_json: unknown; // object or string; the API accepts both
  content_md: string;
}

// WikiSyncBlockListResponse wraps the picker UI's list response.
export interface WikiSyncBlockListResponse {
  blocks: WikiSyncBlock[];
  total: number;
}

// WikiSyncBlockRefListResponse wraps the refs listing.
export interface WikiSyncBlockRefListResponse {
  refs: WikiSyncBlockRef[];
  total: number;
}

// WikiSyncBlockListQuery — query parameters for the list endpoint.
export interface WikiSyncBlockListQuery {
  limit?: number;
  offset?: number;
}

// SyncBlockMarker is a `[[sync:UUID]]` reference extracted from page content.
export interface SyncBlockMarker {
  block_id: string;
  anchor_slug: string;
  // Raw substring for round-trip replacement.
  raw: string;
}

// ExtractSyncMarkers scans content for `[[sync:UUID]]` markers.
// Optionally accepts a second regex group for `[[sync:UUID#anchor]]` form.
export const SYNC_MARKER_RE = /\[\[sync:([0-9a-fA-F-]{36})(?:#([\w-]+))?\]\]/g;

export function extractSyncMarkers(content: string): SyncBlockMarker[] {
  const out: SyncBlockMarker[] = [];
  if (!content) return out;
  let match: RegExpExecArray | null;
  // Reset regex state per call (lastIndex is shared global).
  SYNC_MARKER_RE.lastIndex = 0;
  while ((match = SYNC_MARKER_RE.exec(content)) !== null) {
    out.push({
      block_id: match[1],
      anchor_slug: match[2] || "",
      raw: match[0],
    });
  }
  return out;
}
