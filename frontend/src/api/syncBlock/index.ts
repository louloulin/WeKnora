// v0.7.20 — wiki synced blocks REST client.
//
// 7 endpoints mirror internal/handler/wiki_sync_block.go:
//   POST   /api/v1/knowledgebase/:kb_id/wiki/sync-blocks                 (create)
//   GET    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks                 (list for picker)
//   GET    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id       (get canonical)
//   PUT    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id       (update)
//   DELETE /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id       (delete, mode=cascade|unlink)
//   GET    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id/stats (fan-out stats)
//   GET    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id/refs  (refs list)
//
// Auth: the rbacGuards stack upstream enforces KB-Viewer / KB-Editor roles.
// We never pass tenant_id / user_id from the client — middleware injects them.

import { get, post, put, del } from "../../utils/request";
import type {
  WikiSyncBlock,
  WikiSyncBlockListQuery,
  WikiSyncBlockListResponse,
  WikiSyncBlockRefListResponse,
  WikiSyncBlockRefStats,
  WikiSyncBlockUpsertRequest,
} from "./types";

const base = (kbId: string) =>
  `/api/v1/knowledgebase/${encodeURIComponent(kbId)}/wiki/sync-blocks`;

// Create inserts a new canonical synced block.
export async function createSyncBlock(
  kbId: string,
  body: WikiSyncBlockUpsertRequest,
): Promise<WikiSyncBlock> {
  return post<WikiSyncBlock>(base(kbId), body);
}

// List returns canonical blocks for the picker UI.
export async function listSyncBlocks(
  kbId: string,
  query: WikiSyncBlockListQuery = {},
): Promise<WikiSyncBlockListResponse> {
  const params: Record<string, string> = {};
  if (query.limit !== undefined) params.limit = String(query.limit);
  if (query.offset !== undefined) params.offset = String(query.offset);
  return get<WikiSyncBlockListResponse>(base(kbId), { params });
}

// Get returns one canonical block.
export async function getSyncBlock(
  kbId: string,
  blockId: string,
): Promise<WikiSyncBlock> {
  return get<WikiSyncBlock>(`${base(kbId)}/${encodeURIComponent(blockId)}`);
}

// Update replaces content on an existing synced block, bumping version.
export async function updateSyncBlock(
  kbId: string,
  blockId: string,
  body: WikiSyncBlockUpsertRequest,
): Promise<WikiSyncBlock> {
  return put<WikiSyncBlock>(
    `${base(kbId)}/${encodeURIComponent(blockId)}`,
    body,
  );
}

// Delete removes a canonical block. mode controls ref fate:
//   - "cascade" — remove every ref too (default; matches Notion behaviour)
//   - "unlink"  — leave refs in place but mark content_version = 0
export async function deleteSyncBlock(
  kbId: string,
  blockId: string,
  mode: "cascade" | "unlink" = "cascade",
): Promise<void> {
  await del<void>(
    `${base(kbId)}/${encodeURIComponent(blockId)}?mode=${mode}`,
  );
}

// Stats returns fan-out reach for the picker UI badge.
export async function getSyncBlockStats(
  kbId: string,
  blockId: string,
): Promise<WikiSyncBlockRefStats> {
  return get<WikiSyncBlockRefStats>(
    `${base(kbId)}/${encodeURIComponent(blockId)}/stats`,
  );
}

// ListRefs returns every page that embeds the block.
export async function listSyncBlockRefs(
  kbId: string,
  blockId: string,
): Promise<WikiSyncBlockRefListResponse> {
  return get<WikiSyncBlockRefListResponse>(
    `${base(kbId)}/${encodeURIComponent(blockId)}/refs`,
  );
}

// High-level helper: parse `[[sync:UUID]]` markers from page content so the
// editor can show a "X synced blocks referenced" indicator. Re-exports
// the regex helper for callers that want to inline scan.
export { extractSyncMarkers } from "./types";
