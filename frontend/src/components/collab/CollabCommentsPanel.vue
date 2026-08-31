<!--
  CollabCommentsPanel — v0.7.29 飞书级评论侧栏 (DOC / PPT / SHEET 共用).

  Renders the comment threads anchored to a single doc. The panel calls the
  REST API (no Yjs sync for comments — the polling pattern is simpler and
  keeps the comment thread authoritative on the backend). Threads are
  rendered top-down with chronological replies underneath each anchor;
  each thread shows the anchor reference (e.g. "段落 4-6", "Shape x")
  and a list of messages with author color dots.

  - Add a top-level comment: type into the bottom composer; if you have a
    selection in the parent editor, pass `anchor` via the `:anchor` prop.
  - Reply: click "回复" on any message → composer jumps to that thread.
  - Resolve: click "✓ 解决" on the thread header → backend patch flips
    the resolved flag and the thread collapses to its anchor.
-->
<template>
  <div class="collab-comments">
    <div class="collab-comments__header">
      <h3>评论</h3>
      <button class="collab-comments__toggle" @click="open = !open">
        {{ open ? '收起' : `展开 (${threadCount})` }}
      </button>
    </div>
    <div v-if="open" class="collab-comments__body">
      <div v-if="loading" class="collab-comments__loading">加载中…</div>
      <div v-else-if="threads.length === 0" class="collab-comments__empty">
        还没有评论。选中段落或形状后在这里写下第一条评论。
      </div>
      <div v-else class="collab-comments__threads">
        <div
          v-for="thread in threads"
          :key="thread.thread_id"
          class="collab-comments__thread"
          :class="{ resolved: thread.resolved }"
        >
          <div class="collab-comments__thread-header">
            <span class="collab-comments__anchor">📍 {{ thread.anchor_label }}</span>
            <button
              v-if="!thread.resolved"
              class="collab-comments__resolve"
              @click="resolve(thread)"
            >✓ 解决</button>
            <button class="collab-comments__add-reply" @click="setReply(thread)">回复</button>
          </div>
          <div
            v-for="msg in thread.messages"
            :key="msg.id"
            class="collab-comments__msg"
          >
            <span
              class="collab-comments__avatar"
              :style="{ backgroundColor: msg.author_color }"
              :title="msg.author_name"
            >{{ initialOf(msg.author_name) }}</span>
            <div class="collab-comments__msg-body">
              <div class="collab-comments__msg-meta">
                <strong>{{ msg.author_name }}</strong>
                <span class="collab-comments__msg-time">{{ formatTime(msg.created_at) }}</span>
              </div>
              <div class="collab-comments__msg-text">{{ msg.body }}</div>
            </div>
          </div>
          <div v-if="replyThread === thread.thread_id" class="collab-comments__composer">
            <textarea
              v-model="replyDraft"
              rows="2"
              placeholder="写下回复…"
              @keydown.meta.enter="submitReply(thread)"
              @keydown.ctrl.enter="submitReply(thread)"
            />
            <div class="collab-comments__composer-actions">
              <button @click="replyThread = null">取消</button>
              <button @click="submitReply(thread)" :disabled="!replyDraft.trim() || submitting">发送</button>
            </div>
          </div>
        </div>
      </div>
      <!-- New thread composer -->
      <div class="collab-comments__composer collab-comments__composer--new">
        <textarea
          v-model="newDraft"
          rows="3"
          :placeholder="placeholder"
          @keydown.meta.enter="submitNew"
          @keydown.ctrl.enter="submitNew"
        />
        <div class="collab-comments__composer-actions">
          <span v-if="anchor" class="collab-comments__composer-anchor">📍 {{ anchorLabel }}</span>
          <button @click="submitNew" :disabled="!newDraft.trim() || submitting">
            {{ submitting ? '发送中…' : '发送 (⌘↵)' }}
          </button>
        </div>
        <p v-if="error" class="collab-comments__error">⚠ {{ error }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listCollabDocComments,
  createCollabDocComment,
  updateCollabDocComment,
  type CollabDocComment,
} from '@/api/collabDoc'

const props = defineProps<{
  docId: string
  token: string
  /** Anchor info for new top-level threads (optional). */
  anchor?: { type: 'doc' | 'slide' | 'sheet'; ref: string } | null
  /** Optional human-readable label for the anchor (e.g. "段落 4-6"). */
  anchorLabel?: string
  /** Initial placeholder for the composer. */
  placeholder?: string
}>()

const open = ref(true)
const loading = ref(false)
const submitting = ref(false)
const error = ref<string | null>(null)
const comments = ref<CollabDocComment[]>([])
const replyThread = ref<string | null>(null)
const replyDraft = ref('')
const newDraft = ref('')
let pollTimer: number | null = null

const initialOf = (s: string) => (s || '?').trim().charAt(0).toUpperCase()

