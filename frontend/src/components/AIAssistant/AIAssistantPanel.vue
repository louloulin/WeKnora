<!--
  AI Assistant Panel — v0.7.17 frontend.

  Right-side panel that consumes the SSE assistant backend
  (internal/llmstream + internal/handler/assistant.go) and renders
  streaming tokens + citations inline. Designed to be embedded next
  to a Wiki / KB document editor (see docs/ai-assistant-panel-
  mockup-v0.7.17.html for the visual reference).

  Event flow:
    1. User types a query → emits @submit
    2. askAssistantStream() opens an SSE connection
    3. metadata frame → save conversation_id, render header
    4. citation frames → append to citations[]
    5. token frames → append to assistant answer
    6. done frame → mark streaming=false, persist history
    7. error frame → show retry / citation-only fallback

  The component is fully self-contained — no Vuex / Pinia
  dependency. Mounting is via <AIAssistantPanel :kb-ids="..." />.
-->
<template>
  <aside class="ai-assistant-panel" :class="{ 'is-collapsed': collapsed }">
    <header class="ai-assistant-panel__header">
      <div class="ai-assistant-panel__brand">
        <span class="ai-assistant-panel__logo">A</span>
        <div>
          <div class="ai-assistant-panel__title">{{ t('assistant.title') }}</div>
          <div class="ai-assistant-panel__subtitle">
            {{ scopeLabel }}
          </div>
        </div>
      </div>
      <div class="ai-assistant-panel__actions">
        <button
          type="button"
          class="ai-assistant-panel__btn"
          :title="t('assistant.history')"
          @click="$emit('open-history')"
        >
          <t-icon name="history" size="14px" />
        </button>
        <button
          type="button"
          class="ai-assistant-panel__btn"
          :title="t('assistant.clear')"
          :disabled="messages.length === 0"
          @click="clearMessages"
        >
          <t-icon name="delete" size="14px" />
        </button>
        <button
          type="button"
          class="ai-assistant-panel__btn"
          :title="collapsed ? t('assistant.expand') : t('assistant.collapse')"
          @click="$emit('toggle-collapse')"
        >
          <t-icon :name="collapsed ? 'chevron-left' : 'chevron-right'" size="14px" />
        </button>
      </div>
    </header>

    <div ref="messagesEl" class="ai-assistant-panel__messages">
      <div
        v-for="msg in messages"
        :key="msg.id"
        class="ai-assistant-panel__msg"
        :class="{ 'is-user': msg.role === 'user', 'is-assistant': msg.role === 'assistant' }"
      >
        <div v-if="msg.role === 'user'" class="ai-assistant-panel__msg-bubble">
          {{ msg.content }}
        </div>
        <template v-else>
          <div class="ai-assistant-panel__msg-label">
            <t-icon name="robot" size="12px" /> {{ t('assistant.assistantLabel') }}
            <span v-if="msg.modelName" class="ai-assistant-panel__msg-model">{{ msg.modelName }}</span>
          </div>
          <div class="ai-assistant-panel__msg-bubble" v-html="renderAnswer(msg.content)" />
          <div
            v-if="msg.citations && msg.citations.length"
            class="ai-assistant-panel__citations"
          >
            <div
              v-for="(c, i) in msg.citations"
              :key="i"
              class="ai-assistant-panel__citation"
              @click="$emit('open-citation', c)"
            >
              <span class="ai-assistant-panel__citation-kind" :class="'is-' + c.type">
                {{ c.type === 'kb' ? 'KB' : 'WIKI' }}
              </span>
              <div class="ai-assistant-panel__citation-body">
                <div class="ai-assistant-panel__citation-title">{{ c.title }}</div>
                <div class="ai-assistant-panel__citation-snippet">{{ c.snippet }}</div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div v-if="error" class="ai-assistant-panel__error">
        <t-icon name="error-circle" size="14px" />
        <span>{{ error }}</span>
        <button type="button" class="ai-assistant-panel__btn" @click="retry">
          {{ t('assistant.retry') }}
        </button>
      </div>
    </div>

    <footer class="ai-assistant-panel__composer">
      <div class="ai-assistant-panel__hints">
        <span class="ai-assistant-panel__hint">
          <t-icon name="bookmark" size="11px" />
          {{ citationsCount }} {{ t('assistant.citationsCount') }}
        </span>
        <span v-if="streaming" class="ai-assistant-panel__hint">
          <t-icon name="loading" size="11px" />
          {{ t('assistant.generating') }}
        </span>
        <span v-else-if="lastLatencyMs" class="ai-assistant-panel__hint">
          ⏱ {{ lastLatencyMs }}ms
        </span>
      </div>
      <form class="ai-assistant-panel__form" @submit.prevent="onSubmit">
        <textarea
          v-model="query"
          ref="inputEl"
          class="ai-assistant-panel__input"
          :placeholder="t('assistant.placeholder')"
          :disabled="streaming"
          rows="1"
          @keydown.enter.exact.prevent="onSubmit"
          @input="autoGrow"
        />
        <button
          v-if="streaming"
          type="button"
          class="ai-assistant-panel__send is-stop"
          @click="abort"
        >
          {{ t('assistant.stop') }}
        </button>
        <button
          v-else
          type="submit"
          class="ai-assistant-panel__send"
          :disabled="!canSend"
        >
          {{ t('assistant.send') }}
        </button>
      </form>
    </footer>
  </aside>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onUnmounted } from 'vue'
