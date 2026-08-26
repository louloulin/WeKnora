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