/**
 * Build #21 — types for the wiki backlinks graph cache status endpoint.
 *
 * Mirrors the Build #20 / Build #11 split so this module can be
 * unit-tested in Node without pulling in `utils/request.ts`. The
 * server's `WikiBacklinksCacheStatus` payload is intentionally slim —
 * only the timestamps + source_event_id — so the panel footer can show
 * "last computed at" without paying the full graph cost.
 */

/** Slim metadata returned by `GET /backlinks/cache-status`. Fields are
 * nullable because a cold cache row returns 200 with `{ "slug": "...",
 * "computed_at": null, "updated_at": null }` (per the handler contract)
 * so the panel can render the cold state instead of 404. */
export interface WikiBacklinksCacheStatus {
  slug: string
  /** ISO-8601 RFC3339. Null when the cache is cold. */
  computed_at: string | null
  /** ISO-8601 RFC3339. Null when the cache is cold. */
  updated_at: string | null
  /** Event id that produced the cached snapshot (Build #21 source of
   * truth traceability). Empty string for the cold-read writeback. */
  source_event_id?: string
}