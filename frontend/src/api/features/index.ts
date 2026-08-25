import { get } from '@/utils/request'

/**
 * Runtime feature flags returned by `GET /api/v1/system/features`.
 * Shape mirrors the backend `internal/types/features.go` FeaturesFlags —
 * keep both in sync. New flags should be added as new fields, never by
 * mutating the meaning of existing ones.
 */
export interface FeaturesFlags {
  wiki_wysiwyg: boolean
}

/**
 * Wire envelope: `{ code, msg, data: { flags: FeaturesFlags } }`.
 * The request util flattens non-2xx responses into a thrown Error, so
 * only success reaches `data`.
 */
export interface FeaturesResponse {
  flags: FeaturesFlags
}

/**
 * Fetches runtime feature flags. Fails open: when the call rejects
 * (5xx, network down, 401), the caller is expected to handle the
 * error path and treat every flag as `false`. We deliberately do not
 * swallow the error here so the Pinia store can record it for
 * observability and decide the fail-open policy in one place.
 */
export function getFeatures(): Promise<{ data: FeaturesResponse }> {
  return get('/api/v1/system/features')
}