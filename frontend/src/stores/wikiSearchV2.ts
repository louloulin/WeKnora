import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  searchWikiPagesV2,
  searchWikiPagesLegacy,
  clampLimit,
} from '../api/wiki/searchV2'
import type { WikiSearchV2Hit } from '../api/wiki/searchV2Types'

/**
 * Build #19 / P2.x.a — wiki search v2 store.
 *
 * Drives the toolbar's WikiSearchBar / WikiSearchResults. Defaults to
 * `?v=2` so the user gets server-rendered `<mark>` snippets and ACL-
 * filtered hits. On transport-level failure (5xx / network) the store
 * falls back to the legacy endpoint silently — the toolbar keeps
 * working during the 6-month dual-track window.
 *
 * State is intentionally separate from `useWikiSearchStore` (Build
 * #9-A) so the legacy and v2 surfaces can be removed independently
 * once Build #19.x retires the legacy endpoint.
 */

const DEFAULT_LIMIT = 20
const DEFAULT_DEBOUNCE_MS = 200

export const useWikiSearchV2Store = defineStore('wikiSearchV2', () => {
  const query = ref('')
  const hits = ref<WikiSearchV2Hit[]>([])
  const total = ref(0)
  const tookMs = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const showResults = ref(false)
  const lastQuery = ref('')
  const lastKBIDs = ref<string[]>([])
  /** True once a fallback to legacy has run; UI can show a soft hint. */
  const usedLegacy = ref(false)
  // Build #19.x — fuzzy + partialMatch toggles. Defaults match the brief:
  // fuzzy=true (English typo is the common case), partialMatch=false
  // (high false-positive rate). The store passes both to the v2 request;
  // they survive across calls until the user toggles them off.
  const fuzzy = ref(true)
  const partialMatch = ref(false)

  const trimmedQuery = computed(() => query.value.trim())
  const hasResults = computed(() => hits.value.length > 0)
  const isEmpty = computed(
    () => !loading.value && !error.value && trimmedQuery.value.length > 0 && !hasResults.value,
  )

  let debounceTimer: ReturnType<typeof setTimeout> | null = null
  let latestToken = 0

  function reset() {
    query.value = ''
    hits.value = []
    total.value = 0
    tookMs.value = 0
    loading.value = false
    error.value = null
    showResults.value = false
    lastQuery.value = ''
    usedLegacy.value = false
    // Reset toggles to brief defaults so a fresh search starts from the
    // user-expected baseline, not whatever was left from a previous run.
    fuzzy.value = true
    partialMatch.value = false
  }

  function clearResults() {
    hits.value = []
    total.value = 0
    tookMs.value = 0
    showResults.value = false
    usedLegacy.value = false
  }

  function openResults() {
    showResults.value = true
  }

  function closeResults() {
    showResults.value = false
  }

  function setQuery(next: string) {
    query.value = next
  }

  // Build #19.x — fuzzy / partialMatch toggles. Setting a toggle does
  // NOT auto-rerun the search; the caller (WikiSearchBarV2) follows up
  // with `scheduleSearch` so the same debounce + token-guard pipeline
  // applies. setQuery is the same — neither auto-reruns.
  function setFuzzy(next: boolean) {
    fuzzy.value = next
  }
  function setPartialMatch(next: boolean) {
    partialMatch.value = next
  }

  /**
   * Schedule a debounced v2 search. Empty / too-short queries short
   * circuit to a clear result. The token guard discards responses
   * from superseded calls.
   */
  function scheduleSearch(kbId: string, kbIds: string[] = [], pageTypes: string[] = []) {
    if (debounceTimer) clearTimeout(debounceTimer)
    if (trimmedQuery.value.length === 0) {
      clearResults()
      loading.value = false
      error.value = null
      return
    }
    loading.value = true
    error.value = null
    const myToken = ++latestToken
    debounceTimer = setTimeout(() => {
      debounceTimer = null
      void runSearch(kbId, kbIds, pageTypes, myToken)
    }, DEFAULT_DEBOUNCE_MS)
  }

  async function runSearch(kbId: string, kbIds: string[], pageTypes: string[], token: number) {
    if (token !== latestToken) return
    const limit = clampLimit(DEFAULT_LIMIT)
    try {
      const res = await searchWikiPagesV2(kbId, {
        q: trimmedQuery.value,
        kb_ids: kbIds,
        page_types: pageTypes,
        limit,
        fuzzy: fuzzy.value,
        partial_match: partialMatch.value,
      })
      if (token !== latestToken) return
      hits.value = res.hits
      total.value = res.total
      tookMs.value = res.took_ms
      loading.value = false
      error.value = null
      showResults.value = true
      lastQuery.value = trimmedQuery.value
      lastKBIDs.value = res.kb_ids
      usedLegacy.value = false
    } catch (err) {
      // Silent fallback to legacy — the legacy payload shape is
      // different, so we surface a soft "fallback" state rather than
      // mixing payload shapes.
      try {
        await searchWikiPagesLegacy(kbId, trimmedQuery.value, limit)
        if (token !== latestToken) return
        usedLegacy.value = true
        hits.value = []
        total.value = 0
        tookMs.value = 0
        loading.value = false
        error.value = null
        showResults.value = true
        lastQuery.value = trimmedQuery.value
      } catch (legacyErr) {
        if (token !== latestToken) return
        const msg = legacyErr && typeof legacyErr === 'object' && 'message' in legacyErr
          ? String((legacyErr as { message?: unknown }).message)
          : ''
        error.value = msg || 'wiki.searchV2.error'
        loading.value = false
        hits.value = []
        total.value = 0
      }
    }
  }

  return {
    // state
    query,
    hits,
    total,
    tookMs,
    loading,
    error,
    showResults,
    lastQuery,
    lastKBIDs,
    usedLegacy,
    fuzzy,
    partialMatch,
    // getters
    trimmedQuery,
    hasResults,
    isEmpty,
    // actions
    setQuery,
    setFuzzy,
    setPartialMatch,
    scheduleSearch,
    reset,
    clearResults,
    openResults,
    closeResults,
  }
})