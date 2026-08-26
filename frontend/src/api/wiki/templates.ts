import { post } from '../../utils/request'
import type {
  WikiApplyTemplateRequest,
  WikiApplyTemplateResult,
} from './templateTypes'

export {
  type ParsedTemplatePlaceholders,
  type WikiApplyTemplateCreatedPage,
  type WikiApplyTemplateErrorCode,
  type WikiApplyTemplateRequest,
  type WikiApplyTemplateResolvedSection,
  type WikiApplyTemplateResult,
  type WikiTemplatePlaceholderChild,
  type WikiTemplatePlaceholderSection,
  type WikiTemplateSkeleton,
  parseTemplateBody,
} from './templateTypes'

/**
 * Wiki page template API client (Build #18 / P1.2).
 *
 * Two endpoints, both under
 * `/api/v1/knowledgebase/:kb_id/wiki/pages/:slug/`:
 *
 *   POST .../preview-template   dry-run: validates the skeleton and
 *                               returns the rewritten parent body
 *                               without mutating the DB.
 *   POST .../apply-template     materialises children + rewrites the
 *                               parent body atomically.
 *
 * Both endpoints run under the same OwnedWikiKBOrAdmin +
 * KBAccessWrite guard as page-write endpoints.
 */

function pagePath(kbId: string, slug: string): string {
  return `/api/v1/knowledgebase/${encodeURIComponent(kbId)}/wiki/pages/${slug
    .split('/')
    .map(encodeURIComponent)
    .join('/')}`
}

/**
 * previewWikiTemplate asks the server to validate the skeleton and
 * return the rewritten body + per-kind counts without writing. The
 * dialog uses this to show "what the apply will produce" before the
 * user clicks the final "Apply" button.
 */
export function previewWikiTemplate(
  kbId: string,
  slug: string,
  body: WikiApplyTemplateRequest,
): Promise<WikiApplyTemplateResult> {
  return post<WikiApplyTemplateResult>(
    `${pagePath(kbId, slug)}/preview-template`,
    body,
  )
}

/**
 * applyWikiTemplate materialises the skeleton: deletes prior
 * auto-template children, creates new children, rewrites the parent
 * body. Returns the new page set + rewritten body for the dialog's
 * success summary.
 */
export function applyWikiTemplate(
  kbId: string,
  slug: string,
  body: WikiApplyTemplateRequest,
): Promise<WikiApplyTemplateResult> {
  return post<WikiApplyTemplateResult>(
    `${pagePath(kbId, slug)}/apply-template`,
    body,
  )
}