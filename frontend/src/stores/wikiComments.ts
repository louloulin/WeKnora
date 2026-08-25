import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  type WikiComment,
  type WikiCommentCreateRequest,
  type WikiCommentMention,
  type WikiCommentUpdateRequest,
  type WikiMentionCandidate,
  createWikiComment,
  deleteWikiComment,
  listWikiComments,
  searchMentionCandidates,
  updateWikiComment,
} from '../api/wiki/comments'

const MAX_BODY_LEN = 4096

function errMsg(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string' && m) return m
  }
  return fallback
}

export interface WikiCommentState {
  bySlug: Record<string, WikiComment[]>
  loadingSlugs: Record<string, boolean>
  error: string | null
  /** @mention autocomplete state, keyed by `${slug}::${token}`. */
  mention: {
    open: boolean
    loading: boolean
    query: string
    candidates: WikiMentionCandidate[]
  }
}

export const useWikiCommentsStore = defineStore('wikiComments', () => {
  const bySlug = ref<Record<string, WikiComment[]>>({})
  const loadingSlugs = ref<Record<string, boolean>>({})
  const error = ref<string | null>(null)

  const mentionOpen = ref(false)
  const mentionLoading = ref(false)
  const mentionQuery = ref('')
  const mentionCandidates = ref<WikiMentionCandidate[]>([])

  const totalCount = computed(() =>
    Object.values(bySlug.value).reduce((sum, list) => sum + list.length, 0),
  )

  function commentsFor(slug: string): WikiComment[] {
    return bySlug.value[slug] ?? []
  }

  function isLoading(slug: string): boolean {
    return Boolean(loadingSlugs.value[slug])
  }

  async function fetchComments(kbId: string, slug: string): Promise<void> {
    loadingSlugs.value[slug] = true
    error.value = null
    try {
      const res = await listWikiComments(kbId, slug)
      bySlug.value[slug] = res.data?.comments ?? []
    } catch (err) {
      const msg = errMsg(err, 'comments.loadFailed')
      // 404 means the backend route isn't deployed yet — treat as empty.
      if (msg.includes('404') || msg.toLowerCase().includes('not found')) {
        bySlug.value[slug] = []
      } else {
        error.value = msg
      }
    } finally {
      loadingSlugs.value[slug] = false
    }
  }

  async function addComment(
    kbId: string,
    slug: string,
    payload: WikiCommentCreateRequest,
  ): Promise<WikiComment | null> {
    const body = payload.body.trim()
    if (!body) {
      error.value = 'comments.empty'
      return null
    }
    if (body.length > MAX_BODY_LEN) {
      error.value = 'comments.tooLong'
      return null
    }
    try {
      const res = await createWikiComment(kbId, slug, {
        ...payload,
        body,
        mentions: dedupeMentions(payload.mentions ?? []),
      })
      const comment = res.data
      if (comment) {
        const list = bySlug.value[slug] ?? []
        bySlug.value[slug] = [...list, comment]
      }
      return comment ?? null
    } catch (err) {
      error.value = errMsg(err, 'comments.createFailed')
      return null
    }
  }

  async function editComment(
    kbId: string,
    slug: string,
    commentId: string,
    payload: WikiCommentUpdateRequest,
  ): Promise<WikiComment | null> {
    const body = payload.body.trim()
    if (!body) {
      error.value = 'comments.empty'
      return null
    }
    try {
      const res = await updateWikiComment(kbId, slug, commentId, {
        ...payload,
        body,
        mentions: dedupeMentions(payload.mentions ?? []),
      })
      const updated = res.data
      if (updated) {
        const list = bySlug.value[slug] ?? []
        bySlug.value[slug] = list.map((c) => (c.id === commentId ? updated : c))
      }
      return updated ?? null
    } catch (err) {
      error.value = errMsg(err, 'comments.updateFailed')
      return null
    }
  }

  async function removeComment(
    kbId: string,
    slug: string,
    commentId: string,
  ): Promise<boolean> {
    try {
      await deleteWikiComment(kbId, slug, commentId)
      const list = bySlug.value[slug] ?? []
      bySlug.value[slug] = list.filter((c) => c.id !== commentId)
      return true
    } catch (err) {
      error.value = errMsg(err, 'comments.deleteFailed')
      return false
    }
  }

  function clearError(): void {
    error.value = null
  }

  function resetMentions(): void {
    mentionOpen.value = false
    mentionLoading.value = false
    mentionQuery.value = ''
    mentionCandidates.value = []
  }

  async function searchMentions(kbId: string, query: string): Promise<void> {
    const trimmed = query.trim()
    if (!trimmed) {
      mentionCandidates.value = []
      mentionOpen.value = false
      mentionLoading.value = false
      return
    }
    mentionOpen.value = true
    mentionLoading.value = true
    mentionQuery.value = trimmed
    try {
      const res = await searchMentionCandidates(kbId, trimmed, 8)
      const candidates = res.data?.candidates ?? []
      // 404 (backend not yet wired) → empty list, no error toast.
      mentionCandidates.value = candidates
      mentionOpen.value = candidates.length > 0
    } catch {
      mentionCandidates.value = []
      mentionOpen.value = false
    } finally {
      mentionLoading.value = false
    }
  }

  return {
    bySlug,
    loadingSlugs,
    error,
    mentionOpen,
    mentionLoading,
    mentionQuery,
    mentionCandidates,
    totalCount,
    commentsFor,
    isLoading,
    fetchComments,
    addComment,
    editComment,
    removeComment,
    clearError,
    resetMentions,
    searchMentions,
  }
})

function dedupeMentions(mentions: WikiCommentMention[]): WikiCommentMention[] {
  const seen = new Set<string>()
  const result: WikiCommentMention[] = []
  for (const m of mentions) {
    if (!m.userId || seen.has(m.userId)) continue
    seen.add(m.userId)
    result.push(m)
  }
  return result
}