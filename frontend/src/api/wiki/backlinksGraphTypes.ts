/**
 * Shared types for the wiki page backlinks graph panel — Build #20.
 *
 * Mirrors the Build #11 `backlinksTypes.ts` split so this module can
 * be unit-tested in Node without pulling in `utils/request.ts`. The
 * HTTP payload (`WikiBacklinkGraph`) is the server's snake_case shape;
 * consumers can normalise on demand, but the panel reads these fields
 * directly to avoid an extra transformation layer.
 */

import type { WikiPageBacklink } from './backlinksTypes'

/** Query parameters accepted by `GET /backlinks/graph`. All optional;
 * the server applies the documented defaults / clamps when absent. */
export interface WikiBacklinkGraphRequest {
  max_indirect?: number
  max_related?: number
  jaccard?: number
}

/** A 2-hop backlink row — embeds a `WikiPageBacklink` and adds `via`,
 * the 1-hop slug that introduced the indirection. The panel uses `via`
 * as the click target (D5), not `slug`. */
export interface WikiBacklinkIndirect extends WikiPageBacklink {
  via: string
}

/** A related page scored by Jaccard similarity over `out_links`. */
export interface WikiPageBacklinkRelated extends WikiPageBacklink {
  jaccard: number
}

/** A target slug in the current page's `out_links` that does not
 * resolve to any live page. Read-only — the panel renders these as a
 * non-clickable list with an i18n hint. */
export interface WikiBacklinkBroken {
  target_slug: string
}

/** Per-section counts returned alongside the four arrays. */
export interface WikiBacklinkGraphStats {
  direct_count: number
  indirect_count: number
  related_count: number
  broken_count: number
  out_link_count: number
}

/** Full payload returned by `GET /backlinks/graph`. The four sections
 * are always present (possibly empty arrays) so the panel can iterate
 * without nullish guards. */
export interface WikiBacklinkGraph {
  direct: WikiPageBacklink[]
  indirect: WikiBacklinkIndirect[]
  related: WikiPageBacklinkRelated[]
  broken: WikiBacklinkBroken[]
  stats: WikiBacklinkGraphStats
}