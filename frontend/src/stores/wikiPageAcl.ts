import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  type WikiAclSaveRequest,
  type WikiPageAcl,
  type WikiUserCandidate,
  defaultWikiPageAcl,
  getWikiPageAcl,
  putWikiPageAcl,
  searchWikiAclCandidates,
} from '../api/wiki/acl'

function errMsg(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string' && m) return m
  }
  return fallback
}

export const useWikiPageAclStore = defineStore('wikiPageAcl', () => {
  /** Keyed by `${kbId}::${slug}`. */
  const byPage = ref<Record<string, WikiPageAcl>>({})
  const loadingPages = ref<Record<string, boolean>>({})
  const savingPages = ref<Record<string, boolean>>({})
  const error = ref<string | null>(null)

  // @mention autocomplete analogue — re-used by the ACL allow-list picker.
  const candidateOpen = ref(false)
  const candidateLoading = ref(false)
  const candidateQuery = ref('')
  const candidateList = ref<WikiUserCandidate[]>([])

  function pageKey(kbId: string, slug: string): string {
    return `${kbId}::${slug}`
  }

  function aclFor(kbId: string, slug: string): WikiPageAcl {
    return byPage.value[pageKey(kbId, slug)] ?? defaultWikiPageAcl()
  }

  function isRestricted(kbId: string, slug: string): boolean {
    const acl = aclFor(kbId, slug)
    return acl.mode !== 'inherit'
  }

  function isLoading(kbId: string, slug: string): boolean {
    return Boolean(loadingPages.value[pageKey(kbId, slug)])
  }

  function isSaving(kbId: string, slug: string): boolean {
    return Boolean(savingPages.value[pageKey(kbId, slug)])
  }

  async function fetchAcl(kbId: string, slug: string): Promise<void> {
    const key = pageKey(kbId, slug)
    loadingPages.value[key] = true
    error.value = null
    try {
      const res = await getWikiPageAcl(kbId, slug)
      byPage.value[key] = normalizeAcl(res.data)
    } catch (err) {
      const msg = errMsg(err, 'acl.loadFailed')
      // 404 / missing column → default (inherit). No error toast.
      if (msg.includes('404') || msg.toLowerCase().includes('not found')) {
        byPage.value[key] = defaultWikiPageAcl()
      } else {
        error.value = msg
      }
    } finally {
      loadingPages.value[key] = false
    }
  }

  async function saveAcl(
    kbId: string,
    slug: string,
    payload: WikiAclSaveRequest,
  ): Promise<WikiPageAcl | null> {
    const key = pageKey(kbId, slug)
    savingPages.value[key] = true
    error.value = null
    try {
      const res = await putWikiPageAcl(kbId, slug, payload)
      const next = normalizeAcl(res.data)
      byPage.value[key] = next
      return next
    } catch (err) {
      error.value = errMsg(err, 'acl.saveFailed')
      return null
    } finally {
      savingPages.value[key] = false
    }
  }

  function clearError(): void {
    error.value = null
  }

  function resetCandidates(): void {
    candidateOpen.value = false
    candidateLoading.value = false
    candidateQuery.value = ''
    candidateList.value = []
  }

  async function searchCandidates(query: string): Promise<void> {
    const trimmed = query.trim()
    if (!trimmed) {
      candidateList.value = []
      candidateOpen.value = false
      candidateLoading.value = false
      return
    }
    candidateOpen.value = true
    candidateLoading.value = true
    candidateQuery.value = trimmed
    try {
      const res = await searchWikiAclCandidates(trimmed, 10)
      const list = res.data?.candidates ?? []
      candidateList.value = list
      candidateOpen.value = list.length > 0
    } catch {
      candidateList.value = []
      candidateOpen.value = false
    } finally {
      candidateLoading.value = false
    }
  }

  return {
    byPage,
    loadingPages,
    savingPages,
    error,
    candidateOpen,
    candidateLoading,
    candidateQuery,
    candidateList,
    aclFor,
    isRestricted,
    isLoading,
    isSaving,
    fetchAcl,
    saveAcl,
    clearError,
    resetCandidates,
    searchCandidates,
  }
})

function normalizeAcl(input: unknown): WikiPageAcl {
  const fallback = defaultWikiPageAcl()
  if (!input || typeof input !== 'object') return fallback
  const src = input as Record<string, unknown>
  const modeRaw = src.mode
  const mode: WikiPageAcl['mode'] =
    modeRaw === 'private' || modeRaw === 'allow_list' ? modeRaw : 'inherit'
  const allowUserIds = Array.isArray(src.allowUserIds)
    ? src.allowUserIds.filter((x): x is string => typeof x === 'string')
    : []
  const allowGroupIds = Array.isArray(src.allowGroupIds)
    ? src.allowGroupIds.filter((x): x is string => typeof x === 'string')
    : []
  const denyInherited = Boolean(src.denyInherited)
  const revision = typeof src.revision === 'number' ? src.revision : undefined
  const updatedAt = typeof src.updatedAt === 'string' ? src.updatedAt : undefined
  return {
    mode,
    allowUserIds,
    allowGroupIds,
    denyInherited,
    revision,
    updatedAt,
  }
}