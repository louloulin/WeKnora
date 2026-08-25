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
import {
  decodeAclError,
  emitAclEvent,
  errMsg,
  errStatus,
  normalizeAcl,
  onAclEvent,
  type AclEvent,
} from './wikiPageAclConflict'

export { normalizeAcl, onAclEvent, emitAclEvent } from './wikiPageAclConflict'

// Discriminated result for saveAcl so callers can branch on
// `conflict: true` (server has a newer revision → toast + refetch)
// vs. genuine failures (network / denied / unknown).
// `conflict` carries the canonical ACL the server now holds.
export type SaveAclResult =
  | { ok: true; acl: WikiPageAcl }
  | { ok: false; conflict: true; current: WikiPageAcl }
  | { ok: false; conflict: false; error: string }

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
      // The backend wraps the body as `{ data: WikiPageAcl }`. The
      // typed `get<WikiPageAcl>` signature lies about this — the
      // runtime axios interceptor unwraps the response but keeps
      // the inner `{ data }` envelope. A pre-existing pattern across
      // the wiki stores; tracked separately as a typing cleanup.
      const res = (await getWikiPageAcl(kbId, slug)) as { data?: unknown }
      const next = normalizeAcl(res.data)
      byPage.value[key] = next
      emitAclEvent({ kind: 'updated', kbId, slug, acl: next })
    } catch (err) {
      // Backend now exists and returns 200 for both "no ACL" and a
      // configured ACL. A 404 here means the route is genuinely gone
      // (old backend / route not deployed) — still fall back to
      // inherit so the UI stays usable, but record nothing in `error`
      // so we don't spam toasts. Real errors (500 / network) set
      // `error` and leave `byPage` untouched.
      const status = errStatus(err)
      if (status === 404) {
        const fallback = defaultWikiPageAcl()
        byPage.value[key] = fallback
        emitAclEvent({ kind: 'updated', kbId, slug, acl: fallback })
      } else {
        error.value = errMsg(err, 'acl.loadFailed')
      }
    } finally {
      loadingPages.value[key] = false
    }
  }

  async function saveAcl(
    kbId: string,
    slug: string,
    payload: WikiAclSaveRequest,
  ): Promise<SaveAclResult> {
    const key = pageKey(kbId, slug)
    savingPages.value[key] = true
    error.value = null
    try {
      const res = (await putWikiPageAcl(kbId, slug, payload)) as { data?: unknown }
      const next = normalizeAcl(res.data)
      byPage.value[key] = next
      emitAclEvent({ kind: 'updated', kbId, slug, acl: next })
      return { ok: true, acl: next }
    } catch (err) {
      const decoded = decodeAclError(err, 'acl.saveFailed')
      if (decoded.kind === 'conflict') {
        // Optimistic-lock conflict — backend returns the canonical ACL
        // under `currentAcl` (or `current_acl` / `data` depending on
        // shape; `decodeAclError` tolerates both). Adopt the server's
        // view, surface the conflict event, and let the dialog close +
        // reset.
        byPage.value[key] = decoded.current
        emitAclEvent({ kind: 'conflict', kbId, slug, current: decoded.current })
        return { ok: false, conflict: true, current: decoded.current }
      }
      error.value = decoded.message
      return { ok: false, conflict: false, error: decoded.message }
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
      const res = (await searchWikiAclCandidates(trimmed, 10)) as { data?: { candidates?: WikiUserCandidate[] } }
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

// `normalizeAcl` is exported via `wikiPageAclConflict.ts` re-export above.
