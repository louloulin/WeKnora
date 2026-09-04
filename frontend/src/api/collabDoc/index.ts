/**
 * Collaborative document API client.
 *
 * v0.7.25 — collaborative_docs (Feishu / Tencent document parity).
 *
 * Wraps the REST surface exposed by the backend at /api/v1/collaborative-docs.
 * The realtime Yjs WebSocket connection is established by the editor
 * component itself (see useYjsCollabDoc) so the Yjs binary framing stays
 * out of the typed HTTP layer.
 */
import { get, post, patch, del, postUpload } from "@/utils/request";

export type CollabDocKind = "doc" | "sheet" | "slide";

export interface CollabDoc {
  id: string;
  tenant_id: number;
  kb_id: string;
  title: string;
  doc_kind: CollabDocKind;
  schema_version: number;
  owner_user_id: number;
  visibility: string;
  share_token: string;
  /** Optional share-link expiry (ISO 8601) — set when the owner enabled share. */
  share_expires_at?: string | null;
  /** True iff the share link is currently password-protected. */
  share_password_protected?: boolean;
  created_at: string;
  updated_at: string;
  archived_at?: string | null;
}

export interface CollabDocSession {
  id: string;
  tenant_id: number;
  doc_id: string;
  user_id: number;
  client_id: number;
  color: string;
  display_name: string;
  last_heartbeat: string;
  joined_at: string;
}

export interface ListCollabDocsFilter {
  kb_id?: string;
  doc_kind?: CollabDocKind;
  archived?: boolean;
  limit?: number;
  offset?: number;
}

interface ApiEnvelope<T> {
  success: boolean;
  data: T;
  total?: number;
}

export async function listCollabDocs(
  filter: ListCollabDocsFilter = {},
): Promise<{ items: CollabDoc[]; total: number }> {
  const r = await get<ApiEnvelope<CollabDoc[]> & { total?: number }>(
    "/collaborative-docs",
    { params: filter as Record<string, unknown> },
  );
  return { items: r.data ?? [], total: r.total ?? 0 };
}

export async function getCollabDoc(id: string): Promise<CollabDoc> {
  const r = await get<ApiEnvelope<CollabDoc>>(`/collaborative-docs/${id}`);
  return r.data;
}

export async function createCollabDoc(payload: {
  kb_id: string;
  title: string;
  doc_kind?: CollabDocKind;
}): Promise<CollabDoc> {
  const r = await post<ApiEnvelope<CollabDoc>>("/collaborative-docs", payload);
  return r.data;
}

export async function updateCollabDoc(
  id: string,
  payload: { title?: string; visibility?: string },
): Promise<CollabDoc> {
  const r = await patch<ApiEnvelope<CollabDoc>>(
    `/collaborative-docs/${id}`,
    payload,
  );
  return r.data;
}

export async function archiveCollabDoc(id: string): Promise<void> {
  await post(`/collaborative-docs/${id}/archive`, {});
}

export async function deleteCollabDoc(id: string): Promise<void> {
  await del(`/collaborative-docs/${id}`);
}

export async function listCollabDocPresence(
  id: string,
): Promise<CollabDocSession[]> {
  const r = await get<ApiEnvelope<CollabDocSession[]>>(
    `/collaborative-docs/${id}/presence`,
  );
  return r.data ?? [];
}

/** Trigger a KB sync. Backend dispatches to docparser /chunk. */
export async function syncCollabDocToKB(id: string): Promise<unknown> {
  return post(`/collaborative-docs/${id}/sync-to-kb`, {});
}

/** Upload a binary blob (e.g. .docx) as the document body. */
export async function uploadCollabDocBytes(
  id: string,
  bytes: Uint8Array,
  filename: string,
): Promise<unknown> {
  const form = new FormData();
  const buf = bytes.buffer.slice(
    bytes.byteOffset,
    bytes.byteOffset + bytes.byteLength,
  ) as ArrayBuffer;
  form.append("file", new Blob([buf]), filename);
  return postUpload(`/collaborative-docs/${id}/upload`, form);
}

/** Download the current binary blob of a document. */
export async function downloadCollabDocBytes(id: string): Promise<Blob> {
  const res = await get<Blob>(`/collaborative-docs/${id}/download`, {
    responseType: "blob",
  });
  return res;
}

