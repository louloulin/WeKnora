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
            <button
              v-else
              class="collab-comments__reopen"
              @click="reopen(thread)"
              title="重新打开线程"
            >↺ 重新打开</button>
            <button class="collab-comments__add-reply" @click="setReply(thread)">回复</button>
            <button class="collab-comments__delete" @click="remove(thread)">删除</button>
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
              ref="replyDraftTextarea"
              v-model="replyDraft"
              rows="2"
              placeholder="写下回复… 输入 @ 提及同事"
              @input="onComposerInput('reply')"
              @keydown.meta.enter="submitReply(thread)"
              @keydown.ctrl.enter="submitReply(thread)"
              @keydown="mentionKeydown"
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
          ref="newDraftTextarea"
          v-model="newDraft"
          rows="3"
          :placeholder="placeholder || '写评论… 输入 @ 提及同事'"
          @input="onComposerInput('new')"
          @keydown.meta.enter="submitNew"
          @keydown.ctrl.enter="submitNew"
          @keydown="mentionKeydown"
        />
        <div class="collab-comments__composer-actions">
          <span v-if="anchor" class="collab-comments__composer-anchor">📍 {{ anchorLabel }}</span>
          <button @click="submitNew" :disabled="!newDraft.trim() || submitting">
            {{ submitting ? '发送中…' : '发送 (⌘↵)' }}
          </button>
        </div>
        <p v-if="error" class="collab-comments__error">⚠ {{ error }}</p>
      </div>

      <!-- v0.7.197 — @-mention picker popover. Floats above the composer
           that originated the `@` keystroke; pointer/keyboard navigable;
           selecting a member inserts `@username ` into the textarea and
           records a MentionChip used at submit time. -->
      <div v-if="mentionOpen" class="collab-comments__mention-popover" data-testid="mention-popover">
        <div class="collab-comments__mention-title">提到 {{ mentionQuery ? '"\@' + mentionQuery + '"' : '成员' }}</div>
        <div v-if="mentionLoading" class="collab-comments__mention-empty">搜索中…</div>
        <div v-else-if="visibleMentionResults.length === 0" class="collab-comments__mention-empty">没有匹配成员</div>
        <ul v-else class="collab-comments__mention-list">
          <li
            v-for="(m, idx) in visibleMentionResults"
            :key="m.user_id"
            class="collab-comments__mention-item"
            :class="{ active: idx === mentionActive }"
            @mousedown.prevent="pickMention(m)"
            @mouseenter="mentionActive = idx"
            data-testid="mention-option"
          >
            <span class="collab-comments__mention-avatar">{{ (m.username || m.email)[0]?.toUpperCase() }}</span>
            <span class="collab-comments__mention-meta">
              <span class="collab-comments__mention-name">{{ m.username || m.email.split('@')[0] }}</span>
              <span class="collab-comments__mention-email">{{ m.email }}</span>
            </span>
            <span class="collab-comments__mention-role">{{ m.role }}</span>
          </li>
        </ul>
        <div class="collab-comments__mention-hint">↑↓ 选择 · Enter 确认 · Esc 关闭</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listCollabDocComments,
  createCollabDocComment,
  updateCollabDocComment, deleteCollabDocComment,
  type CollabDocComment,
} from '@/api/collabDoc'
import { listMembers, type TenantMember } from '@/api/tenant/members'
import { useAuthStore } from '@/stores/auth'

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

// v0.7.49 — notify the parent editor that a thread was created so it can
// attach the comment mark to the selected text range.
const emit = defineEmits<{ created: [comment: CollabDocComment]; deleted: [comment: CollabDocComment]; loaded: [comments: CollabDocComment[]] }>()

const open = ref(false)
const loading = ref(false)
const submitting = ref(false)
const error = ref<string | null>(null)
const comments = ref<CollabDocComment[]>([])
const replyThread = ref<string | null>(null)
const replyDraft = ref('')
const newDraft = ref('')
let pollTimer: number | null = null

// v0.7.197 — @-mention picker. The composer parses the trailing '@' as the
// user types; matches are looked up via the tenant members API. Selected
// users get inserted as a chip (the textarea body preserves the literal
// `@username` text for read-side rendering; the structured `mentions` map
// carries the user_id → display-name relationship used in the payload).
interface MentionChip {
  user_id: string
  display: string
  start: number
  end: number   // exclusive cursor after the inserted `@display` text
}
const mentions = ref<MentionChip[]>([])
const mentionQuery = ref('')
const mentionOpen = ref(false)
const mentionResults = ref<TenantMember[]>([])
const mentionActive = ref(0)
const mentionLoading = ref(false)
let mentionSearchTimer: number | null = null
let mentionAnchorTextarea: HTMLTextAreaElement | null = null
let mentionAnchorStart = 0
let mentionAnchorEnd = 0

