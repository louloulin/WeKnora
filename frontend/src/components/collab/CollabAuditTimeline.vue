<!--
  CollabAuditTimeline — v0.7.30 audit log timeline panel.

  Renders the immutable operation history for a single collaborative doc.
  Two modes:
    • Timeline  – paginated list, newest first (default)
    • Summary   – rolled-up counts by action / day (toggle on the right)

  Powers the "history" tab in the doc detail panel. Does not write — only
  reads from `listCollabDocAudit` / `collabDocAuditSummary`.
-->
<template>
  <div class="collab-audit">
    <header class="collab-audit__header">
      <h4 class="collab-audit__title">操作历史</h4>
      <div class="collab-audit__mode-toggle">
        <button
          type="button"
          class="collab-audit__mode-btn"
          :class="{ active: mode === 'timeline' }"
          @click="mode = 'timeline'"
        >时间线</button>
        <button
          type="button"
          class="collab-audit__mode-btn"
          :class="{ active: mode === 'summary' }"
          @click="mode = 'summary'"
        >汇总</button>
      </div>
      <button
        v-if="mode === 'timeline'"
        type="button"
        class="collab-audit__refresh"
        :disabled="loading"
        @click="reloadTimeline"
        data-testid="audit-refresh"
      >
        {{ loading ? '加载中…' : '刷新' }}
      </button>
    </header>

    <div v-if="loadError" class="collab-audit__error">{{ loadError }}</div>

    <!-- Timeline mode -->
    <ol v-else-if="mode === 'timeline'" class="collab-audit__timeline" data-testid="audit-timeline">
      <li
        v-for="e in entries"
        :key="e.id"
        class="collab-audit__row"
        :class="`collab-audit__row--${e.action.split('.')[0]}`"
      >
        <span
          class="collab-audit__avatar"
          :style="{ backgroundColor: e.actor_color || '#7d8590' }"
        >{{ initialOf(e.actor_name) }}</span>
        <div class="collab-audit__body">
          <div class="collab-audit__line1">
            <strong class="collab-audit__actor">{{ e.actor_name || '匿名' }}</strong>
            <span class="collab-audit__verb">{{ describe(e) }}</span>
            <span class="collab-audit__time">{{ formatTime(e.created_at) }}</span>
          </div>
          <div v-if="e.payload" class="collab-audit__payload">{{ describePayload(e.payload) }}</div>
        </div>
      </li>
      <li v-if="!loading && entries.length === 0" class="collab-audit__empty">
        暂无操作记录
      </li>
    </ol>

    <!-- Summary mode -->
    <div v-else class="collab-audit__summary" data-testid="audit-summary">
      <div class="collab-audit__summary-stat">
        <span class="collab-audit__summary-num">{{ summary?.total_entries ?? 0 }}</span>
        <span class="collab-audit__summary-label">总操作数</span>
      </div>
      <h5 class="collab-audit__subheading">按动作</h5>
      <ul class="collab-audit__chips">
        <li
          v-for="[action, n] in Object.entries(summary?.by_action ?? {})"
          :key="action"
          class="collab-audit__chip"
        >
          <span class="collab-audit__chip-name">{{ describeActionShort(action) }}</span>
          <span class="collab-audit__chip-n">{{ n }}</span>
        </li>
      </ul>
      <h5 class="collab-audit__subheading">按天</h5>
      <ul class="collab-audit__days">
        <li v-for="d in summary?.by_day ?? []" :key="d.day" class="collab-audit__day">
          <span class="collab-audit__day-name">{{ d.day }}</span>
          <span class="collab-audit__day-bar-wrap">
            <span
              class="collab-audit__day-bar"
              :style="{ width: dayBarWidth(d.count) + '%' }"
            ></span>
          </span>
          <span class="collab-audit__day-n">{{ d.count }}</span>
        </li>
      </ul>
      <p v-if="(summary?.by_day ?? []).length === 0" class="collab-audit__empty">
        暂无汇总数据
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch as vueWatch } from 'vue'
import {
  listCollabDocAudit,
  collabDocAuditSummary,
  type CollabDocAuditEntry,
  type CollabDocAuditSummary,
  type ListCollabDocAuditFilter,
} from '@/api/collabDoc'

interface Props {
  docId: string
  /** Optional: pre-filter to one action. */
  initialAction?: ListCollabDocAuditFilter['action']
  /** Poll every 30 s so newly-recorded events appear without manual refresh. */
  pollMs?: number
}
const props = withDefaults(defineProps<Props>(), { pollMs: 30_000 })

const mode = ref<'timeline' | 'summary'>('timeline')
const entries = ref<CollabDocAuditEntry[]>([])
const summary = ref<CollabDocAuditSummary | null>(null)
const loading = ref(false)
const loadError = ref<string | null>(null)
let pollHandle: ReturnType<typeof setInterval> | null = null

const initialOf = (s: string) => (s || '?').trim().charAt(0).toUpperCase()

const ACTION_VERB: Record<string, string> = {
  create: '创建了文档',
  rename: '重命名了文档',
  upload: '上传了新版本',
  save: '保存了修改',
  'share.enable': '开启了分享链接',
  'share.disable': '关闭了分享链接',
  'share.access': '查看了分享链接',
  archive: '归档了文档',
  restore: '恢复了文档',
  delete: '删除了文档',
  'comment.add': '添加了评论',
  'comment.reply': '回复了评论',
  'comment.solve': '解决了评论',
  'comment.delete': '删除了评论',
  polish: '运行了 AI 润色',
  sync_to_kb: '同步到了知识库',
  export: '导出了文档',
}

