<script setup lang="ts">
/**
 * Notification bell dropdown (Build #P0.4).
 *
 * Mounts in the top nav. The store polls /notifications/unread-count
 * every 30s; the dropdown lazy-loads /notifications when opened.
 * Mutations (read / dismiss / read-all) are optimistic and roll back
 * on failure. ESC and outside-click close the dropdown per the
 * project's standard pattern.
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useNotificationStore } from '../stores/notifications'
import type { Notification } from '../api/notifications'

const store = useNotificationStore()
const { items, unread, loading, error, open, statusFilter, hasMore, isEmpty } =
  storeToRefs(store)
const { t } = useI18n()

const wrapperRef = ref<HTMLElement | null>(null)
const popoverRef = ref<HTMLElement | null>(null)

const visibleUnread = computed(() => unread.value)
const dropdownLabel = computed(() =>
  visibleUnread.value > 99 ? '99+' : String(visibleUnread.value),
)

function toggle() {
  store.setOpen(!open.value)
}

function close() {
  store.setOpen(false)
}

async function handleRead(id: number) {
  await store.readOne(id)
}

async function handleDismiss(id: number) {
  await store.dismissOne(id)
}

async function handleReadAll() {
  await store.readAll()
}

async function handleLoadMore() {
  await store.loadMore()
}

function kindChip(kind: Notification['kind']): string {
  switch (kind) {
    case 'wiki.comment.created':
    case 'wiki.comment.reply':
      return 'comment'
    case 'wiki.mentioned':
      return 'mention'
    case 'agent.shared':
      return 'agent'
    case 'kb.shared':
      return 'kb'
    case 'system.alert':
      return 'system'
    default:
      return 'other'
  }
}

function timeAgo(iso: string): string {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const now = Date.now()
  const sec = Math.max(1, Math.round((now - then) / 1000))
  if (sec < 60) return `${sec}s`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}m`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}h`
  const day = Math.round(hr / 24)
  if (day < 30) return `${day}d`
  const month = Math.round(day / 30)
  return `${month}mo`
}

function onDocumentClick(e: MouseEvent) {
  if (!open.value) return
  const target = e.target as Node | null
  if (!target) return
  if (wrapperRef.value?.contains(target)) return
  if (popoverRef.value?.contains(target)) return
  close()
}

function onKey(e: KeyboardEvent) {
  if (!open.value) return
  if (e.key === 'Escape') {
    e.stopPropagation()
    close()
  }
}

watch(open, async (next) => {
  if (next) {
    await nextTick()
  }
})

onMounted(() => {
  store.startPolling()
  document.addEventListener('click', onDocumentClick)
  document.addEventListener('keydown', onKey)
})

onBeforeUnmount(() => {
  store.stopPolling()
  document.removeEventListener('click', onDocumentClick)
  document.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div ref="wrapperRef" class="notif-bell-wrapper">
    <button
      type="button"
      class="notif-bell-btn"
      :aria-label="t('notifications.bellAriaLabel')"
      :aria-expanded="open"
      data-testid="notification-bell"
      @click.stop="toggle"
    >
      <span class="notif-bell-icon" aria-hidden="true">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none">
          <path
            d="M12 2a6 6 0 0 0-6 6v3.5l-2 2.5h16l-2-2.5V8a6 6 0 0 0-6-6Z"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linejoin="round"
          />
          <path
            d="M9 18a3 3 0 0 0 6 0"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
          />
        </svg>
      </span>
      <span v-if="visibleUnread > 0" class="notif-bell-badge" data-testid="notification-bell-badge">
        {{ dropdownLabel }}
      </span>
    </button>
    <div
      v-if="open"
      ref="popoverRef"
      class="notif-dropdown"
      role="dialog"
      :aria-label="t('notifications.dropdownAriaLabel')"
      data-testid="notification-dropdown"
    >
      <header class="notif-dropdown-header">
        <h3>{{ t('notifications.title') }}</h3>
        <div class="notif-dropdown-actions">
          <button
            v-if="visibleUnread > 0"
            type="button"
            class="notif-link-btn"
            data-testid="notification-read-all"
            @click="handleReadAll"
          >
            {{ t('notifications.markAllRead') }}
          </button>
        </div>
      </header>
      <div class="notif-dropdown-filters">
        <button
          type="button"
          class="notif-chip"
          :class="{ active: statusFilter === null }"
          @click="store.setStatusFilter(null)"
        >
          {{ t('notifications.filterAll') }}
        </button>
        <button
          type="button"
          class="notif-chip"
          :class="{ active: statusFilter === 'unread' }"
          @click="store.setStatusFilter('unread')"
        >
          {{ t('notifications.filterUnread') }}
        </button>
      </div>
      <div class="notif-dropdown-body" data-testid="notification-list">
        <div v-if="loading && items.length === 0" class="notif-state">
          {{ t('notifications.loading') }}
        </div>
        <div v-else-if="error" class="notif-state error">
          {{ error }}
        </div>
        <div v-else-if="isEmpty" class="notif-state">
          {{ t('notifications.empty') }}
        </div>
        <ul v-else class="notif-list">
          <li
            v-for="item in items"
            :key="item.id"
            class="notif-item"
            :class="{ unread: item.status === 'unread' }"
            :data-kind="kindChip(item.kind)"
            :data-status="item.status"
          >
            <button
              type="button"
              class="notif-item-main"
              @click="handleRead(item.id)"
            >
              <span class="notif-item-title">{{ item.title }}</span>
              <span v-if="item.body" class="notif-item-body">{{ item.body }}</span>
              <span class="notif-item-meta">
                <span class="notif-item-time">{{ timeAgo(item.created_at) }}</span>
                <span class="notif-item-kind">{{ t(`notifications.kind.${item.kind}`) }}</span>
              </span>
            </button>
            <button
              v-if="item.status !== 'dismissed'"
              type="button"
              class="notif-dismiss-btn"
              :aria-label="t('notifications.dismissAriaLabel')"
              @click.stop="handleDismiss(item.id)"
            >
              ×
            </button>
          </li>
        </ul>
        <div v-if="hasMore" class="notif-loadmore">
          <button
            type="button"
            class="notif-link-btn"
            :disabled="loading"
            @click="handleLoadMore"
          >
            {{ t('notifications.loadMore') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.notif-bell-wrapper {
  position: relative;
  display: inline-flex;
}
.notif-bell-btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  transition: background-color 120ms ease, border-color 120ms ease;
}
.notif-bell-btn:hover,
.notif-bell-btn:focus-visible {
  background: rgba(85, 214, 255, 0.08);
  border-color: rgba(85, 214, 255, 0.4);
  outline: none;
}
.notif-bell-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.notif-bell-badge {
  position: absolute;
  top: 2px;
  right: 2px;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: #ff7b8b;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 18px;
  text-align: center;
}
.notif-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  width: 380px;
  max-height: 520px;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel, #101d30);
  border: 1px solid var(--line, #27405f);
  border-radius: 12px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.32);
  overflow: hidden;
  z-index: 1000;
}
.notif-dropdown-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--line, #27405f);
}
.notif-dropdown-header h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
}
.notif-dropdown-actions {
  display: inline-flex;
  gap: 8px;
}
.notif-dropdown-filters {
  display: flex;
  gap: 6px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--line, #27405f);
}
.notif-chip {
  padding: 4px 10px;
  border: 1px solid var(--line, #27405f);
  border-radius: 999px;
  background: transparent;
  color: inherit;
  font-size: 12px;
  cursor: pointer;
}
.notif-chip.active {
  background: rgba(85, 214, 255, 0.16);
  border-color: rgba(85, 214, 255, 0.6);
}
.notif-dropdown-body {
  flex: 1;
  overflow: auto;
}
.notif-state {
  padding: 24px 16px;
  text-align: center;
  color: var(--muted, #9db0c9);
  font-size: 13px;
}
.notif-state.error {
  color: #ff7b8b;
}
.notif-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.notif-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--line, #27405f);
}
.notif-item:last-child {
  border-bottom: 0;
}
.notif-item.unread {
  background: rgba(85, 214, 255, 0.04);
}
.notif-item-main {
  flex: 1;
  text-align: left;
  background: transparent;
  border: 0;
  padding: 0;
  color: inherit;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.notif-item-title {
  font-size: 13px;
  font-weight: 600;
}
.notif-item.unread .notif-item-title::before {
  content: '';
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 6px;
  border-radius: 50%;
  background: #55d6ff;
  vertical-align: middle;
}
.notif-item-body {
  font-size: 12px;
  color: var(--muted, #9db0c9);
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.notif-item-meta {
  display: flex;
  gap: 8px;
  font-size: 11px;
  color: var(--muted, #9db0c9);
}
.notif-dismiss-btn {
  flex-shrink: 0;
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--muted, #9db0c9);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
}
.notif-dismiss-btn:hover {
  background: rgba(255, 123, 139, 0.12);
  color: #ff7b8b;
}
.notif-link-btn {
  background: transparent;
  border: 0;
  color: #55d6ff;
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}
.notif-link-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.notif-loadmore {
  padding: 10px 16px;
  text-align: center;
  border-top: 1px solid var(--line, #27405f);
}
</style>
