import { get, post, put, del } from "../../utils/request";

// encodeSlugPath encodes each segment of a hierarchical wiki slug (e.g.
// "foo/bar baz?") so the URL is safe while preserving the "/" separators
// between segments. Using encodeURIComponent on the whole slug would also
// escape the "/" and break hierarchical routing on the backend.
function encodeSlugPath(slug: string): string {
  return slug.split("/").map(encodeURIComponent).join("/");
}

// Wiki Page Types
export interface WikiPage {
  id: string;
  tenant_id: number;
  knowledge_base_id: string;
  slug: string;
  title: string;
  page_type: string;
  status: string;
  content: string;
  // Sanitized HTML render cached by the WYSIWYG editor (Build #2b).
  // Empty string / null means "use the legacy markdown render". Callers
  // MUST treat it as untrusted text and pipe it through the same
  // sanitizer used by the editor before injecting into `v-html`.
  content_html?: string;
  summary: string;
  aliases: string[];
  parent_slug?: string;
  category_path?: string[];
  wiki_path?: string;
  depth?: number;
  sort_order?: number;
  source_refs: string[];
  in_links: string[];
  out_links: string[];
  page_metadata: Record<string, any>;
  version: number;
  // Author kind of the current version: 'pipeline' | 'agent' | 'user' |
  // 'revert'. Empty/missing on legacy rows (treat as 'pipeline').
  last_edit_source?: string;
  last_editor_id?: string;
  created_at: string;
  updated_at: string;
}

export interface WikiPageListResponse {
  pages: WikiPage[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface WikiFolder {
  id: string;
  tenant_id: number;
  knowledge_base_id: string;
  parent_id: string;
  name: string;
  path: string;
  depth: number;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface WikiFolderNode extends WikiFolder {
  page_count: number;
  has_children: boolean;
}

export interface WikiFolderListResponse {
  parent_id: string;
  folders: WikiFolderNode[];
}

export interface WikiGraphMeta {
  mode: 'overview' | 'ego' | string;
  total: number;
  returned: number;
  truncated: boolean;
  center?: string;
  depth?: number;
  familiar_count?: number;
}

export interface WikiGraphData {
  nodes: { slug: string; title: string; page_type: string; link_count: number; familiar?: boolean }[];
  edges: { source: string; target: string }[];
  meta: WikiGraphMeta;
}

export interface WikiStats {
  total_pages: number;
  pages_by_type: Record<string, number>;
  total_links: number;
  orphan_count: number;
  recent_updates: WikiPage[];
  pending_tasks: number;
  pending_issues: number;
  is_active: boolean;
}

export interface WikiPageIssue {
  id: string;
  tenant_id: number;
  knowledge_base_id: string;
  slug: string;
  issue_type: string;
  description: string;
  suspected_knowledge_ids: string[];
  status: string;
  reported_by: string;
  created_at: string;
  updated_at: string;
}

// Wiki API Functions
export function listWikiPages(kbId: string, params?: {
  page_type?: string;
  status?: string;
  query?: string;
  category_path?: string;
  category_depth?: number;
  page?: number;
  page_size?: number;
  sort_by?: string;
  sort_order?: string;
}) {
  const query = new URLSearchParams();
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== '') {
        query.set(key, String(value));
      }
    });
  }
  const qs = query.toString();
  return get(`/api/v1/knowledgebase/${kbId}/wiki/pages${qs ? '?' + qs : ''}`);
}

// listWikiFolders returns the direct child folders of parentId ("" = root),
// each enriched with a recursive page_count and a has_children flag so the tree
// can render expand affordances and empty folders without a second request.
// pageTypes scopes the view to a sidebar tab: only folders whose subtree holds
// a page of those types (or are entirely empty) come back, and page_count is
// counted within those types.
export function listWikiFolders(kbId: string, parentId = '', pageTypes = '') {
  const query = new URLSearchParams();
  if (parentId) query.set('parent_id', parentId);
  if (pageTypes) query.set('page_types', pageTypes);
  const qs = query.toString();
  return get(`/api/v1/knowledgebase/${kbId}/wiki/folders${qs ? '?' + qs : ''}`);
}

// createWikiFolder creates a new empty folder under parentId ("" = root).
export function createWikiFolder(kbId: string, parentId: string, name: string) {
  return post(`/api/v1/knowledgebase/${kbId}/wiki/folders`, { parent_id: parentId, name });
}

// updateWikiFolder renames and/or reparents a folder. Pass move_parent: true
// (and parent_id) to reparent; omit it for a pure rename.
export function updateWikiFolder(
  kbId: string,
  folderId: string,
  data: { name?: string; parent_id?: string; move_parent?: boolean },
) {
  return put(`/api/v1/knowledgebase/${kbId}/wiki/folders/${folderId}`, data);
}