const describe = (e: CollabDocAuditEntry): string => {
  const verb = ACTION_VERB[e.action] ?? e.action
  if (e.target) return `${verb} (${e.target})`
  return verb
}

const describeActionShort = (action: string): string => {
  if (action.endsWith('.enable')) return '开启分享'
  if (action.endsWith('.disable')) return '关闭分享'
  if (action.endsWith('.access')) return '访问分享'
  if (action.startsWith('comment.')) return '评论'
  return action
}

const describePayload = (payload: string): string => {
  // Best-effort: try JSON-pretty, else raw.
  try {
    return JSON.stringify(JSON.parse(payload), null, 0).slice(0, 200)
  } catch {
    return payload.length > 200 ? payload.slice(0, 200) + '…' : payload
  }
}

const formatTime = (iso: string): string => {
  try {
    const d = new Date(iso)
    return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
  } catch {
    return iso.slice(0, 16)
  }
}

const dayBarWidth = (count: number): number => {
  if (!summary.value || summary.value.total_entries === 0) return 0
  return Math.min(100, Math.round((count / summary.value.total_entries) * 100 * 5))
}

const reloadTimeline = async () => {
  loading.value = true
  loadError.value = null
  try {
    const { entries: rows } = await listCollabDocAudit(props.docId, {
      action: props.initialAction,
      limit: 200,
    })
    entries.value = rows
  } catch (e: any) {
    loadError.value = e?.message ?? String(e)
  } finally {
    loading.value = false
  }
}

const reloadSummary = async () => {
  loading.value = true
  loadError.value = null
  try {
    summary.value = await collabDocAuditSummary({ doc: props.docId })
  } catch (e: any) {
    loadError.value = e?.message ?? String(e)
  } finally {
    loading.value = false
  }
}

/** Re-fetch when mode toggles between timeline / summary. */
vueWatch(mode, async (m) => {
  if (m === 'timeline') await reloadTimeline()
  else await reloadSummary()
})

onMounted(async () => {
  await reloadTimeline()
  if (props.pollMs > 0) {
    pollHandle = setInterval(() => {
      if (mode.value === 'timeline') reloadTimeline()
      else reloadSummary()
    }, props.pollMs)
  }
})

onBeforeUnmount(() => {
  if (pollHandle) clearInterval(pollHandle)
})
</script>

<style scoped>
.collab-audit {
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 12px;
  color: var(--td-text-color-primary);
}
.collab-audit__header {
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid var(--td-component-stroke);
  padding-bottom: 8px;
}
.collab-audit__title {
  font-size: 13px;
  font-weight: 600;
  margin: 0;
  flex: 1;
}
.collab-audit__mode-toggle { display: inline-flex; }
.collab-audit__mode-btn {
  background: transparent;
  border: 1px solid var(--td-component-stroke);
  padding: 3px 10px;
  font-size: 11px;
  cursor: pointer;
}
.collab-audit__mode-btn.active {
  background: var(--td-brand-color-1);
  color: var(--td-brand-color-7);
}
.collab-audit__refresh {
  background: transparent;
  border: 1px solid var(--td-component-stroke);
  padding: 3px 10px;
  font-size: 11px;
  cursor: pointer;
  border-radius: 3px;
}
.collab-audit__refresh:disabled { opacity: 0.5; cursor: default; }
.collab-audit__error { color: var(--td-error-color); padding: 8px 0; }
.collab-audit__timeline {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 360px;
  overflow-y: auto;
}
.collab-audit__row {
  display: grid;
  grid-template-columns: 22px 1fr;
  gap: 8px;
  align-items: flex-start;
  padding: 4px 0;
}
.collab-audit__avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 11px;
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  flex-shrink: 0;
}
.collab-audit__body { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.collab-audit__line1 { display: flex; flex-wrap: wrap; gap: 6px; align-items: baseline; }
.collab-audit__actor { font-weight: 600; }
.collab-audit__verb { color: var(--td-text-color-secondary); }
.collab-audit__time { color: var(--td-text-color-placeholder); margin-left: auto; font-size: 11px; }
.collab-audit__payload {
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 10px;
  color: var(--td-text-color-placeholder);
  background: var(--td-bg-color-secondarycontainer);
  padding: 2px 6px;
  border-radius: 3px;
  word-break: break-all;
}
.collab-audit__empty {
  color: var(--td-text-color-placeholder);
  padding: 20px;
  text-align: center;
  list-style: none;
}
.collab-audit__summary { display: flex; flex-direction: column; gap: 6px; }
.collab-audit__summary-stat {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 8px 0;
}
.collab-audit__summary-num { font-size: 22px; font-weight: 700; }
.collab-audit__summary-label { color: var(--td-text-color-secondary); }
.collab-audit__subheading {
  font-size: 11px;
  text-transform: uppercase;
  color: var(--td-text-color-placeholder);
  margin: 8px 0 0;
}
.collab-audit__chips { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px; }
.collab-audit__chip {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  padding: 3px 8px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
  font-size: 11px;
}
.collab-audit__chip-n { font-weight: 600; }
.collab-audit__days { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.collab-audit__day {
  display: grid;
  grid-template-columns: 100px 1fr 40px;
  gap: 6px;
  align-items: center;
}
.collab-audit__day-name { font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.collab-audit__day-bar-wrap {
  height: 6px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 3px;
  overflow: hidden;
}
.collab-audit__day-bar {
  display: block;
  height: 100%;
  background: var(--td-brand-color-7);
}
.collab-audit__day-n { text-align: right; font-size: 11px; }
</style>
