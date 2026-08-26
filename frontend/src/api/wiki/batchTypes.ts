// Build #12 — wiki 页面批量操作公共类型

// WikiBatchFailure is one per-page failure entry returned by the server.
// Code is a stable machine-readable token (`not_found` | `folder_not_found`
// | `folder_conflict` | `folder_not_empty` | `internal`) so the UI can
// render an i18n string per category without re-parsing Error.
export interface WikiBatchFailure {
  slug: string;
  code: string;
  error: string;
}

// WikiBatchResult is the response shape returned by batch-move /
// batch-delete / batch-status. Succeeded is deduped post-input; Failed
// captures per-row errors so the caller can surface partial success.
export interface WikiBatchResult {
  succeeded: string[];
  failed: WikiBatchFailure[];
}

// build the JSON body for batch-move. Trimmed here so the harness / smoke
// script can mirror the call shape without duplicating string logic.
export interface WikiBatchMoveBody {
  slugs: string[];
  folder_id: string;
}

export interface WikiBatchDeleteBody {
  slugs: string[];
}

export interface WikiBatchStatusBody {
  slugs: string[];
  status: string;
}

// WikiBatchErrorCodeToI18nKey maps the server-side code token to the i18n
// key the UI should display in the per-failure detail row. Kept here
// (not in the component) so unit tests can import it without pulling
// vue-i18n / axios. Keys are namespaced under knowledgeEditor.wikiBrowser
// because Build #12 is wiki-scoped and the project already has a separate
// top-level `batchManage` namespace for chat session bulk actions.
export const WikiBatchErrorCodeToI18nKey: Record<string, string> = {
  not_found: 'knowledgeEditor.wikiBrowser.batch.error.notFound',
  folder_not_found: 'knowledgeEditor.wikiBrowser.batch.error.folderNotFound',
  folder_conflict: 'knowledgeEditor.wikiBrowser.batch.error.folderConflict',
  folder_not_empty: 'knowledgeEditor.wikiBrowser.batch.error.folderNotEmpty',
  kb_mismatch: 'knowledgeEditor.wikiBrowser.batch.error.kbMismatch',
  internal: 'knowledgeEditor.wikiBrowser.batch.error.internal',
};

// WikiBatchJobState — possible lifecycle values returned by the
// polling endpoint. The frontend uses these to drive the toast copy
// and to decide whether to surface the undo button.
//
// Build #13.
export type WikiBatchJobState =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'partial';

// WikiBatchJobType — `move` and `delete` are undoable; `status` and
// the (reserved) `tag` are not.
//
// Build #13.
export type WikiBatchJobType = 'move' | 'delete' | 'status' | 'tag';

// WikiBatchJob — row returned by GET /batch-jobs/:id. Result carries
// the per-slug outcome once the worker finishes.
//
// Build #13.
//
// Build #15 — `progress` carries running counters the worker
// publishes on a throttled cadence. Absent on freshly-enqueued jobs
// until the first flush; populated with the terminal snapshot once
// the worker finishes.
export interface WikiBatchJob {
  id: string;
  tenant_id: number;
  knowledge_base_id: string;
  type: WikiBatchJobType;
  params: unknown;
  undo_state?: unknown;
  state: WikiBatchJobState;
  result?: WikiBatchResult | { error: string };
  created_by: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  expires_at?: string;
  progress?: WikiBatchJobProgress;
}

// WikiBatchJobProgress — running counters published by the worker on
// a throttled cadence (every 5 slugs, or on terminal). `total` is
// captured at enqueue time so the polling toast can render a stable
// "{processed}/{total}" fraction across the lifetime of the job.
//
// Build #15.
export interface WikiBatchJobProgress {
  total: number;
  processed: number;
  succeeded: number;
  failed: number;
  updated_at: string;
}

// WikiBatchJobFailureRecord — one row in the per-slug failure ledger
// (wiki_batch_job_failures). Distinct from the audit event stream
// (Build #14) which records "what happened when" — this record tells
// you which slug failed because of what.
//
// Build #15.
export interface WikiBatchJobFailureRecord {
  id: number;
  tenant_id: number;
  knowledge_base_id: string;
  batch_job_id: string;
  slug: string;
  code: string;
  error: string;
  occurred_at: string;
}

// WikiBatchFailureGroupCount — aggregated count for one error code in
// the failure drawer. Used by the code-tab badges.
//
// Build #15.
export interface WikiBatchFailureGroupCount {
  code: string;
  count: number;
}

// WikiBatchFailureListResponse — paginated wrapper for the failure
// drawer. Groups are computed over the full filtered set so the code
// tabs stay accurate on every page.
//
// Build #15.
export interface WikiBatchFailureListResponse {
  failures: WikiBatchJobFailureRecord[];
  groups: WikiBatchFailureGroupCount[];
  total: number;
  page: number;
  page_size: number;
}

