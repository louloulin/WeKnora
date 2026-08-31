<!--
  CollabAiPolishDialog — v0.7.26 paragraph-level AI润色 popover for the
  CollabDocProEditor.

  Flow:
    1. User selects a paragraph (or the entire doc), clicks "问 AI".
    2. We POST to /chat/agent-chat with the selected text + a "polish"
       instruction; the chat endpoint streams the reply.
    3. The popover shows the original vs the candidate, with "接受"/"取消"
       buttons. Accept writes the candidate back to the editor and
       triggers the same patchParagraphText path used for normal edits.

  The agent backend is the same one the chat panel uses, so any model
  configured in WeKnora is available. The /chat endpoint is wired
  unconditionally — the docx-engine knowledge sync is decoupled from
  this surface.
-->
<template>
  <Teleport to="body">
    <div v-if="open" class="collab-ai-popover" :style="popoverStyle">
      <div class="collab-ai-popover__header">
        <span>AI 润色</span>
        <button class="collab-ai-popover__close" @click="onClose" type="button">×</button>
      </div>
      <div class="collab-ai-popover__body">
        <div class="collab-ai-popover__col">
          <h4>原文</h4>
          <pre class="collab-ai-popover__orig">{{ original }}</pre>
        </div>
        <div class="collab-ai-popover__col">
          <h4>AI 建议</h4>
          <pre v-if="streaming" class="collab-ai-popover__cand streaming">{{ streamedText || '思考中…' }}</pre>
          <pre v-else-if="error" class="collab-ai-popover__cand error">{{ error }}</pre>
          <pre v-else class="collab-ai-popover__cand">{{ streamedText }}</pre>
        </div>
      </div>
      <div class="collab-ai-popover__footer">
        <button class="collab-ai-popover__btn" @click="onClose" type="button">取消</button>
        <button class="collab-ai-popover__btn primary" :disabled="!streamedText || streaming" @click="onAccept" type="button">
          {{ streaming ? '生成中...' : '接受并替换' }}
        </button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{
  open: boolean
  anchor: { x: number; y: number }
  original: string
  kbId?: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'accept', replacement: string): void
}>()

const streamedText = ref('')
const streaming = ref(false)
const error = ref<string | null>(null)
let abortCtrl: AbortController | null = null

const popoverStyle = computed(() => ({
  left: `${Math.min(props.anchor.x, window.innerWidth - 520)}px`,
  top: `${Math.min(props.anchor.y, window.innerHeight - 360)}px`,
}))

const reset = () => {
  streamedText.value = ''
  streaming.value = false
  error.value = null
}

const onClose = () => {
  if (abortCtrl) {
    abortCtrl.abort()
    abortCtrl = null
  }
  emit('close')
}

const onAccept = () => {
  emit('accept', streamedText.value.trim())
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      reset()
      void runPolish()
    } else if (abortCtrl) {
      abortCtrl.abort()
      abortCtrl = null
    }
  },
)

const runPolish = async () => {
  streaming.value = true
  error.value = null
  abortCtrl = new AbortController()
  try {
    // Use the agent chat endpoint. We POST a JSON body asking for a
    // streamed reply. The body shape mirrors the chat panel so any
    // WeKnora-deployed model is reachable.
    const url = '/api/v1/chat/agent-chat/stream'
    const payload = {
      query: `请润色以下中文段落，使其表达更流畅、语气更专业，保持原意不变：\n\n${props.original}\n\n仅返回润色后的文本，不要添加解释。`,
      kb_id: props.kbId || '',
      stream: true,
    }
    const resp = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('weknora_token') || ''}`,
      },
      body: JSON.stringify(payload),
      signal: abortCtrl.signal,
    })
    if (!resp.ok || !resp.body) {
      throw new Error(`AI 请求失败: ${resp.status}`)
    }
    const reader = resp.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buf = ''
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buf += decoder.decode(value, { stream: true })
      let idx: number
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        const frame = buf.slice(0, idx)
        buf = buf.slice(idx + 2)
        const line = frame.split('\n').find((l) => l.startsWith('data:'))
        if (!line) continue
        const data = line.slice(5).trim()
        if (!data || data === '[DONE]') continue
        try {
          const json = JSON.parse(data)
          const token =
            (typeof json.content === 'string' && json.content) ||
            (typeof json.delta === 'string' && json.delta) ||
            (typeof json.text === 'string' && json.text) ||
            ''
          if (token) streamedText.value += token
        } catch {
          // ignore malformed frame
        }
      }
    }
  } catch (e: any) {
    if (e?.name === 'AbortError') return
    error.value = e?.message || 'AI 请求出错'
  } finally {
    streaming.value = false
    abortCtrl = null
  }
}

onBeforeUnmount(() => {
  if (abortCtrl) abortCtrl.abort()
})
</script>

<style scoped>
.collab-ai-popover {
  position: fixed; z-index: 9999;
  width: 480px; max-width: calc(100vw - 32px);
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #dcdcdc);
  border-radius: 8px;
  box-shadow: 0 12px 32px rgba(0,0,0,0.18);
  display: flex; flex-direction: column;
  max-height: 360px;
}
.collab-ai-popover__header { display: flex; align-items: center; padding: 8px 12px; border-bottom: 1px solid var(--td-component-stroke, #dcdcdc); font-weight: 600; }
.collab-ai-popover__close { margin-left: auto; background: transparent; border: none; cursor: pointer; font-size: 18px; }
.collab-ai-popover__body { display: flex; gap: 12px; padding: 12px; flex: 1; min-height: 0; }
.collab-ai-popover__col { flex: 1; min-width: 0; }
.collab-ai-popover__col h4 { margin: 0 0 6px; font-size: 12px; color: var(--td-text-color-secondary, #666); }
.collab-ai-popover__orig, .collab-ai-popover__cand {
  margin: 0; padding: 8px;
  background: var(--td-bg-color-secondarycontainer, #f5f5f5);
  border-radius: 4px;
  font-size: 13px; line-height: 1.5; white-space: pre-wrap;
  max-height: 200px; overflow: auto;
}
.collab-ai-popover__cand.streaming { background: var(--td-brand-color-1, #e6f3ff); }
.collab-ai-popover__cand.error { background: var(--td-error-color-1, #fde8e8); color: var(--td-error-color-7, #c00); }
.collab-ai-popover__footer { display: flex; justify-content: flex-end; gap: 8px; padding: 8px 12px; border-top: 1px solid var(--td-component-stroke, #dcdcdc); }
.collab-ai-popover__btn { padding: 4px 12px; border: 1px solid var(--td-component-stroke, #dcdcdc); background: transparent; border-radius: 4px; cursor: pointer; }
.collab-ai-popover__btn.primary { background: var(--td-brand-color-7, #2b6cb0); color: #fff; border-color: var(--td-brand-color-7, #2b6cb0); }
.collab-ai-popover__btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
