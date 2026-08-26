import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  type WikiBatchRouteResult,
  type WikiTag,
  type WikiTagColor,
  type WikiTagCreateRequest,
  type WikiTagUpdateRequest,
  type WikiTagWithCount,
  batchTagWikiPages,
  createWikiTag,
  deleteWikiTag,
  getWikiPageTags,
  getWikiTag,
  listWikiTags,
  setWikiPageTags,
  updateWikiTag,
} from '../api/wiki/tags'

/**
 * Wiki tags Pinia store (Build #17 / P1.1).
 *
 * Owns three concerns:
 *   1. The KB-scoped tag dictionary (`tags` map + `byId` lookup).
 *   2. Per-page tag associations (`pageTags` map keyed by slug).
 *   3. Async/sync dispatch helpers for the bulk-tag flow.
 *
 * All mutations go through the API client — the store never mutates
 * `tags` / `pageTags` optimistically. Optimistic updates would let
 * the cache drift out of sync with the DB UNIQUE constraint, which
 * is the one rule the backend enforces strictly.
 */

// WikiTagsStore is exported as a `use*` Pinia hook.
export const useWikiTagsStore = defineStore('wikiTags', () => {
  // KB-scoped tag dictionary. Keyed by tagID so the panel and the
  // picker can resolve a tag from either a list or an ID without
  // scanning the array.
  const tags = ref<WikiTagWithCount[]>([])
  const byId = ref<Record<string, WikiTagWithCount>>({})
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  // Per-page tag associations. Keyed by `slug` so multiple pages
  // can be open without colliding.
  const pageTags = ref<Record<string, WikiTag[]>>({})
  const loadingPages = ref<Record<string, boolean>>({})
  const savingPages = ref<Record<string, boolean>>({})

  function indexById(list: WikiTagWithCount[]): Record<string, WikiTagWithCount> {
    const out: Record<string, WikiTagWithCount> = {}
    for (const t of list) {
      out[t.id] = t
    }
    return out
  }

  async function fetchTags(kbId: string): Promise<void> {
    loading.value = true
    error.value = null
    try {
      // The backend wraps the body as `{ tags: WikiTagWithCount[] }`.
      const res = (await listWikiTags(kbId)) as { tags?: WikiTagWithCount[] }
      const list = res.tags ?? []
      tags.value = list
      byId.value = indexById(list)
    } catch (e) {
      error.value = (e as Error).message ?? 'unknown'
      tags.value = []
      byId.value = {}
    } finally {
      loading.value = false
    }
  }

  async function fetchTag(kbId: string, tagId: string): Promise<WikiTag | null> {
    try {
      return await getWikiTag(kbId, tagId)
    } catch (e) {
      error.value = (e as Error).message ?? 'unknown'
      return null
    }
  }

  async function createTag(
    kbId: string,
    body: WikiTagCreateRequest,
  ): Promise<WikiTag | null> {
    saving.value = true
    error.value = null
    try {
      const tag = await createWikiTag(kbId, body)
      // Optimistically refresh the local list so the panel sees the
      // new tag immediately. Failure here does not roll back the
      // server write — the server is the source of truth and a
      // stale list is recoverable on the next fetchTags().
      try {
        await fetchTags(kbId)
      } catch {
        // ignore
      }
      return tag
    } catch (e) {
      error.value = (e as Error).message ?? 'unknown'
      return null
    } finally {
      saving.value = false
    }
  }

  async function updateTag(
    kbId: string,
    tagId: string,
    patch: WikiTagUpdateRequest,
  ): Promise<WikiTag | null> {
    saving.value = true
    error.value = null
    try {
      const updated = await updateWikiTag(kbId, tagId, patch)
      // Same pattern as createTag — refresh the local list so the
      // panel reflects the change without forcing the caller to do it.
      try {
        await fetchTags(kbId)
      } catch {
        // ignore
      }
      return updated
    } catch (e) {
      error.value = (e as Error).message ?? 'unknown'
      return null
    } finally {
      saving.value = false
    }
  }

  async function deleteTag(kbId: string, tagId: string): Promise<boolean> {
    saving.value = true
    error.value = null
    try {
      await deleteWikiTag(kbId, tagId)
      // Local prune so the chip disappears before the next round-trip.
      tags.value = tags.value.filter((t) => t.id !== tagId)
      const next = { ...byId.value }
      delete next[tagId]
      byId.value = next
      return true
    } catch (e) {
      error.value = (e as Error).message ?? 'unknown'
      return false
    } finally {
      saving.value = false
    }
  }

  async function fetchPageTags(
    kbId: string,
    slug: string,
  ): Promise<WikiTag[]> {
    loadingPages.value[slug] = true
    error.value = null
    try {
      const res = (await getWikiPageTags(kbId, slug)) as { tags?: WikiTag[] }
      const list = res.tags ?? []
      pageTags.value[slug] = list
      return list
    } catch (e) {
      error.value = (e as Error).message ?? 'unknown'
      pageTags.value[slug] = []
      return []
    } finally {
      loadingPages.value[slug] = false
    }
  }

  async function setPageTags(
    kbId: string,
    slug: string,
    tagIds: string[],
  ): Promise<WikiTag[]> {
    savingPages.value[slug] = true
    error.value = null
    try {
      const res = (await setWikiPageTags(kbId, slug, tagIds)) as { tags?: WikiTag[] }
      const list = res.tags ?? []
      pageTags.value[slug] = list
      return list
    } catch (e) {
      error.value = (e as Error).message ?? 'unknown'
      throw e
    } finally {
      savingPages.value[slug] = false
    }
  }

  async function batchTagPages(
    kbId: string,
    slugs: string[],
    tagId: string,
    op: 'add' | 'remove',
  ): Promise<WikiBatchRouteResult | null> {
    saving.value = true
    error.value = null
    try {
      const route = await batchTagWikiPages(kbId, {
        slugs,
        tag_id: tagId,
        op,
      })
      return route
    } catch (e) {
      error.value = (e as Error).message ?? 'unknown'
      return null
    } finally {
      saving.value = false
    }
  }

  // Helpers consumed by the panel / picker without round-tripping.

  function isLoadingPage(slug: string): boolean {
    return Boolean(loadingPages.value[slug])
  }

  function isSavingPage(slug: string): boolean {
    return Boolean(savingPages.value[slug])
  }

  function tagsFor(slug: string): WikiTag[] {
    return pageTags.value[slug] ?? []
  }

  function tagById(tagId: string): WikiTagWithCount | undefined {
    return byId.value[tagId]
  }

  function reset(): void {
    tags.value = []
    byId.value = {}
    pageTags.value = {}
    error.value = null
  }

  return {
    tags,
    byId,
    loading,
    saving,
    error,
    pageTags,
    fetchTags,
    fetchTag,
    createTag,
    updateTag,
    deleteTag,
    fetchPageTags,
    setPageTags,
    batchTagPages,
    isLoadingPage,
    isSavingPage,
    tagsFor,
    tagById,
    reset,
  }
})

// WikiBatchTagOp re-export so consumers can import from the store
// barrel without reaching into the API module.
export type { WikiBatchRouteResult, WikiTag, WikiTagColor, WikiTagWithCount }