// deleteWikiFolder removes an empty folder (no pages, no sub-folders).
export function deleteWikiFolder(kbId: string, folderId: string) {
  return del(`/api/v1/knowledgebase/${kbId}/wiki/folders/${folderId}`);
}

// moveWikiPage relocates a page into folderId ("" = root). The slug is sent in
// the body because wiki slugs are hierarchical.
export function moveWikiPage(kbId: string, slug: string, folderId: string) {
  return put(`/api/v1/knowledgebase/${kbId}/wiki/move-page`, { slug, folder_id: folderId });
}

export function createWikiPage(kbId: string, data: Partial<WikiPage>) {
  return post(`/api/v1/knowledgebase/${kbId}/wiki/pages`, data);
}

export function getWikiPage(kbId: string, slug: string) {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}`);
}

// Re-exported from `backlinksTypes.ts` so consumers can keep
// importing from the canonical `api/wiki` entry point while
// the helpers module + Node tests pull the same shape.
import type { WikiPageBacklink } from './backlinksTypes';
export type { WikiPageBacklink };

// Build #12 — wiki 页面批量操作公共类型
// Build #13 — 加 WikiBatchRouteResult + WikiBatchJob 用于异步路径
// Build #14 — 加审计事件类型
import {
  WikiBatchJobTerminalStates,
  isWikiBatchJobUndoable,
  WikiBatchAuditActorSystem,
  WikiBatchErrorCodeToI18nKey,
} from './batchTypes';
import type {
  WikiBatchResult,
  WikiBatchRouteResult,
  WikiBatchJob,
  WikiBatchJobState,
  WikiBatchJobType,
  WikiBatchMoveBody,
  WikiBatchDeleteBody,
  WikiBatchStatusBody,
  WikiBatchJobAuditEvent,
  WikiBatchAuditAction,
  WikiBatchAuditListResponse,
  WikiBatchAuditFilter,
  WikiBatchJobProgress,
  WikiBatchJobFailureRecord,
  WikiBatchFailureGroupCount,
  WikiBatchFailureListResponse,
  WikiBatchFailureFilter,
} from './batchTypes';
export type {
  WikiBatchResult,
  WikiBatchRouteResult,
  WikiBatchJob,
  WikiBatchJobState,
  WikiBatchJobType,
  WikiBatchMoveBody,
  WikiBatchDeleteBody,
  WikiBatchStatusBody,
  WikiBatchJobAuditEvent,
  WikiBatchAuditAction,
  WikiBatchAuditListResponse,
  WikiBatchAuditFilter,
  WikiBatchJobProgress,
  WikiBatchJobFailureRecord,
  WikiBatchFailureGroupCount,
  WikiBatchFailureListResponse,
  WikiBatchFailureFilter,
};
export {
  WikiBatchJobTerminalStates,
  isWikiBatchJobUndoable,
  WikiBatchAuditActorSystem,
  WikiBatchErrorCodeToI18nKey,
};

// getWikiPageBacklinks returns the set of pages that link to
// `slug` within `kbId`, ordered by updatedAt desc with the
// backend handling orphan filtering. The server contract is
// empty-array + 200 when the page exists but has no inbound
// links; 404 when the page itself does not exist.
export function getWikiPageBacklinks(kbId: string, slug: string) {
  return get<WikiPageBacklink[]>(
    `/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}/backlinks`,
  );
}

// Build #12 — wiki 页面批量操作端点。三个 POST 端点共用
// `WikiBatchResult` 响应形状;slugs 在服务端去重 + 空字符串剔除。
// `folder_id` 空字符串表示移至 root。
//
// Build #13 — 三个端点改为返回 `WikiBatchRouteResult`;客户端根据
// `kind` 字段决定是直接展示同步结果,还是改走异步 toast + 轮询。

export function batchMoveWikiPages(
  kbId: string,
  slugs: string[],
  folderId: string,
) {
  const body: WikiBatchMoveBody = { slugs, folder_id: folderId };
  return post<WikiBatchRouteResult>(
    `/api/v1/knowledgebase/${kbId}/wiki/pages/batch-move`,
    body,
  );
}

export function batchDeleteWikiPages(kbId: string, slugs: string[]) {
  const body: WikiBatchDeleteBody = { slugs };
  return post<WikiBatchRouteResult>(
    `/api/v1/knowledgebase/${kbId}/wiki/pages/batch-delete`,
    body,
  );
}

export function batchUpdateWikiPagesStatus(
  kbId: string,
  slugs: string[],
  status: string,
) {
  const body: WikiBatchStatusBody = { slugs, status };
  return post<WikiBatchRouteResult>(
    `/api/v1/knowledgebase/${kbId}/wiki/pages/batch-status`,
    body,
  );
}

// Build #13 — async job status + undo.

// getWikiBatchJob polls a job's progress. Default 2s cadence is the
// component's responsibility; this function does not schedule its own
// retries.
export function getWikiBatchJob(kbId: string, jobId: string) {
  return get<WikiBatchJob>(
    `/api/v1/knowledgebase/${kbId}/wiki/batch-jobs/${jobId}`,
  );
}

// undoWikiBatchJob triggers the server-side undo path. The returned
// WikiBatchJob has `expires_at` cleared; status jobs return 422 (caller
// should never invoke this for status jobs).
export function undoWikiBatchJob(kbId: string, jobId: string) {
  return post<WikiBatchJob>(
    `/api/v1/knowledgebase/${kbId}/wiki/batch-jobs/${jobId}/undo`,
    {},
  );
}

// cancelWikiBatchJob aborts a queued batch before any worker picks it
// up. The server returns 409 once the job has moved past queued.
//
// Build #14.
export function cancelWikiBatchJob(kbId: string, jobId: string) {
  return post<WikiBatchJob>(
    `/api/v1/knowledgebase/${kbId}/wiki/batch-jobs/${jobId}/cancel`,
    {},
  );
}

// getWikiBatchJobAudit returns every audit event for one batch job,
// oldest-first. Per-job cardinality is bounded (<= 7 events), so the
// drawer renders the full chain without pagination.
//
// Build #14.
export function getWikiBatchJobAudit(kbId: string, jobId: string) {
  return get<WikiBatchJobAuditEvent[]>(
    `/api/v1/knowledgebase/${kbId}/wiki/batch-jobs/${jobId}/audit`,
  );
}

// getWikiBatchJobFailures returns the per-slug failure ledger for one
// batch job, oldest-first, paginated. Optional `code` filter narrows
// to one error bucket; the `groups` slice is always computed over the
// full filtered set so the drawer's code tabs stay accurate on every
// page.
//
// Build #15.
export function getWikiBatchJobFailures(
  kbId: string,
  jobId: string,
  filter: WikiBatchFailureFilter = {},
) {
  return get<WikiBatchFailureListResponse>(
    `/api/v1/knowledgebase/${kbId}/wiki/batch-jobs/${jobId}/failures`,
    filter as Record<string, unknown>,
  );
}

// listWikiBatchAudit lists KB-wide audit events, newest-first. Filters
// are optional; the server clamps `since` to the last 90 days.
//
// Build #14.
export function listWikiBatchAudit(
  kbId: string,
  filter: WikiBatchAuditFilter = {},
) {
  return get<WikiBatchAuditListResponse>(
    `/api/v1/knowledgebase/${kbId}/wiki/batch-audit`,
    filter as Record<string, unknown>,
  );
}

// exportWikiBatchAuditCsv streams the filtered events as
// text/csv. Returns the raw Axios response so the caller can grab the
// blob via `response.data`. The server caps the export at 10000
// rows to keep response times sane.
//
// Build #14.
export function exportWikiBatchAuditCsv(
  kbId: string,
  filter: Pick<WikiBatchAuditFilter, 'actor' | 'since'> = {},
) {
  const query = new URLSearchParams();
  if (filter.actor) query.set('actor', filter.actor);
  if (filter.since) query.set('since', filter.since);
  const qs = query.toString();
  const suffix = qs ? `?${qs}` : '';
  return get<Blob>(
    `/api/v1/knowledgebase/${kbId}/wiki/batch-audit/export${suffix}`,
    { responseType: 'blob' },
  );
}

// WikiPageUpdatePayload is a partial update: absent fields keep their stored
// value. `version` is the optimistic-lock guard — send the version the page
// had when the user started editing; the backend answers 409 (with
// `current_version` in the body) when someone else edited in between.
export interface WikiPageUpdatePayload {
  title?: string;
  content?: string;
  // content_html is the sanitized render produced by the WYSIWYG editor
  // (Build #2b Decision 2). It is dual-written with `content` so existing
  // markdown consumers keep working while the new editor path stores a
  // pre-rendered cache. Absent / empty clears the cached HTML on the
  // server side and falls back to `content` for rendering.
  content_html?: string | null;
  summary?: string;
  page_type?: string;
  status?: string;
  aliases?: string[];
  version?: number;
}

export function updateWikiPage(kbId: string, slug: string, data: WikiPageUpdatePayload) {
  return put(`/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}`, data);
}

export function deleteWikiPage(kbId: string, slug: string) {
  return del(`/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}`);
}

// WikiPageRevision is one immutable snapshot of a superseded page version.
// `content` is only populated when fetching a single revision.
export interface WikiPageRevision {
  id: string;
  tenant_id: number;
  knowledge_base_id: string;
  page_id: string;
  slug: string;
  version: number;
  title: string;
  page_type: string;
  status: string;
  content?: string;
  summary: string;
  aliases: string[];
  edit_source: string;
  editor_id: string;
  edited_at: string;
  created_at: string;
}

export interface WikiRevisionListResponse {
  revisions: WikiPageRevision[];
  total: number;
  current_version: number;
}

// listWikiRevisions returns the page's historical snapshots newest-first
// (content omitted) plus the current version number. The current version has
// no revision row — it is the page itself.
export function listWikiRevisions(kbId: string, slug: string, params?: { limit?: number; offset?: number }) {
  const query = new URLSearchParams();
  if (params?.limit !== undefined) query.set('limit', String(params.limit));
  if (params?.offset !== undefined) query.set('offset', String(params.offset));
  const qs = query.toString();
  return get(`/api/v1/knowledgebase/${kbId}/wiki/revisions/${encodeSlugPath(slug)}${qs ? '?' + qs : ''}`);
}

// getWikiRevision returns one snapshot with full content.
export function getWikiRevision(kbId: string, slug: string, version: number) {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/revisions/${encodeSlugPath(slug)}?version=${version}`);
}

