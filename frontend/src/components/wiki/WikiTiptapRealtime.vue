<!--
  WikiTiptapRealtime — v0.7.19 Tiptap editor with Yjs realtime collaboration.

  This wraps the existing WikiTiptapEditor with realtime sync. When realtime
  is enabled (default), edits propagate to all peers via Yjs CRDT; when
  disabled (the feature flag), falls back to local-only editing.
-->
<template>
  <div class="wiki-tiptap-realtime">
    <div class="wiki-tiptap-realtime-toolbar">
      <WikiPresenceBar
        :peers="peers"
        :connected="connected"
        :error="error"
        class="wiki-tiptap-realtime-presence"
      />
      <button
        type="button"
        class="wiki-tiptap-realtime-toggle"
        :aria-pressed="realtimeEnabled"
        @click="toggleRealtime"
      >
        {{ realtimeEnabled ? 'Realtime: ON' : 'Realtime: OFF' }}
      </button>
    </div>
    <WikiTiptapEditor
      v-if="!realtimeEnabled"
      v-bind="$attrs"
      :model-value="localContent"
      @update:model-value="onLocalUpdate"
    />
    <div v-else class="wiki-tiptap-realtime-host" ref="hostEl" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import WikiTiptapEditor from './WikiTiptapEditor.vue'
import WikiPresenceBar from './WikiPresenceBar.vue'
import { useYjsWiki } from '../../composables/useYjsWiki'

const props = defineProps<{
  kbId: string
  pageId: string
  token: string
  displayName: string
  initialContent?: string
}>()

const emit = defineEmits<{
  (e: 'realtime-error', message: string): void
}>()

const hostEl = ref<HTMLDivElement | null>(null)
const realtimeEnabled = ref(true)
const localContent = ref(props.initialContent || '')

const handle = shallowRef<ReturnType<typeof useYjsWiki> | null>(null)
const editor = handle.value ? handle.value.editor : shallowRef(null)
const connected = handle.value ? handle.value.connected : ref(false)
const peers = handle.value ? handle.value.peers : ref([])
const error = handle.value ? handle.value.error : ref<string | null>(null)

const mount = () => {
  if (!hostEl.value) return
  const h = useYjsWiki({
    kbId: props.kbId,
    pageId: props.pageId,
    token: props.token,
    displayName: props.displayName,
    initialContent: props.initialContent,
  })
  handle.value = h
  // Mount the Tiptap editor into our host div once it's available.
  watch(h.editor, (ed) => {
    if (ed && hostEl.value) {
      // Tiptap attaches to its parent; we replace the host content with the editor element.
      hostEl.value.innerHTML = ''
      ed.options.element = hostEl.value
      // Force re-mount by re-creating the view via destroy + new Editor is heavy;
      // instead, append the editor's view DOM.
      const view = (ed as unknown as { view: { dom: HTMLElement } }).view.dom
      hostEl.value.appendChild(view)
    }
  }, { immediate: true })
}

const unmount = () => {
  handle.value?.destroy()
  handle.value = null
}

const toggleRealtime = () => {
  realtimeEnabled.value = !realtimeEnabled.value
  if (realtimeEnabled.value) {
    mount()
  } else {
    unmount()
  }
}

const onLocalUpdate = (value: string) => {
  localContent.value = value
}

watch(() => props.token, () => {
  if (realtimeEnabled.value) {
    unmount()
    mount()
  }
})

watch(error, (msg) => {
  if (msg) emit('realtime-error', msg)
})

onMounted(() => {
  if (realtimeEnabled.value) mount()
})

onBeforeUnmount(() => {
  unmount()
})

defineOptions({ inheritAttrs: false })
</script>

<style scoped>
.wiki-tiptap-realtime { display: flex; flex-direction: column; gap: 8px; }
.wiki-tiptap-realtime-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.wiki-presence-realtime-presence { flex: 1; }
.wiki-tiptap-realtime-toggle {
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid var(--color-border, #30363d);
  background: var(--color-bg-elevated, #161b22);
  color: var(--color-text, #e6edf3);
  font-size: 12px;
  cursor: pointer;
}
.wiki-tiptap-realtime-toggle[aria-pressed="true"] {
  background: rgba(63, 185, 80, .15);
  border-color: #3fb950;
  color: #3fb950;
}
.wiki-tiptap-realtime-host {
  min-height: 200px;
  border: 1px solid var(--color-border, #30363d);
  border-radius: 8px;
  padding: 12px;
  background: var(--color-bg, #0e1117);
}
</style>
