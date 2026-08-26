import { defineStore } from 'pinia'
import { ref } from 'vue'

import {
  getWikiBacklinkGraph,
  getWikiBacklinksCacheStatus,
  getWikiPageBacklinks,
} from '../api/wiki'
import type { WikiBacklinksCacheStatus } from '../api/wiki/backlinksCacheTypes'
import type { WikiBacklinkGraph } from '../api/wiki/backlinksGraphTypes'
import {
  type Backlink,
  backlinkCacheKey,
  normalizeBacklinks,
} from '../api/wiki/backlinksHelpers'

/**
 * Backlinks panel cache (Build #11).
 *
 * Strategy: stale-while-revalidate. The store keeps a `Record`
 * keyed by `${kbId}::${slug}`. `backlinksFor` returns whatever
 * is cached (synchronous, cheap). `loadBacklinks` triggers a
 * network refresh and writes the result back. The panel reads
 * via `backlinksFor` and triggers `loadBacklinks` in `onMounted`
 * / `watch(selectedPage)`. This keeps panel paints under 50ms
 * even on a slow KB while still surfacing fresh data within
 * one frame after the request resolves.
 *
 * `error` is intentionally a per-key flag rather than a single
 * global: a single page failing must not blank out other panels.
 */

export const useWikiBacklinksStore = defineStore('wikiBacklinks', () => {
  const byKey = ref<Record<string, Backlink[]>>({})
  const loadingByKey = ref<Record<string, boolean>>({})
  const errorByKey = ref<Record<string, string | null>>({})

  function key(kbId: string, slug: string): string {
    return backlinkCacheKey(kbId, slug)
  }

  function backlinksFor(kbId: string, slug: string): Backlink[] | undefined {
    return byKey.value[key(kbId, slug)]
  }

  function isLoading(kbId: string, slug: string): boolean {
    return Boolean(loadingByKey.value[key(kbId, slug)])
  }

  function errorFor(kbId: string, slug: string): string | null {
    const e = errorByKey.value[key(kbId, slug)]
    return e ?? null
  }

  function clearError(kbId: string, slug: string): void {
    errorByKey.value[key(kbId, slug)] = null
  }

  async function loadBacklinks(kbId: string, slug: string): Promise<Backlink[]> {
    const k = key(kbId, slug)
    loadingByKey.value[k] = true
    errorByKey.value[k] = null
    try {
      const res = (await getWikiPageBacklinks(kbId, slug)) as {
        data?: unknown
      }
      const list = normalizeBacklinks(res.data)
      byKey.value[k] = list
      return list
    } catch (err) {
      // Silent degrade (D8): keep whatever cache we had so the
      // panel can still render. The error flag is exposed via
      // `errorFor` for components that want to log / show a
      // muted hint, but the panel itself does not toast.
      const msg =
        err instanceof Error ? err.message : 'wiki.backlinks.loadFailed'
      errorByKey.value[k] = msg
      // Return the cached value if present, else [].
      return byKey.value[k] ?? []
    } finally {
      loadingByKey.value[k] = false
    }
  }

  function invalidate(kbId: string, slug: string): void {
    const k = key(kbId, slug)
    delete byKey.value[k]
    delete loadingByKey.value[k]
    delete errorByKey.value[k]
  }

  // Build #20 — four-section graph (direct / indirect / related /
  // broken) plus per-section stats. The cache is keyed the same way
  // as the flat list (`backlinkCacheKey`) so `invalidate` covers both
  // layers. The store exposes a separate `graphByKey` cache and a
  // per-key `loadFailedByKey` so the panel can fall back to the
  // Build #11 flat list when the graph endpoint fails, instead of
  // blanking out the sidebar entirely.
  const graphByKey = ref<Record<string, WikiBacklinkGraph | null>>({})
  const graphLoadingByKey = ref<Record<string, boolean>>({})
  const graphErrorByKey = ref<Record<string, string | null>>({})

  function graphFor(kbId: string, slug: string): WikiBacklinkGraph | null {
    return graphByKey.value[key(kbId, slug)] ?? null
  }

  function isGraphLoading(kbId: string, slug: string): boolean {
    return Boolean(graphLoadingByKey.value[key(kbId, slug)])
  }

  function graphErrorFor(kbId: string, slug: string): string | null {
    return graphErrorByKey.value[key(kbId, slug)] ?? null
  }

  async function loadBacklinkGraph(
    kbId: string,
    slug: string,
  ): Promise<WikiBacklinkGraph | null> {
    const k = key(kbId, slug)
    graphLoadingByKey.value[k] = true
    graphErrorByKey.value[k] = null
    try {
      const res = (await getWikiBacklinkGraph(kbId, slug)) as { data?: unknown }
      const payload = res?.data as WikiBacklinkGraph | undefined
      graphByKey.value[k] = payload ?? null
      return graphByKey.value[k]
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : 'wiki.backlinksGraph.loadFailed'
      graphErrorByKey.value[k] = msg
      // Preserve any previously cached payload so the panel can show
      // a muted "stale" indicator instead of a blank section.
      return graphByKey.value[k] ?? null
    } finally {
      graphLoadingByKey.value[k] = false
    }
  }

  function invalidateGraph(kbId: string, slug: string): void {
    const k = key(kbId, slug)
    delete graphByKey.value[k]
    delete graphLoadingByKey.value[k]
    delete graphErrorByKey.value[k]
  }

  // Build #21 — cache-status (slim "last computed at" payload). Same
  // stale-while-revalidate strategy as the graph layer above: we keep
  // whatever is cached and only flip an error flag when the request
  // fails. A cold cache returns 200 with computed_at=null, so the
  // panel can render the "never computed" hint without re-querying.
  const cacheStatusByKey = ref<Record<string, WikiBacklinksCacheStatus | null>>({})
  const cacheStatusLoadingByKey = ref<Record<string, boolean>>({})
  const cacheStatusErrorByKey = ref<Record<string, string | null>>({})

  function cacheStatusFor(kbId: string, slug: string): WikiBacklinksCacheStatus | null {
    return cacheStatusByKey.value[key(kbId, slug)] ?? null
  }

  function isCacheStatusLoading(kbId: string, slug: string): boolean {
    return Boolean(cacheStatusLoadingByKey.value[key(kbId, slug)])
  }

  function cacheStatusErrorFor(kbId: string, slug: string): string | null {
    return cacheStatusErrorByKey.value[key(kbId, slug)] ?? null
  }

  async function loadBacklinksCacheStatus(
    kbId: string,
    slug: string,
  ): Promise<WikiBacklinksCacheStatus | null> {
    const k = key(kbId, slug)
    cacheStatusLoadingByKey.value[k] = true
    cacheStatusErrorByKey.value[k] = null
    try {
      const res = (await getWikiBacklinksCacheStatus(kbId, slug)) as { data?: unknown }
      const payload = res?.data as WikiBacklinksCacheStatus | undefined
      cacheStatusByKey.value[k] = payload ?? null
      return cacheStatusByKey.value[k]
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : 'wiki.backlinksGraph.cacheStatusLoadFailed'
      cacheStatusErrorByKey.value[k] = msg
      // Preserve whatever payload we had so the footer can still show
      // a stale "last computed" instead of blanking on transient errors.
      return cacheStatusByKey.value[k] ?? null
    } finally {
      cacheStatusLoadingByKey.value[k] = false
    }
  }

  function invalidateCacheStatus(kbId: string, slug: string): void {
    const k = key(kbId, slug)
    delete cacheStatusByKey.value[k]
    delete cacheStatusLoadingByKey.value[k]
    delete cacheStatusErrorByKey.value[k]
  }

  return {
    byKey,
    loadingByKey,
    errorByKey,
    backlinksFor,
    isLoading,
    errorFor,
    clearError,
    loadBacklinks,
    invalidate,
    // Build #20 — graph view
    graphByKey,
    graphLoadingByKey,
    graphErrorByKey,
    graphFor,
    isGraphLoading,
    graphErrorFor,
    loadBacklinkGraph,
    invalidateGraph,
    // Build #21 — cache-status (last computed at footer)
    cacheStatusByKey,
    cacheStatusLoadingByKey,
    cacheStatusErrorByKey,
    cacheStatusFor,
    isCacheStatusLoading,
    cacheStatusErrorFor,
    loadBacklinksCacheStatus,
    invalidateCacheStatus,
  }
})