const formatTime = (iso: string) => {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getMonth() + 1}/${d.getDate()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

interface Thread {
  thread_id: string
  anchor_label: string
  resolved: boolean
  messages: CollabDocComment[]
}

const threads = computed<Thread[]>(() => {
  const map = new Map<string, Thread>()
  for (const c of comments.value) {
    let t = map.get(c.thread_id)
    if (!t) {
      t = {
        thread_id: c.thread_id,
        anchor_label: describeAnchor(c.anchor_type, c.anchor_ref),
        resolved: !!c.resolved,
        messages: [],
      }
      map.set(c.thread_id, t)
    }
    t.messages.push(c)
    // Once any reply is resolved the thread is resolved.
    if (c.resolved) t.resolved = true
  }
  return Array.from(map.values()).sort((a, b) => {
    const aTop = a.messages[0]
    const bTop = b.messages[0]
    return (aTop?.created_at ?? '').localeCompare(bTop?.created_at ?? '')
  })
})

const threadCount = computed(() => threads.value.length)

const describeAnchor = (type: string, ref: string): string => {
  if (!ref) return type
  try {
    const o = JSON.parse(ref)
    if (type === 'doc' && o.from != null) return `段落 ${o.from + 1}-${(o.to ?? o.from) + 1}`
    if (type === 'slide' && o.slide != null) return `幻灯片 ${o.slide + 1}`
    if (type === 'sheet' && o.cell) return `单元格 ${o.cell}`
  } catch {}
  return type
}

const refresh = async () => {
  loading.value = true
  try {
    const res = await listCollabDocComments(props.docId, { limit: 500 })
    comments.value = res.comments || []
    error.value = null
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

const setReply = (thread: Thread) => {
  replyThread.value = thread.thread_id
  replyDraft.value = ''
}

const submitReply = async (thread: Thread) => {
  if (!replyDraft.value.trim()) return
  submitting.value = true
  try {
    await createCollabDocComment(props.docId, {
      thread_id: thread.thread_id,
      parent_id: thread.messages[0]?.id,
      anchor_type: thread.messages[0]?.anchor_type ?? 'doc',
      anchor_ref: thread.messages[0]?.anchor_ref ?? '',
      body: replyDraft.value.trim(),
    })
    replyDraft.value = ''
    replyThread.value = null
    await refresh()
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    submitting.value = false
  }
}

const submitNew = async () => {
  if (!newDraft.value.trim()) return
  if (!props.anchor) {
    error.value = '请先在编辑器中选中段落 / 形状 / 单元格后再评论'
    return
  }
  submitting.value = true
  try {
    await createCollabDocComment(props.docId, {
      anchor_type: props.anchor.type,
      anchor_ref: props.anchor.ref,
      body: newDraft.value.trim(),
    })
    newDraft.value = ''
    await refresh()
    MessagePlugin.success('评论已添加')
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    submitting.value = false
  }
}

const resolveThread = async (thread: Thread) => {
  submitting.value = true
  try {
    // Mark the top-level (first) message resolved.
    const head = thread.messages[0]
    await updateCollabDocComment(props.docId, head.id, { resolved: true })
    await refresh()
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    submitting.value = false
  }
}

const resolve = (thread: Thread) => {
  if (!window.confirm(`确认解决线程 (${thread.messages.length} 条消息) ?`)) return
  resolveThread(thread)
}

onMounted(() => {
  refresh()
  // Light polling so a second tab typing a comment at the same time
  // surfaces within ~5s. Yjs could replace this for realtime, but the
  // comment thread is small enough that polling keeps the server
  // authoritative and avoids Yjs schema drift.
  pollTimer = window.setInterval(refresh, 5000)
})

onBeforeUnmount(() => {
  if (pollTimer) window.clearInterval(pollTimer)
})
</script>

<style scoped>
.collab-comments {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  margin: 12px 0;
}
.collab-comments__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
}
.collab-comments__header h3 {
  margin: 0;
  font-size: 14px;
}
.collab-comments__toggle {
  background: transparent;
  border: none;
  color: var(--td-brand-color-7);
  cursor: pointer;
  font-size: 12px;
}
.collab-comments__body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.collab-comments__loading,
.collab-comments__empty {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  text-align: center;
  padding: 12px 0;
}
.collab-comments__threads {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.collab-comments__thread {
  background: var(--td-bg-color-secondary);
  border-radius: 6px;
  padding: 8px 12px;
  border-left: 3px solid var(--td-brand-color-7);
}
.collab-comments__thread.resolved {
  opacity: 0.55;
  border-left-color: var(--td-success-color-7);
}
.collab-comments__thread-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.collab-comments__anchor {
  flex: 1;
}
.collab-comments__resolve,
.collab-comments__add-reply {
  background: transparent;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  padding: 2px 8px;
  cursor: pointer;
  font-size: 11px;
  color: var(--td-text-color-secondary);
}
.collab-comments__resolve:hover { color: var(--td-success-color-7); }
.collab-comments__msg {
  display: flex;
  gap: 8px;
  padding: 6px 0;
}
.collab-comments__avatar {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  color: #fff;
  font-size: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.collab-comments__msg-body { flex: 1; min-width: 0; }
.collab-comments__msg-meta {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--td-text-color-secondary);
  margin-bottom: 2px;
}
.collab-comments__msg-text {
  font-size: 13px;
  word-break: break-word;
  white-space: pre-wrap;
}
.collab-comments__composer {
  margin-top: 6px;
}
.collab-comments__composer textarea {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  padding: 6px 8px;
  font-family: inherit;
  font-size: 13px;
  resize: vertical;
}
.collab-comments__composer-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
}
.collab-comments__composer-actions button {
  background: var(--td-brand-color-7);
  color: #fff;
  border: none;
  border-radius: 4px;
  padding: 4px 12px;
  cursor: pointer;
  font-size: 12px;
}
.collab-comments__composer-actions button:disabled {
  background: var(--td-component-stroke);
  cursor: not-allowed;
}
.collab-comments__composer-anchor {
  margin-right: auto;
  font-size: 11px;
  color: var(--td-text-color-secondary);
}
.collab-comments__composer--new {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed var(--td-component-stroke);
}
.collab-comments__error {
  color: var(--td-error-color-7);
  font-size: 12px;
  margin: 4px 0 0 0;
}
</style>