// revertWikiPage rolls the page back to a stored revision. Applied as a
// regular edit: the pre-revert state is snapshotted and version advances,
// so a revert is itself revertable.
export function revertWikiPage(kbId: string, slug: string, version: number) {
  return post(`/api/v1/knowledgebase/${kbId}/wiki/revert`, { slug, version });
}

export interface WikiIndexEntryDTO {
  slug: string;
  title: string;
  summary: string;
  parent_slug?: string;
  category_path?: string[];
  wiki_path?: string;
  depth?: number;
  sort_order?: number;
}

export interface WikiIndexGroup {
  type: string;
  total: number;
  items: WikiIndexEntryDTO[];
  next_cursor?: string;
}

export interface WikiIndexResponse {
  intro: string;
  version: number;
  groups: WikiIndexGroup[];
}

// getWikiIndex fetches the structured index view for a wiki KB. The
// backend replaced the legacy "markdown blob of intro + directory" with
// { intro, groups } so a 40k-page wiki no longer round-trips multiple
// megabytes on every index open. Pass `types` to restrict which
// page_type buckets come back; `limit` bounds the per-group window;
// `cursor` resumes from a previous response.
export function getWikiIndex(
  kbId: string,
  params?: { types?: string[]; limit?: number; cursor?: string },
) {
  const query = new URLSearchParams();
  if (params) {
    if (params.types && params.types.length > 0) query.set('types', params.types.join(','));
    if (params.limit !== undefined) query.set('limit', String(params.limit));
    if (params.cursor) query.set('cursor', params.cursor);
  }
  const qs = query.toString();
  const suffix = qs ? `?${qs}` : '';
  return get(`/api/v1/knowledgebase/${kbId}/wiki/index${suffix}`);
}