import { Message } from 'tdesign-icons-vue-next'
import { useI18n } from 'vue-i18n'
import { askAssistantStream } from '@/api/assistant'
import type {
  AssistantAskRequest,
  AssistantCitation,
  AssistantStreamEvent,
} from '@/api/assistant/types'

const props = defineProps<{
  /** Knowledge base ids to scope the retrieval. Empty = all visible KBs. */
  kbIds?: string[]
  /** Whether to include wiki hits in the retrieval. */
  includeWiki?: boolean
  /** Whether the panel is collapsed (icon strip). */
  collapsed?: boolean
}>()

defineEmits<{
  (e: 'open-citation', c: AssistantCitation): void
  (e: 'open-history'): void
  (e: 'toggle-collapse'): void
}>()

const { t } = useI18n()

// --- Reactive state ---------------------------------------------------
type Role = 'user' | 'assistant'
interface ChatMessage {
  id: string
  role: Role
  content: string
  citations: AssistantCitation[]
  modelName?: string
}

const messages = ref<ChatMessage[]>([])
const query = ref('')
const streaming = ref(false)
const error = ref('')
const lastLatencyMs = ref(0)
const abortController = ref<AbortController | null>(null)

const messagesEl = ref<HTMLElement | null>(null)
const inputEl = ref<HTMLTextAreaElement | null>(null)

const scopeLabel = computed(() => {
  const ids = props.kbIds?.length ? props.kbIds.join(', ') : t('assistant.scopeAll')
  return props.includeWiki
    ? t('assistant.scopeWithWiki', { ids })
    : t('assistant.scopeKbOnly', { ids })
})

const citationsCount = computed(() => {
  let n = 0
  for (const m of messages.value) {
    if (m.role === 'assistant') n += m.citations.length
  }
  return n
})

const canSend = computed(() => query.value.trim().length > 0 && !streaming.value)

// --- Helpers ----------------------------------------------------------
function autoGrow(e: Event) {
  const el = e.target as HTMLTextAreaElement
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 140) + 'px'
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesEl.value) {
      messagesEl.value.scrollTop = messagesEl.value.scrollHeight
    }
  })
}

function clearMessages() {
  if (streaming.value) abort()
  messages.value = []
  error.value = ''
}

let messageSeq = 0
function nextId(prefix: string): string {
  messageSeq += 1
  return `${prefix}-${Date.now()}-${messageSeq}`
}

/** Render assistant answer with [KB: ...] / [Wiki: ...] inline
 *  citation references hyperlinked to the citation card. */
function renderAnswer(text: string): string {
  if (!text) return ''
  // Escape HTML first to prevent injection from streamed content.
  const esc = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  // Match [KB: foo] / [Wiki: bar] inline tokens.
  return esc.replace(/\[(KB|Wiki):\s*([^\]]+)\]/g, (_m, kind, label) => {
    return `<span class="ai-assistant-panel__inline-cite" data-kind="${kind}">${kind}: ${label.trim()}</span>`
  })
}

// --- Submission + streaming ------------------------------------------
async function onSubmit() {
  if (!canSend.value) return
  await submit(query.value.trim())
}

async function submit(q: string) {
  error.value = ''
  messages.value.push({ id: nextId('user'), role: 'user', content: q, citations: [] })
  const assistantMsg: ChatMessage = {
    id: nextId('assistant'),
    role: 'assistant',
    content: '',
    citations: [],
    modelName: '',
  }
  messages.value.push(assistantMsg)
  query.value = ''
  scrollToBottom()

  const req: AssistantAskRequest = {
    query: q,
    source_kb_ids: props.kbIds,
    include_wiki: props.includeWiki ?? true,
    max_results_per_source: 5,
  }
  await runStream(req, assistantMsg)
}

async function retry() {
  // Find the last user message and resubmit.
  const lastUser = [...messages.value].reverse().find((m) => m.role === 'user')
  if (lastUser) {
    // Remove the failed assistant turn if any.
    messages.value = messages.value.filter(
      (m) => m.id !== messages.value[messages.value.length - 1].id || m.role === 'user',
    )
    await submit(lastUser.content)
  }
}

