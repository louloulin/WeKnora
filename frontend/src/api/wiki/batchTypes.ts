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