const authStore = useAuthStore()
const currentUserId = computed(() => String((authStore.user as any)?.id || (authStore.user as any)?.user_id || ''))

const visibleMentionResults = computed(() => {
  const me = currentUserId.value
  return mentionResults.value.filter((m) => m.user_id !== me)
})

function updateMentionsFromText(text: string) {
  // Drop mentions whose inserted token has been edited/removed by the user.
  const kept = mentions.value.filter((m) => text.slice(m.start, m.end) === '@' + m.display)
  if (kept.length !== mentions.value.length) {
    // Indices shift: rebuild from scratch on the remaining chips.
    const rebuilt: MentionChip[] = []
    for (const k of kept) {
      const idx = text.indexOf('@' + k.display)
      if (idx >= 0) rebuilt.push({ ...k, start: idx, end: idx + 1 + k.display.length })
    }
    mentions.value = rebuilt
  } else {
    // Just re-sync start/end in case the text grew/shrank around them.
    for (const m of mentions.value) {
      const idx = text.indexOf('@' + m.display)
      if (idx >= 0) {
        m.start = idx
        m.end = idx + 1 + m.display.length
      }
    }
  }
}

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
    emit('loaded', comments.value)
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

function closeMentionPicker() {
  mentionOpen.value = false
  mentionQuery.value = ''
  mentionResults.value = []
  mentionActive.value = 0
}

function detectAtTrigger(text: string, caret: number, source: 'new' | 'reply'): boolean {
  // Only the most recent '@...' (since last whitespace/start) is the active query.
  const before = text.slice(0, caret)
  const m = before.match(/(?:^|\s)@([^\s@]*)$/)
  if (!m) {
    if (mentionOpen.value) closeMentionPicker()
    return false
  }
  const query = m[1]
  const atIndex = caret - query.length - 1
  mentionAnchorTextarea = source === 'new' ? (newDraftTextarea.value) : (replyDraftTextarea.value)
  mentionAnchorStart = atIndex
  mentionAnchorEnd = caret
  mentionQuery.value = query
  if (!mentionOpen.value) mentionOpen.value = true
  scheduleMentionSearch(query)
  return true
}

function scheduleMentionSearch(query: string) {
  if (mentionSearchTimer) window.clearTimeout(mentionSearchTimer)
  // 120ms debounce — feels instant while preventing request storm while typing
  mentionSearchTimer = window.setTimeout(() => runMentionSearch(query), 120)
}

async function runMentionSearch(query: string) {
  const tenantId = (authStore as any)?.activeTenant?.id ?? (authStore as any)?.user?.tenant_id ?? (authStore as any)?.tenantId
  if (!tenantId) {
    mentionResults.value = []
    return
  }
  mentionLoading.value = true
  try {
    const resp = await listMembers(Number(tenantId), { q: query, page: 1, page_size: 8 })
    mentionResults.value = resp?.data?.members ?? []
    mentionActive.value = 0
  } catch {
    mentionResults.value = []
  } finally {
    mentionLoading.value = false
  }
}

function pickMention(member: TenantMember) {
  if (!mentionAnchorTextarea) return
  const display = (member.username || member.email.split('@')[0]).trim()
  if (!display) return
  const draft = mentionAnchorTextarea === newDraftTextarea.value ? newDraft : replyDraft
  const before = draft.value.slice(0, mentionAnchorStart)
  const after = draft.value.slice(mentionAnchorEnd)
  const inserted = '@' + display + ' '
  const next = before + inserted + after
  draft.value = next
  const newCaret = (before + inserted).length
  // Record chip (use display name as a stable lookup key)
  const chip: MentionChip = {
    user_id: member.user_id,
    display,
    start: before.length,
    end: before.length + inserted.length - 1,  // exclude trailing space
  }
  // Remove any overlapping chip first
  mentions.value = mentions.value.filter((m) => m.display !== display)
  mentions.value.push(chip)
  updateMentionsFromText(next)
  // Restore caret
  nextTick(() => {
    mentionAnchorTextarea!.focus()
    mentionAnchorTextarea!.setSelectionRange(newCaret, newCaret)
  })
  closeMentionPicker()
}

