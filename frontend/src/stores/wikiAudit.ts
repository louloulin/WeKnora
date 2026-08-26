import { defineStore } from 'pinia'
import { ref } from 'vue'

import {
  listWikiAuditEvents,
} from '../api/wiki'
import type {
  WikiAuditEvent,
  WikiAuditEventListResponse,
  WikiAuditFilter,
  WikiAuditSource,
} from '../api/wiki/auditTypes'

/**
 * Build #24 — unified wiki audit Pinia store.
 *
 * The drawer reads `eventsFor(kbId)` and triggers `loadAudit` on
 * `onMounted` / on filter change. The store caches the last response
 * per KB so opening the drawer twice is a no-op until the user
 * re-runs the query. Source counts are stored separately so the
 * toolbar badge can render them without subscribing to the events
 * array.
 *
 * The store is deliberately permissive about partial failures:
 * `errorFor(kbId)` is per-key so a single failed refresh never
 * blanks out the toolbar badge.
 */

export interface WikiAuditCacheEntry {
  events: WikiAuditEvent[]
  total: number
  page: number
  page_size: number
  source_counts: Record<WikiAuditSource, number>
  loaded_at: number
}

const initialSourceCounts = (): Record<WikiAuditSource, number> => ({
  audit_logs: 0,
  wiki_batch_job_audit: 0,
  wiki_backlinks_cache_invalidation_log: 0,
  wiki_page_acl_audit: 0,
})

export const useWikiAuditStore = defineStore('wikiAudit', () => {
  const byKey = ref<Record<string, WikiAuditCacheEntry>>({})
  const loadingByKey = ref<Record<string, boolean>>({})
  const errorByKey = ref<Record<string, string | null>>({})

  function key(kbId: string): string {
    return kbId
  }

  function entryFor(kbId: string): WikiAuditCacheEntry | undefined {
    return byKey.value[key(kbId)]
  }

  function eventsFor(kbId: string): WikiAuditEvent[] {
    return entryFor(kbId)?.events ?? []
  }

  function sourceCountsFor(kbId: string): Record<WikiAuditSource, number> {
    return entryFor(kbId)?.source_counts ?? initialSourceCounts()
  }

  function totalFor(kbId: string): number {
    return entryFor(kbId)?.total ?? 0
  }

  function isLoading(kbId: string): boolean {
    return Boolean(loadingByKey.value[key(kbId)])
  }

  function errorFor(kbId: string): string | null {
    return errorByKey.value[key(kbId)] ?? null
  }

  async function loadAudit(
    kbId: string,
    filter: WikiAuditFilter = {},
  ): Promise<WikiAuditEventListResponse | null> {
    if (!kbId) return null
    const k = key(kbId)
    loadingByKey.value[k] = true
    errorByKey.value[k] = null
    try {
      const resp = await listWikiAuditEvents(kbId, filter)
      byKey.value[k] = {
        events: resp.events ?? [],
        total: resp.total ?? 0,
        page: resp.page ?? 1,
        page_size: resp.page_size ?? 50,
        source_counts: resp.source_counts ?? initialSourceCounts(),
        loaded_at: Date.now(),
      }
      return resp
    } catch (err) {
      errorByKey.value[k] =
        err instanceof Error ? err.message : 'failed to load audit events'
      return null
    } finally {
      loadingByKey.value[k] = false
    }
  }

  function reset(kbId: string): void {
    const k = key(kbId)
    delete byKey.value[k]
    delete loadingByKey.value[k]
    delete errorByKey.value[k]
  }

  return {
    byKey,
    loadingByKey,
    errorByKey,
    entryFor,
    eventsFor,
    sourceCountsFor,
    totalFor,
    isLoading,
    errorFor,
    loadAudit,
    reset,
  }
})