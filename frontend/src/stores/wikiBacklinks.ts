import { defineStore } from 'pinia'
import { ref } from 'vue'

import { getWikiPageBacklinks } from '../api/wiki'
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
  }
})