function mentionKeydown(e: KeyboardEvent): boolean {
  if (!mentionOpen.value || visibleMentionResults.value.length === 0) return false
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    mentionActive.value = (mentionActive.value + 1) % visibleMentionResults.value.length
    return true
  }
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    mentionActive.value = (mentionActive.value - 1 + visibleMentionResults.value.length) % visibleMentionResults.value.length
    return true
  }
  if (e.key === 'Enter' || e.key === 'Tab') {
    e.preventDefault()
    const pick = visibleMentionResults.value[mentionActive.value]
    if (pick) pickMention(pick)
    return true
  }
  if (e.key === 'Escape') {
    e.preventDefault()
    closeMentionPicker()
    return true
  }
  return false
}

const newDraftTextarea = ref<HTMLTextAreaElement | null>(null)
const replyDraftTextarea = ref<HTMLTextAreaElement | null>(null)

function onComposerInput(source: 'new' | 'reply') {
  const ta = source === 'new' ? newDraftTextarea.value : replyDraftTextarea.value
  if (!ta) return
  const text = source === 'new' ? newDraft.value : replyDraft.value
  updateMentionsFromText(text)
  detectAtTrigger(text, ta.selectionStart ?? text.length, source)
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
      mentioned_user_ids: mentions.value
        .filter((m) => replyDraft.value.includes('@' + m.display))
        .map((m) => m.user_id),
    })
    replyDraft.value = ''
    replyThread.value = null
    mentions.value = mentions.value.filter((m) => !replyDraft.value.includes('@' + m.display))
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
    const mentionedIds = mentions.value
      .filter((m) => newDraft.value.includes('@' + m.display))
      .map((m) => m.user_id)
    const created = await createCollabDocComment(props.docId, {
      anchor_type: props.anchor.type,
      anchor_ref: props.anchor.ref,
      body: newDraft.value.trim(),
      mentioned_user_ids: mentionedIds.length ? mentionedIds : undefined,
    })
    newDraft.value = ''
    mentions.value = []
    await refresh()
    emit('created', created)
    MessagePlugin.success(mentionedIds.length ? `评论已添加，@${mentionedIds.length} 人将收到通知` : '评论已添加')
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

const reopenThread = async (thread: Thread) => {
  submitting.value = true
  try {
    const head = thread.messages[0]
    await updateCollabDocComment(props.docId, head.id, { resolved: false })
    await refresh()
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    submitting.value = false
  }
}

const reopen = (thread: Thread) => {
  reopenThread(thread)
}

const resolve = (thread: Thread) => {
  if (!window.confirm(`确认解决线程 (${thread.messages.length} 条消息) ?`)) return
  resolveThread(thread)
}

// v0.7.52 — delete the whole thread (replies cascade via FK) and notify the
// parent editor so the comment mark is stripped from the text.
const removeThread = async (thread: Thread) => {
  submitting.value = true
  try {
    const head = thread.messages[0]
    await deleteCollabDocComment(props.docId, head.id)
    await refresh()
    emit('deleted', head)
    MessagePlugin.success('评论已删除')
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    submitting.value = false
  }
}

const remove = (thread: Thread) => {
  if (!window.confirm(`确认删除线程 (${thread.messages.length} 条消息) ?`)) return
  removeThread(thread)
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

/* v0.7.197 — @-mention picker */
.collab-comments__mention-popover {
  position: fixed;
  z-index: 1200;
  right: 24px;
  bottom: 24px;
  width: 320px;
  max-height: 320px;
  overflow: auto;
  background: var(--app-surface-bg, #1f1f23);
  color: var(--app-text, #e7e9ee);
  border: 1px solid var(--app-border, #333);
  border-radius: 8px;
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.35);
  padding: 6px 0;
  font-size: 13px;
}
.collab-comments__mention-title {
  padding: 6px 12px;
  font-size: 11px;
  color: var(--app-text-muted, #888);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.collab-comments__mention-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.collab-comments__mention-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  cursor: pointer;
}
.collab-comments__mention-item.active,
.collab-comments__mention-item:hover {
  background: rgba(88, 166, 255, 0.15);
}
.collab-comments__mention-avatar {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--td-brand-color, #5aa8ff);
  color: #fff;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex: 0 0 auto;
}
.collab-comments__mention-meta {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.collab-comments__mention-name {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.collab-comments__mention-email {
  color: var(--app-text-muted, #888);
  font-size: 11px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.collab-comments__mention-role {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.08);
  color: var(--app-text-muted, #aaa);
  text-transform: uppercase;
}
.collab-comments__mention-empty {
  padding: 14px 12px;
  color: var(--app-text-muted, #888);
  font-size: 12px;
  text-align: center;
}
.collab-comments__mention-hint {
  padding: 6px 12px;
  font-size: 10px;
  color: var(--app-text-muted, #888);
  border-top: 1px solid var(--app-border, #333);
  margin-top: 4px;
}

</style>
