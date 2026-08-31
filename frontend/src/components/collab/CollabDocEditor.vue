<!--
  CollabDocEditor — v0.7.25 DOC-kind collaborative editor.
  Wires a TipTap editor to the shared Yjs doc via the useYjsCollabDoc
  composable. Same TipTap stack as WikiTiptapRealtime; the only difference
  is the Yjs provider URL and the Y.Doc namespace ("collab-doc-...").
-->
<template>
  <div class="collab-doc-editor">
    <div class="collab-doc-editor__toolbar">
      <span class="collab-doc-editor__title">{{ title }}</span>
      <span class="collab-doc-editor__connection" :class="{ connected }">
        {{ connected ? '已连接' : '连接中...' }}
      </span>
      <span class="collab-doc-editor__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-doc-editor__peer"
          :style="{ backgroundColor: p.color }"
          :title="p.displayName"
        >{{ initialOf(p.displayName) }}</span>
      </span>
    </div>
    <EditorContent :editor="editor" class="collab-doc-editor__surface" />
    <p v-if="error" class="collab-doc-editor__error">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { Editor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import { Collaboration } from '@tiptap/extension-collaboration'
import { CollaborationCursor } from '@tiptap/extension-collaboration-cursor'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'

const props = defineProps<{
  docId: string
  title: string
  token: string
  displayName: string
}>()

const editor = shallowRef<Editor | null>(null)
const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
const error = ref<string | null>(null)

let handle: ReturnType<typeof useYjsCollabDoc> | null = null

const setup = () => {
  if (!props.docId || !props.token) return
  handle = useYjsCollabDoc({ docId: props.docId, token: props.token, displayName: props.displayName })
  connected.value = handle.connected.value
  peers.value = handle.peers.value
  error.value = handle.error.value
  watch(handle.connected, (v) => (connected.value = v))
  watch(handle.peers, (v) => (peers.value = v))
  watch(handle.error, (v) => (error.value = v))
  editor.value = new Editor({
    extensions: [
      StarterKit.configure({ history: false }),
      Link,
      Collaboration.configure({ document: handle.ydoc }),
      CollaborationCursor.configure({ provider: handle.provider, user: { name: props.displayName, color: '#58a6ff' } }),
    ],
  })
}

const teardown = () => {
  if (editor.value) {
    editor.value.destroy()
    editor.value = null
  }
  if (handle) {
    handle.destroy()
    handle = null
  }
}

setup()
watch(() => props.docId, () => { teardown(); setup() })
onBeforeUnmount(teardown)

const initialOf = (name: string) => (name || '?').trim().slice(0, 1).toUpperCase()
</script>

<style scoped>
.collab-doc-editor { display: flex; flex-direction: column; height: 100%; }
.collab-doc-editor__toolbar { display: flex; align-items: center; gap: 12px; padding: 8px 12px; border-bottom: 1px solid var(--td-component-stroke); background: var(--td-bg-color-container); }
.collab-doc-editor__title { font-weight: 600; font-size: 14px; }
.collab-doc-editor__connection { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); }
.collab-doc-editor__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-doc-editor__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-doc-editor__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-doc-editor__surface { flex: 1; overflow: auto; padding: 16px 24px; }
.collab-doc-editor__error { color: var(--td-error-color-7); padding: 8px 12px; }
</style>
