import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  type WikiShareCreateRequest,
  type WikiShareLink,
  createWikiShareLink,
  listWikiShareLinks,
  revokeWikiShareLink,
} from '../api/wiki/share'

function errMsg(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string' && m) return m
  }
  return fallback
}

export const useWikiShareLinksStore = defineStore('wikiShareLinks', () => {
  /** Keyed by `${kbId}::${slug}` so multiple KBs share the same store. */
  const byPage = ref<Record<string, WikiShareLink[]>>({})
  const loadingPages = ref<Record<string, boolean>>({})
  const error = ref<string | null>(null)

  function pageKey(kbId: string, slug: string): string {
    return `${kbId}::${slug}`
  }

  function linksFor(kbId: string, slug: string): WikiShareLink[] {
    return byPage.value[pageKey(kbId, slug)] ?? []
  }

  const activeLinkCount = computed(() => {
    let total = 0
    for (const list of Object.values(byPage.value)) {
      total += list.filter((s) => !s.revokedAt).length
    }
    return total
  })

  function isLoading(kbId: string, slug: string): boolean {
    return Boolean(loadingPages.value[pageKey(kbId, slug)])
  }

  async function fetchLinks(kbId: string, slug: string): Promise<void> {
    const key = pageKey(kbId, slug)
    loadingPages.value[key] = true
    error.value = null
    try {
      const res = await listWikiShareLinks(kbId, slug)
      byPage.value[key] = res?.shares ?? []
    } catch (err) {
      const msg = errMsg(err, 'share.loadFailed')
      // 404 → backend not deployed yet. Treat as empty list, no error toast.
      if (msg.includes('404') || msg.toLowerCase().includes('not found')) {
        byPage.value[key] = []
      } else {
        error.value = msg
      }
    } finally {
      loadingPages.value[key] = false
    }
  }

  async function createLink(
    kbId: string,
    slug: string,
    payload: WikiShareCreateRequest,
  ): Promise<WikiShareLink | null> {
    try {
      const res = await createWikiShareLink(kbId, slug, payload)
      const link = res
      if (link) {
        const key = pageKey(kbId, slug)
        const list = byPage.value[key] ?? []
        byPage.value[key] = [link, ...list]
      }
      return link ?? null
    } catch (err) {
      error.value = errMsg(err, 'share.createFailed')
      return null
    }
  }

  async function revokeLink(
    kbId: string,
    slug: string,
    shareId: string,
  ): Promise<boolean> {
    try {
      await revokeWikiShareLink(kbId, slug, shareId)
      const key = pageKey(kbId, slug)
      const list = byPage.value[key] ?? []
      // Optimistic — drop the entry. The backend also stamps revokedAt so
      // an in-flight copy is filtered to "revoked" state on next fetch.
      byPage.value[key] = list.filter((s) => s.id !== shareId)
      return true
    } catch (err) {
      error.value = errMsg(err, 'share.revokeFailed')
      return false
    }
  }

  function clearError(): void {
    error.value = null
  }

  return {
    byPage,
    loadingPages,
    error,
    activeLinkCount,
    linksFor,
    isLoading,
    fetchLinks,
    createLink,
    revokeLink,
    clearError,
  }
})