export interface WikiGraphQueryParams {
  mode?: 'overview' | 'ego';
  center?: string;
  depth?: number;
  types?: string[];
  limit?: number;
}

// getWikiGraph fetches a slice of the wiki link graph. Without params the
// backend returns the top-500 most-connected pages (overview mode). Pass
// `mode: 'ego', center: <slug>` to drill into a specific page's neighborhood.
// For knowledge bases with tens of thousands of pages the overview cap is
// what prevents the browser from choking on a 30MB payload / 100k SVG nodes.
export function getWikiGraph(kbId: string, params?: WikiGraphQueryParams) {
  const query = new URLSearchParams();
  if (params) {
    if (params.mode) query.set('mode', params.mode);
    if (params.center) query.set('center', params.center);
    if (params.depth !== undefined) query.set('depth', String(params.depth));
    if (params.limit !== undefined) query.set('limit', String(params.limit));
    if (params.types && params.types.length > 0) {
      query.set('types', params.types.join(','));
    }
  }
  const qs = query.toString();
  return get(`/api/v1/knowledgebase/${kbId}/wiki/graph${qs ? '?' + qs : ''}`);
}

export function getWikiStats(kbId: string) {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/stats`);
}

export function searchWikiPages(kbId: string, q: string, limit?: number) {
  const params = new URLSearchParams({ q });
  if (limit) params.set('limit', String(limit));
  return get(`/api/v1/knowledgebase/${kbId}/wiki/search?${params.toString()}`);
}

export function listWikiIssues(kbId: string, slug?: string, status?: string) {
  const params = new URLSearchParams();
  if (slug) params.set('slug', slug);
  if (status) params.set('status', status);
  return get(`/api/v1/knowledgebase/${kbId}/wiki/issues?${params.toString()}`);
}

export function updateWikiIssueStatus(kbId: string, issueId: string, status: string) {
  return put(`/api/v1/knowledgebase/${kbId}/wiki/issues/${issueId}/status`, { status });
}

export function rebuildWikiLinks(kbId: string) {
  return post(`/api/v1/knowledgebase/${kbId}/wiki/rebuild-links`, {});
}