async function runStream(req: AssistantAskRequest, into: ChatMessage) {
  streaming.value = true
  const ctrl = new AbortController()
  abortController.value = ctrl
  const startedAt = Date.now()
  try {
    await askAssistantStream(
      req,
      (e: AssistantStreamEvent) => {
        switch (e.type) {
          case 'metadata':
            // conversation_id available; we keep our local id as the
            // turn id so the panel state stays self-contained.
            break
          case 'citation':
            into.citations.push(e.citation)
            scrollToBottom()
            break
          case 'token':
            into.content += e.text
            scrollToBottom()
            break
          case 'done':
            into.modelName = `tokens ${e.promptTokens}/${e.completionTokens}`
            lastLatencyMs.value = Date.now() - startedAt
            break
          case 'error':
            error.value = e.error
            break
        }
      },
      ctrl.signal,
    )
  } catch (err) {
    error.value = (err as Error)?.message || 'stream failed'
  } finally {
    streaming.value = false
    abortController.value = null
  }
}

function abort() {
  if (abortController.value) {
    abortController.value.abort()
    streaming.value = false
    abortController.value = null
  }
}

onUnmounted(() => {
  abort()
})
</script>

<style scoped>
.ai-assistant-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 420px;
  background: var(--app-surface-bg);
  border-left: 1px solid var(--app-border);
  font-size: 14px;
  transition: width 0.15s ease;
}
.ai-assistant-panel.is-collapsed {
  width: 0;
  border-left: none;
  overflow: hidden;
}
.ai-assistant-panel__header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border);
}
.ai-assistant-panel__brand {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
}
.ai-assistant-panel__logo {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: linear-gradient(135deg, #2d6bf7, #10c2a8);
  color: #fff;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}
.ai-assistant-panel__title {
  font-weight: 600;
  font-size: 14px;
}
.ai-assistant-panel__subtitle {
  font-size: 12px;
  color: var(--app-text-muted);
}
.ai-assistant-panel__actions {
  display: flex;
  gap: 4px;
}
.ai-assistant-panel__btn {
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  padding: 5px 7px;
  cursor: pointer;
  color: var(--app-text-muted);
}
.ai-assistant-panel__btn:hover:not(:disabled) {
  background: var(--app-surface-raised);
  color: #1a2438;
}
.ai-assistant-panel__btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.ai-assistant-panel__messages {
  flex: 1;
  overflow-y: auto;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.ai-assistant-panel__msg {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ai-assistant-panel__msg.is-user {
  align-items: flex-end;
}
.ai-assistant-panel__msg-bubble {
  padding: 9px 12px;
  border-radius: 12px;
  background: #f0f4fa;
  max-width: 86%;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}
.is-user .ai-assistant-panel__msg-bubble {
  background: var(--app-brand);
  color: #fff;
}
.ai-assistant-panel__msg-label {
  font-size: 11px;
  color: var(--app-text-muted);
  display: flex;
  align-items: center;
  gap: 4px;
}
.ai-assistant-panel__msg-model {
  margin-left: 6px;
  padding: 1px 6px;
  background: var(--app-surface-raised);
  border-radius: 99px;
  font-size: 10px;
}
.ai-assistant-panel__inline-cite {
  background: #fff7e6;
  color: #f39c12;
  padding: 1px 4px;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
}

.ai-assistant-panel__citations {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}
.ai-assistant-panel__citation {
  display: flex;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--app-border);
  border-radius: 8px;
  cursor: pointer;
  background: var(--app-surface-raised);
}
.ai-assistant-panel__citation:hover {
  border-color: var(--app-brand);
  background: color-mix(in srgb, var(--app-brand) 12%, var(--app-surface-bg));
}
.ai-assistant-panel__citation-kind {
  flex-shrink: 0;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
  align-self: start;
}
.ai-assistant-panel__citation-kind.is-kb {
  background: #e5e0fb;
  color: #6c5ce7;
}
.ai-assistant-panel__citation-kind.is-wiki {
  background: #d6f4eb;
  color: #00b894;
}
.ai-assistant-panel__citation-title {
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 2px;
}
.ai-assistant-panel__citation-snippet {
  font-size: 12px;
  color: var(--app-text-muted);
  line-height: 1.5;
}

.ai-assistant-panel__error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: #fff1f0;
  border: 1px solid #ffccc7;
  border-radius: 8px;
  color: #cf1322;
  font-size: 13px;
}

.ai-assistant-panel__composer {
  border-top: 1px solid var(--app-border);
  padding: 12px 16px 14px;
  background: var(--app-surface-raised);
}
.ai-assistant-panel__hints {
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: var(--app-text-muted);
  margin-bottom: 8px;
}
.ai-assistant-panel__hint {
  display: flex;
  align-items: center;
  gap: 4px;
}
.ai-assistant-panel__form {
  display: flex;
  gap: 8px;
  align-items: flex-end;
}
.ai-assistant-panel__input {
  flex: 1;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  padding: 8px 10px;
  font: inherit;
  resize: none;
  min-height: 36px;
  max-height: 140px;
  outline: none;
  background: var(--app-control-bg);
}
.ai-assistant-panel__input:focus {
  border-color: var(--app-brand);
}
.ai-assistant-panel__send {
  padding: 8px 14px;
  border-radius: 8px;
  border: none;
  background: var(--app-brand);
  color: #fff;
  font-weight: 600;
  cursor: pointer;
}
.ai-assistant-panel__send.is-stop {
  background: #f5222d;
}
.ai-assistant-panel__send:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