// WikiBatchFailureFilter — query parameters for the failure list
// endpoint. `code` empty = no filter; `page_size` is server-clamped
// to 1..200.
//
// Build #15.
export interface WikiBatchFailureFilter {
  code?: string;
  page?: number;
  page_size?: number;
}

// WikiBatchRouteResult is the discriminated response for the three
// /batch-* endpoints under auto-routing. Kind=job means the request
// was queued; Kind=sync means the whole batch ran in-process.
//
// Build #13.
export interface WikiBatchRouteResult {
  kind: 'sync' | 'job';
  result?: WikiBatchResult;
  job?: WikiBatchJob;
}

// WikiBatchJobTerminalStates — once a job reaches one of these the
// polling loop stops. Anything else means "still running".
//
// Build #13.
export const WikiBatchJobTerminalStates: ReadonlyArray<WikiBatchJobState> = [
  'succeeded',
  'failed',
  'partial',
];

// isWikiBatchJobUndoable is a typed predicate the component uses to
// decide whether to surface the Undo button. We mirror the server
// rule (status jobs are not undoable).
//
// Build #13.
export function isWikiBatchJobUndoable(type: WikiBatchJobType): boolean {
  return type === 'move' || type === 'delete';
}

// WikiBatchAuditAction — the seven event kinds the server records in
// `wiki_batch_job_audit`. Mirrors the Go enum; the closed set is the
// union so unknown values become compile errors on the consumer side.
//
// Build #14.
export type WikiBatchAuditAction =
  | 'enqueue'
  | 'start'
  | 'finish'
  | 'undo_request'
  | 'undo_done'
  | 'cancel'
  | 'expire';

// WikiBatchAuditActorSystem is the actor id used when the worker pool
// (or a future cleanup cron) writes a row without a human user.
//
// Build #14.
export const WikiBatchAuditActorSystem = 'system';

// WikiBatchJobAuditEvent — one row in the audit log. Per-job
// cardinality is bounded (<= 7 events), so the drawer can render the
// full chain without pagination.
//
// Build #14.
export interface WikiBatchJobAuditEvent {
  id: number;
  tenant_id: number;
  knowledge_base_id: string;
  batch_job_id: string;
  action: WikiBatchAuditAction;
  actor_id: string;
  occurred_at: string;
  metadata?: Record<string, unknown>;
}

// WikiBatchAuditListResponse — paginated wrapper returned by
// GET /batch-audit. Events are newest-first.
//
// Build #14.
export interface WikiBatchAuditListResponse {
  events: WikiBatchJobAuditEvent[];
  total: number;
  page: number;
  page_size: number;
}

// WikiBatchAuditFilter — query parameters accepted by the list /
// export endpoints. `since` is an RFC3339 string; the server clamps
// it to the last 90 days (D4).
//
// Build #14.
export interface WikiBatchAuditFilter {
  actor?: string;
  action?: WikiBatchAuditAction;
  since?: string;
  page?: number;
  page_size?: number;
}

// WikiBatchPreviewSummary — head-count triple on the dry-run response.
// Pure metadata; the per-slug truth lives in WikiBatchPreviewResponse.
//
// Build #16.
export interface WikiBatchPreviewSummary {
  total: number;
  will_succeed: number;
  will_fail: number;
}

// WikiBatchPreviewResponse — dry-run analogue of WikiBatchResult. Returned
// by the three POST /batch-preview-* endpoints. `success` holds slugs
// that would apply; `failed` reuses WikiBatchFailure so the UI can share
// the i18n key map (WikiBatchErrorCodeToI18nKey) with the real batch
// error UI.
//
// Build #16.
export interface WikiBatchPreviewResponse {
  success: string[];
  failed: WikiBatchFailure[];
  summary: WikiBatchPreviewSummary;
}

// WikiBatchPreviewType — the three preview kinds. Matches the URL
// suffix (`batch-preview-move` | `-delete` | `-status`) so the
// WikiBulkActionBar can route to the right API without per-verb logic
// in the preview dialog itself.
//
// Build #16.
export type WikiBatchPreviewType = 'move' | 'delete' | 'status';

// WikiBatchAsyncThreshold mirrors the Go-side constant in
// `internal/types/wiki_page.go`. The WikiBulkActionBar uses it to decide
// whether to surface the preview button (D7 = A: only when the slug
// count reaches this threshold — small batches skip preview and go
// straight to the synchronous batch-* call).
//
// Keep this value in sync with `internal/types/wiki_page.go`
// (`WikiBatchAsyncThreshold`). If they drift, the preview UX will show
// up too early (preview for small batches) or too late (no preview for
// the async path).
//
// Build #16.
export const WikiBatchAsyncThreshold = 20;