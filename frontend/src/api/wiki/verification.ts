import { get, post } from '../../utils/request'

/** Mirror of api/wiki/index.ts#encodeSlugPath so this client is drop-in. */
function encodeSlugPath(slug: string): string {
  return slug.split('/').map(encodeURIComponent).join('/')
}

/**
 * Wiki Verified Knowledge Engine client (Build #48).
 *
 * Pairs with the backend Build #48 endpoints added in
 * internal/handler/wiki_verification.go. The frontend treats these
 * as the contract — until the backend lands, list calls fail-open
 * and mutation calls return a typed error so callers can show a
 * "verification unavailable" hint instead of crashing.
 *
 * Endpoints:
 *   GET  /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/verification
 *     → run the AI scanner; response includes manual review fields
 *       (review_owner, verified_at, etc.) populated server-side.
 *   POST /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/verification/verify
 *     → mark the page verified by the caller, advance review_due_at.
 *   POST /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/verification/review-due
 *     → set review_owner + review_due_at in one call.
 */

export interface WikiVerificationReport {
  page_id: string
  slug: string
  knowledge_base_id: string
  tenant_id?: string
  status: 'ok' | 'warning' | 'bad' | 'missing'
  trust_score: number
  checks: WikiVerificationCheck[]
  scanned_at: string
  review_owner?: string
  review_due_at?: string | null
  verified_at?: string | null
  verified_by?: string
  manual_verification_ok?: boolean
}

export interface WikiVerificationCheck {
  code: string
  severity: 'ok' | 'warning' | 'bad'
  message: string
  suggested_action: string
}

export interface MarkVerifiedResponse {
  verified_at: string
  verified_by: string
}

export interface SetReviewScheduleRequest {
  owner_id: string
  due_at: string
  by_user_id?: string
}

export interface SetReviewScheduleResponse {
  review_owner: string
  review_due_at: string
}

/** Get the AI verification + manual review state for a page. */
export async function getVerificationReport(
  kbId: string,
  slug: string,
): Promise<WikiVerificationReport | null> {
  return get<WikiVerificationReport>(
    `/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}/verification`,
  )
}

/** Mark the page as verified by the current caller. */
export async function markPageVerified(
  kbId: string,
  slug: string,
): Promise<MarkVerifiedResponse> {
  return post<MarkVerifiedResponse>(
    `/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}/verification/verify`,
  )
}

/** Set or update the review owner + due date for the page. */
export async function setReviewSchedule(
  kbId: string,
  slug: string,
  req: SetReviewScheduleRequest,
): Promise<SetReviewScheduleResponse> {
  return post<SetReviewScheduleResponse>(
    `/api/v1/knowledgebase/${kbId}/wiki/pages/${encodeSlugPath(slug)}/verification/review-due`,
    req,
  )
}
