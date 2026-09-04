import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  listNotifications,
  getUnreadCount,
  markRead,
  markDismissed,
  markAllRead,
  type Notification,
  type NotificationKind,
  type NotificationStatus,
} from '../api/notifications'

const POLL_INTERVAL_MS = 30_000
const DEFAULT_PAGE_SIZE = 20

/**
 * Notification center Pinia store (Build #P0.4).
 *
 * Responsibilities:
 *   - Track the bell dropdown's open / closed state.
 *   - Poll the unread-count endpoint every 30s while a session is
 *     active so the badge stays fresh without a page reload.
 *   - Lazy-load the page slice when the dropdown opens; cache the
 *     last page in memory until the user logs out.
 *   - Drive the four mutations the UI exposes (read, dismiss,
 *     read-all). All mutations are optimistic on the local copy so
 *     the bell responds instantly; failures roll back.
 */
export const useNotificationStore = defineStore('notifications', () => {
  const items = ref<Notification[]>([])
  const total = ref(0)
  const unread = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const open = ref(false)
  const page = ref(1)
  const pageSize = ref(DEFAULT_PAGE_SIZE)
  const statusFilter = ref<NotificationStatus | null>(null)
  const kindFilter = ref<NotificationKind | null>(null)

  let pollTimer: ReturnType<typeof setInterval> | null = null
  let latestToken = 0

  const hasMore = computed(() => items.value.length < total.value)
  const isEmpty = computed(
    () => !loading.value && !error.value && items.value.length === 0,
  )

  function startPolling() {
    if (pollTimer) return
    // v0.7.197 — Bell mounts early (before user login completes on first
    // paint). Wait for a token to land in localStorage before the first
    // poll, otherwise the request goes out with no Authorization header
    // and the badge stays stuck on zero. The setInterval below re-checks
    // every POLL_INTERVAL_MS so a late login is still picked up within
    // 30s, but a 1.5s warm-up covers the typical login round-trip.
    const warmup = () => {
      if (typeof window !== 'undefined' && !window.localStorage.getItem('weknora_token')) {
        window.setTimeout(warmup, 500)
        return
      }
      void refreshUnread()
    }
    window.setTimeout(warmup, 1500)
    pollTimer = setInterval(() => {
      void refreshUnread()
    }, POLL_INTERVAL_MS)
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  async function refreshUnread() {
    try {
      const res = await getUnreadCount()
      unread.value = res.count
    } catch (err) {
      // Silent: the bell just shows the stale count until the next
      // successful poll. Logging here would flood the console.
      error.value = err instanceof Error ? err.message : 'poll failed'
    }
  }

  async function fetchPage(opts: { reset?: boolean } = {}) {
    const myToken = ++latestToken
    loading.value = true
    error.value = null
    try {
      const targetPage = opts.reset ? 1 : page.value
      const res = await listNotifications({
        page: targetPage,
        page_size: pageSize.value,
        status: statusFilter.value ?? undefined,
        kind: kindFilter.value ?? undefined,
      })
      if (myToken !== latestToken) return
      items.value = res.items
      total.value = res.total
      page.value = targetPage
    } catch (err) {
      if (myToken !== latestToken) return
      error.value = err instanceof Error ? err.message : 'fetch failed'
    } finally {
      if (myToken === latestToken) loading.value = false
    }
  }

  async function loadMore() {
    if (!hasMore || loading.value) return
    loading.value = true
    error.value = null
    const nextPage = page.value + 1
    try {
      const res = await listNotifications({
        page: nextPage,
        page_size: pageSize.value,
        status: statusFilter.value ?? undefined,
        kind: kindFilter.value ?? undefined,
      })
      items.value = items.value.concat(res.items)
      page.value = nextPage
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'fetch failed'
    } finally {
      loading.value = false
    }
  }

  async function readOne(id: number) {
    const idx = items.value.findIndex((n) => n.id === id)
    if (idx < 0) return
    const prev = items.value[idx]
    if (prev.status === 'read') return
    items.value[idx] = { ...prev, status: 'read', read_at: new Date().toISOString() }
    if (unread.value > 0) unread.value -= 1
    try {
      await markRead(id)
    } catch {
      items.value[idx] = prev
      if (unread.value >= 0) unread.value += 1
    }
  }

  async function dismissOne(id: number) {
    const idx = items.value.findIndex((n) => n.id === id)
    if (idx < 0) return
    const prev = items.value[idx]
    const wasUnread = prev.status === 'unread'
    items.value[idx] = {
      ...prev,
      status: 'dismissed',
      dismissed_at: new Date().toISOString(),
    }
    if (wasUnread && unread.value > 0) unread.value -= 1
    try {
      await markDismissed(id)
    } catch {
      items.value[idx] = prev
      if (wasUnread) unread.value += 1
    }
  }

  async function readAll() {
    const before = items.value.slice()
    items.value = items.value.map((n) =>
      n.status === 'unread'
        ? { ...n, status: 'read', read_at: new Date().toISOString() }
        : n,
    )
    const dropped = unread.value
    unread.value = 0
    try {
      await markAllRead()
    } catch {
      items.value = before
      unread.value = dropped
    }
  }

  function setOpen(value: boolean) {
    open.value = value
    if (value && items.value.length === 0) {
      void fetchPage({ reset: true })
    }
  }

  function setStatusFilter(value: NotificationStatus | null) {
    statusFilter.value = value
    void fetchPage({ reset: true })
  }

  function setKindFilter(value: NotificationKind | null) {
    kindFilter.value = value
    void fetchPage({ reset: true })
  }

  function reset() {
    items.value = []
    total.value = 0
    unread.value = 0
    loading.value = false
    error.value = null
    open.value = false
    page.value = 1
    statusFilter.value = null
    kindFilter.value = null
    stopPolling()
  }

  return {
    items,
    total,
    unread,
    loading,
    error,
    open,
    page,
    pageSize,
    statusFilter,
    kindFilter,
    hasMore,
    isEmpty,
    startPolling,
    stopPolling,
    refreshUnread,
    fetchPage,
    loadMore,
    readOne,
    dismissOne,
    readAll,
    setOpen,
    setStatusFilter,
    setKindFilter,
    reset,
  }
})
