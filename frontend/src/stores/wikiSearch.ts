import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  searchWikiPagesFullText,
  type WikiSearchResponse,
  type WikiSearchResult,
} from '../api/wiki/search'
import {
  MIN_QUERY_LENGTH,
  pushHistory as pushHistoryPure,
} from '../utils/wikiSearchHistory'

const HISTORY_MAX = 10
const DEFAULT_LIMIT = 50
const DEFAULT_DEBOUNCE_MS = 200

/**
 * Wiki toolbar full-text search store (Build #9-A).
 *
 * Responsibilities:
 *   - Track the raw query and current results.
 *   - Debounce real fetches so we do not hammer the API on every
 *     keystroke.
 *   - Maintain an in-memory history of the last 10 unique queries.
 *   - Expose flags the popup component reads to render the four
 *     states (loading / error / empty / list).
 *
 * State is intentionally separate from `searchQuery` in
 * `WikiBrowser.vue` — the legacy search in the sidebar continues to
 * work unchanged; the new toolbar search has its own UX flow.
 */
export const useWikiSearchStore = defineStore('wikiSearch', () => {
  const query = ref('')
  const results = ref<WikiSearchResult[]>([])
  const total = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const showResults = ref(false)
  const history = ref<string[]>([])

  const trimmedQuery = computed(() => query.value.trim())
  const isQueryLongEnough = computed(() => trimmedQuery.value.length >= MIN_QUERY_LENGTH)
  const hasResults = computed(() => results.value.length > 0)
  const isEmpty = computed(() => !loading.value && !error.value && isQueryLongEnough.value && !hasResults.value)

  let debounceTimer: ReturnType<typeof setTimeout> | null = null
  let currentKbId: string | null = null
  let latestToken = 0

  /**
   * Reset transient state. Does NOT clear history (history is meant
   * to survive query changes within a session).
   */
  function reset() {
    query.value = ''
    results.value = []
    total.value = 0
    loading.value = false
    error.value = null
    showResults.value = false
  }

  function clearResults() {
    results.value = []
    total.value = 0
    showResults.value = false
  }

  function openResults() {
    showResults.value = true
  }

  function closeResults() {
    showResults.value = false
  }

  function pushHistory(q: string) {
    history.value = pushHistoryPure(history.value, q)
  }

  /**
   * Schedule a debounced search. Multiple calls within
   * `DEFAULT_DEBOUNCE_MS` collapse into the last one. The token
   * guard discards responses from superseded calls so the UI never
   * shows a stale result for a query the user has already typed
   * past.
   */
  function scheduleSearch(kbId: string, ms: number = DEFAULT_DEBOUNCE_MS) {
    if (debounceTimer) clearTimeout(debounceTimer)
    if (!isQueryLongEnough.value) {
      clearResults()
      loading.value = false
      error.value = null
      return
    }
    currentKbId = kbId
    loading.value = true
    error.value = null
    const myToken = ++latestToken
    debounceTimer = setTimeout(() => {
      debounceTimer = null
      void runSearch(kbId, myToken)
    }, ms)
  }

  async function runSearch(kbId: string, token: number) {
    if (token !== latestToken) return
    try {
      const res: WikiSearchResponse = await searchWikiPagesFullText(kbId, trimmedQuery.value, DEFAULT_LIMIT)
      if (token !== latestToken) return
      results.value = res.results
      total.value = res.total
      loading.value = false
      error.value = null
      showResults.value = true
      pushHistory(trimmedQuery.value)
    } catch (err) {
      if (token !== latestToken) return
      const msg = err && typeof err === 'object' && 'message' in err
        ? String((err as { message?: unknown }).message)
        : ''
      error.value = msg || 'wiki.search.error'
      loading.value = false
      results.value = []
      total.value = 0
    }
  }

  function setQuery(next: string) {
    query.value = next
  }

  function setKbId(kbId: string) {
    currentKbId = kbId
  }

  return {
    // state
    query,
    results,
    total,
    loading,
    error,
    showResults,
    history,
    // getters
    trimmedQuery,
    isQueryLongEnough,
    hasResults,
    isEmpty,
    // actions
    setQuery,
    setKbId,
    scheduleSearch,
    reset,
    clearResults,
    openResults,
    closeResults,
    pushHistory,
  }
})