/** WebSocket URL for the Yjs collaborative channel. */
export function openCollabDocRealtimeURL(docId: string, token: string): string {
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  const host = window.location.host;
  // y-websocket appends its room name after serverUrl. Keep the server URL
  // query-free; callers pass token through WebsocketProvider.params so the
  // final URL is /realtime/<room>?token=<jwt>, not ?token=<jwt>/<room>.
  void token;
  return `${proto}//${host}/api/v1/collaborative-docs/${encodeURIComponent(docId)}/realtime`;
}

// ── v0.7.29 — comments ────────────────────────────────────────────────

export type CommentAnchorType = "doc" | "slide" | "sheet";

export interface CollabDocComment {
  id: number;
  tenant_id: number;
  doc_id: string;
  thread_id: string;
  parent_id?: number | null;
  author_user_id: number;
  author_name: string;
  author_color: string;
  anchor_type: CommentAnchorType;
  anchor_ref: string;
  body: string;
  resolved: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateCollabDocCommentRequest {
  thread_id?: string;
  parent_id?: number;
  anchor_type: CommentAnchorType;
  anchor_ref?: string;
  body: string;
  /**
   * v0.7.197 — User IDs captured when the composer typed `@` and
   * picked from the mention popover. Each id is sent through the
   * backend's mention fan-out → emits a `wiki.mentioned` notification.
   * Best-effort: the comment write succeeds even if some notifications
   * fail to deliver.
   */
  mentioned_user_ids?: string[];
}

export interface UpdateCollabDocCommentRequest {
  body?: string;
  resolved?: boolean;
}

/** Fetch every comment message for a doc (chronological). */
export async function listCollabDocComments(
  id: string,
  filter: {
    thread_id?: string;
    resolved?: boolean;
    limit?: number;
    offset?: number;
  } = {},
): Promise<{ comments: CollabDocComment[] }> {
  const params = new URLSearchParams();
  if (filter.thread_id) params.set("thread_id", filter.thread_id);
  if (filter.resolved !== undefined)
    params.set("resolved", String(filter.resolved));
  if (filter.limit !== undefined) params.set("limit", String(filter.limit));
  if (filter.offset !== undefined) params.set("offset", String(filter.offset));
  const qs = params.toString();
  return get(`/collaborative-docs/${id}/comments${qs ? `?${qs}` : ""}`);
}

/** Add a comment message (starts a new thread when thread_id is omitted). */
export async function createCollabDocComment(
  id: string,
  req: CreateCollabDocCommentRequest,
): Promise<CollabDocComment> {
  return post(`/collaborative-docs/${id}/comments`, req);
}

/** Edit a comment body or resolved flag. */
export async function updateCollabDocComment(
  id: string,
  commentID: number,
  patch: UpdateCollabDocCommentRequest,
): Promise<CollabDocComment> {
  return post(`/collaborative-docs/${id}/comments/${commentID}`, patch);
}

/** Delete a comment (replies cascade via FK). */
export async function deleteCollabDocComment(
  id: string,
  commentID: number,
): Promise<void> {
  return del(`/collaborative-docs/${id}/comments/${commentID}`);
}

// ── v0.7.30 — audit log ────────────────────────────────────────────────

export type CollabAuditAction =
  | "create"
  | "rename"
  | "upload"
  | "save"
  | "share.enable"
  | "share.disable"
  | "share.access"
  | "archive"
  | "restore"
  | "delete"
  | "comment.add"
  | "comment.reply"
  | "comment.solve"
  | "comment.delete"
  | "polish"
  | "sync_to_kb"
  | "export";

export interface CollabDocAuditEntry {
  id: number;
  tenant_id: number;
  doc_id: string;
  actor_user_id: number;
  actor_name: string;
  actor_color: string;
  action: CollabAuditAction;
  target: string;
  payload: string;
  ip: string;
  user_agent: string;
  created_at: string;
}

export interface CollabDocAuditSummary {
  total_entries: number;
  by_action: Partial<Record<CollabAuditAction, number>>;
  by_day: Array<{ day: string; count: number }>;
}

export interface ListCollabDocAuditFilter {
  actor?: number;
  action?: CollabAuditAction;
  /** RFC3339 lower bound */
  since?: string;
  /** RFC3339 upper bound */
  until?: string;
  limit?: number;
  offset?: number;
}

/** Fetch every audit entry for a single doc (newest first). */
export async function listCollabDocAudit(
  id: string,
  filter: ListCollabDocAuditFilter = {},
): Promise<{ entries: CollabDocAuditEntry[] }> {
  const qs = new URLSearchParams();
  if (filter.actor != null) qs.set("actor", String(filter.actor));
  if (filter.action) qs.set("action", filter.action);
  if (filter.since) qs.set("since", filter.since);
  if (filter.until) qs.set("until", filter.until);
  if (filter.limit != null) qs.set("limit", String(filter.limit));
  if (filter.offset != null) qs.set("offset", String(filter.offset));
  const u = `/collaborative-docs/${id}/audit${qs.toString() ? `?${qs.toString()}` : ""}`;
  return get(u);
}

/** Rolled-up audit counts by action + day (powers the activity chart). */
export async function collabDocAuditSummary(
  filter: ListCollabDocAuditFilter & { doc?: string } = {},
): Promise<CollabDocAuditSummary> {
  const qs = new URLSearchParams();
  if (filter.doc) qs.set("doc", filter.doc);
  if (filter.action) qs.set("action", filter.action);
  if (filter.since) qs.set("since", filter.since);
  if (filter.until) qs.set("until", filter.until);
  const u = `/collaborative-docs/audit/summary${qs.toString() ? `?${qs.toString()}` : ""}`;
  return get(u);
}

// ---------------------------------------------------------------------------
// Share link API (v0.7.38 — share_token + password + expiry).
//
// POST   /collaborative-docs/:id/share  →  EnableShare (returns { share_token, ... })
// DELETE /collaborative-docs/:id/share  →  DisableShare (204)
// GET    /collaborative-docs/share/:token/download?password=X → ShareDownload
// ---------------------------------------------------------------------------

export interface EnableShareRequest {
  /** ≥6 char password. Empty string means an open link (no password). */
  password: string;
  /** Optional ISO-8601 expiry. Null means never expire. */
  expires_at?: string | null;
}

export interface EnableShareResponse {
  id: string;
  share_token: string;
  expires_at: string | null;
  /** True iff the link requires the X-Share-Password header. */
  protected: boolean;
  /** Absolute URL the user can paste into Slack / email. */
  url: string;
}

export async function enableCollabDocShare(
  id: string,
  req: EnableShareRequest,
): Promise<EnableShareResponse> {
  return post(`/collaborative-docs/${id}/share`, req);
}

export async function disableCollabDocShare(id: string): Promise<void> {
  await del(`/collaborative-docs/${id}/share`);
}

/**
 * Download the share-view bytes for a doc. Returns the raw .docx/.pptx/.xlsx
 * bytes as a Blob. Pass the share password via the X-Share-Password header
 * when the link is protected (the backend returns 403 + WWW-Authenticate
 * header when the password is missing or wrong).
 */
export async function downloadCollabDocShare(
  shareToken: string,
  password?: string,
): Promise<Blob> {
  const headers: Record<string, string> = {};
  if (password) headers["X-Share-Password"] = password;
  const u = `/collaborative-docs/share/${encodeURIComponent(shareToken)}/download`;
  const res = await fetch(u, { headers });
  if (!res.ok) {
    throw new Error(`share download failed: HTTP ${res.status}`);
  }
  return res.blob();
}
// v0.7.90 — form responses (Tencent Docs 收集表 parity).
export interface FormResponse {
  id: number;
  tenant_id: number;
  doc_id: string;
  submitter_token: string;
  submitter_name: string;
  submitter_user_id: number;
  answers: string; // raw JSON string
  client_ip: string;
  user_agent: string;
  created_at: string;
}
export interface FormResponseQuestionSummary {
  question_id: string;
  question_type: string;
  question_title: string;
  total: number;
  counts?: Record<string, number>;
  latest_sample?: string[];
}
export interface FormResponseSummary {
  total: number;
  by_question: FormResponseQuestionSummary[];
}
export interface ListFormResponsesFilter {
  limit?: number;
  offset?: number;
}

export async function getFormResponses(
  docId: string,
  filter: ListFormResponsesFilter = {},
): Promise<{ items: FormResponse[]; total: number }> {
  const r = await get<{
    success: boolean;
    data: { items: FormResponse[]; total: number };
  }>(`/collaborative-docs/${docId}/responses`, {
    params: filter as Record<string, unknown>,
  });
  return r.data;
}

export async function getFormResponseSummary(
  docId: string,
): Promise<FormResponseSummary> {
  const r = await get<{ success: boolean; data: FormResponseSummary }>(
    `/collaborative-docs/${docId}/responses/summary`,
  );
  return r.data;
}
