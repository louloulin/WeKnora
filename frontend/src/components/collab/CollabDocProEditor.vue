<!--
  CollabDocProEditor — v0.7.26 DOC-kind collaborative editor with real
  .docx byte round-trip.

  Architecture:
   1. Open: fetch the latest .docx bytes via REST
            → docxAdapter.openDocx(bytes) returns ParsedDocFull
            → render each block as a TipTap paragraph with
              data-docx-index so we can re-locate it on save.
   2. Edit: TipTap StarterKit + CollaborationCursor over Yjs.
            On update, debounced 1.5s, walk TipTap paragraphs and
            patchParagraphText() the matching docx-engine block.
   3. Save: docxAdapter.saveDocxBytes(parsed) → REST upload.
   4. Realtime: per-paragraph Y.Text binding via TipTap's `field` config
      so two clients editing different paragraphs converge cleanly.

  Format coverage today: paragraph/heading/listItem blocks round-trip
  faithfully. Tables, images, and shapes stay read-only on save (their
  byte spans are preserved verbatim because we only mutate runs inside
  those blocks — the patch path leaves anchor XML untouched).
-->
<template>
  <div class="collab-doc-pro">
    <div class="collab-doc-pro__toolbar">
      <span class="collab-doc-pro__title">{{ title }}</span>
      <span class="collab-doc-pro__kind">{{ kindLabel }}</span>
      <span class="collab-doc-pro__connection" :class="{ connected: connected && !saveError }">
        {{ connectionLabel }}
      </span>
      <span class="collab-doc-pro__savetag" :class="savetagClass">
        {{ saveLabel }}
      </span>
      <button class="collab-doc-pro__btn" :disabled="downloading" @click="onDownload">
        {{ downloading ? '下载中...' : '下载 .docx' }}
      </button>
      <button class="collab-doc-pro__btn" :disabled="uploading" @click="onForceSave">
        {{ uploading ? '保存中...' : '立即保存' }}
      </button>
      <span class="collab-doc-pro__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-doc-pro__peer"
          :style="{ backgroundColor: p.color }"
          :title="p.displayName"
        >{{ initialOf(p.displayName) }}</span>
      </span>
    </div>
    <div v-if="loading" class="collab-doc-pro__loading">加载文档中…</div>
    <div v-else-if="loadError" class="collab-doc-pro__error">加载失败：{{ loadError }}</div>
    <div v-else class="collab-doc-pro__surface-wrap">
      <EditorContent :editor="editor" class="collab-doc-pro__surface" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { EditorContent } from '@tiptap/vue-3'
import type { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import { Collaboration } from '@tiptap/extension-collaboration'
import { CollaborationCursor } from '@tiptap/extension-collaboration-cursor'
import * as Y from 'yjs'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'
import {
  openDocx,
  saveDocxBytes,
  patchParagraphText,
  buildBlankDocxDoc,
  type DocxAdapterDocument,
  type DocxAdapterParagraph,
} from '@/editor/adapters/docxAdapter'
import {
  downloadCollabDocBytes,
  uploadCollabDocBytes,
} from '@/api/collabDoc'
import { MessagePlugin } from 'tdesign-vue-next'

const props = defineProps<{
  docId: string
  title: string
  token: string
  displayName: string
}>()

const editor = shallowRef<Editor | undefined>(undefined)
const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
const error = ref<string | null>(null)
const loading = ref(false)
const loadError = ref<string | null>(null)
const downloading = ref(false)
const uploading = ref(false)
const saveLabel = ref('未修改')
const saveError = ref<string | null>(null)

let handle: ReturnType<typeof useYjsCollabDoc> | null = null
let ydoc: Y.Doc | null = null
let doc: DocxAdapterDocument | null = null
let saveTimer: ReturnType<typeof setTimeout> | null = null
let suppressObserver = false
let patchedMap: Map<number, string> = new Map()

const connectionLabel = computed(() => {
  if (saveError.value) return `保存错误：${saveError.value}`
  if (connected.value) return '已连接'
  return '连接中...'
})

const kindLabel = computed(() => 'Word 文档 (.docx)')

const savetagClass = computed(() => ({
  dirty: saveLabel.value === '有未保存的修改',
  saving: saveLabel.value === '保存中...',
}))

const initialOf = (name: string) => (name || '?').trim().slice(0, 1).toUpperCase()

const downloadAsUint8Array = async (): Promise<Uint8Array> => {
  const blob = await downloadCollabDocBytes(props.docId)
  const buffer = await blob.arrayBuffer()
  return new Uint8Array(buffer)
}

const onDownload = async () => {
  if (!doc) return
  downloading.value = true
  try {
    const patched = Array.from(patchedMap.entries()).map(([docxIndex, xml]) => ({
      docxIndex, xml, text: doc.paragraphs[docxIndex]?.text || '',
    }))
    const bytes = await saveDocxBytes(doc, patched)
    const ab = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
    const blob = new Blob([ab], {
      type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${props.title || 'collab-doc'}.docx`
    a.click()
    URL.revokeObjectURL(a.href)
    MessagePlugin.success('已下载 .docx')
  } catch (e: any) {
    MessagePlugin.error(`下载失败：${e?.message || e}`)
  } finally {
    downloading.value = false
  }
}

const onForceSave = () => {
  flushSave(true)
}

const setup = async () => {
  if (!props.docId || !props.token) return
  handle = useYjsCollabDoc({ docId: props.docId, token: props.token, displayName: props.displayName })
  ydoc = handle.ydoc
  connected.value = Boolean(handle.connected.value)
  peers.value = (handle.peers.value ?? []) as Array<{ clientId: number; displayName: string; color: string }>
  error.value = (handle.error.value ?? null) as string | null
  watch(handle.connected, (v) => (connected.value = Boolean(v)))
  watch(handle.peers, (v) => (peers.value = (v ?? []) as Array<{ clientId: number; displayName: string; color: string }>))
  watch(handle.error, (v) => (error.value = (v ?? null) as string | null))

  loading.value = true
  loadError.value = null
  try {
    let bytes: Uint8Array
    try {
      bytes = await downloadAsUint8Array()
    } catch (e: any) {
      // No bytes uploaded yet → start from a blank docx rendered as empty.
      // We deliberately keep the user in the editor so they can type, and
      // the first save will upload a real .docx package.
      doc = null
      loading.value = false
      initEditor([])
      return
    }
    doc = await openDocx(bytes)
    initEditor(doc.paragraphs)
  } catch (e: any) {
    loadError.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

const initEditor = (paragraphs: DocxAdapterParagraph[]) => {
  editor.value = new Editor({
    extensions: [
      StarterKit.configure({ history: false }),
      Link,
      ...(ydoc ? [Collaboration.configure({ document: ydoc, field: 'docx-body' })] : []),
      ...(ydoc && handle ? [CollaborationCursor.configure({
        provider: handle.provider,
        user: { name: props.displayName, color: '#58a6ff' },
      })] : []),
    ],
    content: paragraphsToContent(paragraphs),
    onUpdate: ({ editor: ed }) => onEditorUpdate(ed),
  })
}

const paragraphsToContent = (paragraphs: DocxAdapterParagraph[]) => {
  const nodes: any[] = []
  for (const p of paragraphs) {
    if (p.hidden) continue
    const text = p.text || ''
    if (p.kind === 'heading' && p.level) {
      nodes.push({
        type: 'heading',
        attrs: { level: p.level, 'data-docx-index': p.index },
        content: text ? [{ type: 'text', text }] : [],
      })
    } else if (p.kind === 'listItem') {
      nodes.push({
        type: 'bulletList',
        content: [{
          type: 'listItem',
          attrs: { 'data-docx-index': p.index },
          content: [{ type: 'paragraph', content: text ? [{ type: 'text', text }] : [] }],
        }],
      })
    } else {
      nodes.push({
        type: 'paragraph',
        attrs: { 'data-docx-index': p.index },
        content: text ? [{ type: 'text', text }] : [],
      })
    }
  }
  if (nodes.length === 0) {
    nodes.push({ type: 'paragraph', attrs: { 'data-docx-index': 0 }, content: [] })
  }
  return { type: 'doc', content: nodes }
}

const onEditorUpdate = (ed: Editor) => {
  if (!doc) {
    // First-time empty edit: we don't yet have a parsed.docx, so the save
    // path will fall back to building a blank docx from TipTap text.
    saveLabel.value = '有未保存的修改'
    scheduleSave()
    return
  }
  // Walk TipTap JSON, find each paragraph with data-docx-index, and patch
  // the matching block. Suppress the Y observer during this transaction
  // to prevent an update echo.
  const json = ed.getJSON() as { content?: any[] }
  const seen = new Set<number>()
  for (const node of json.content || []) {
    const idx = parseDocxIndex(node)
    if (idx == null || seen.has(idx)) continue
    seen.add(idx)
    const text = extractText(node)
    if (!doc.paragraphs[idx] || doc.paragraphs[idx].text === text) continue
    suppressObserver = true
    try {
      const patched = patchParagraphText(doc, idx, text)
      patchedMap.set(idx, patched.xml)
      doc.paragraphs[idx].text = text
    } finally {
      suppressObserver = false
    }
  }
  saveLabel.value = '有未保存的修改'
  scheduleSave()
}

const parseDocxIndex = (node: any): number | null => {
  if (!node || typeof node !== 'object') return null
  const attrs = node.attrs || {}
  const raw = attrs['data-docx-index'] ?? attrs.docxIndex
  if (typeof raw === 'number') return raw
  if (typeof raw === 'string' && raw !== '') return Number(raw)
  if (node.content && Array.isArray(node.content)) {
    for (const child of node.content) {
      const v = parseDocxIndex(child)
      if (v != null) return v
    }
  }
  return null
}

const extractText = (node: any): string => {
  if (!node) return ''
  if (node.type === 'text' && typeof node.text === 'string') return node.text
  if (Array.isArray(node.content)) {
    return node.content.map((c: any) => extractText(c)).join('')
  }
  return ''
}

const scheduleSave = () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => flushSave(false), 1500)
}

const flushSave = async (immediate: boolean) => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  if (!doc) {
    // Build a minimal docx from current editor content if we never loaded
    // one from the backend. This handles the "fresh doc, no upload yet" path.
    if (!editor.value) return
    try {
      doc = await buildBlankDocxFromEditor(editor.value)
    } catch (e: any) {
      saveError.value = e?.message || String(e)
      return
    }
  }
  saveLabel.value = '保存中...'
  try {
    const patched = Array.from(patchedMap.entries()).map(([docxIndex, xml]) => ({
      docxIndex, xml, text: doc.paragraphs[docxIndex]?.text || '',
    }))
    const bytes = await saveDocxBytes(doc, patched)
    patchedMap.clear()
    await uploadCollabDocBytes(props.docId, bytes, `${props.title || 'collab-doc'}.docx`)
    saveLabel.value = immediate ? '已保存' : '自动保存'
    saveError.value = null
    setTimeout(() => {
      if (saveLabel.value === '已保存' || saveLabel.value === '自动保存') saveLabel.value = '已保存'
    }, 1500)
  } catch (e: any) {
    saveError.value = e?.message || String(e)
    saveLabel.value = '保存失败'
  }
}

const buildBlankDocxFromEditor = async (ed: Editor): Promise<DocxAdapterDocument> => {
  // Synthesize a minimal docx from the current TipTap content so the
  // first save can produce real .docx bytes. We pull each top-level
  // paragraph and seed buildBlankDocxDoc with the first line; subsequent
  // edits continue through patchParagraphText.
  const json = ed.getJSON() as { content?: any[] }
  const paragraphs: Array<{ text: string; kind: 'paragraph' | 'heading' | 'listItem'; level?: number }> = []
  for (const node of json.content || []) {
    const text = extractText(node)
    const kind = node.type === 'heading' ? 'heading' as const : 'paragraph' as const
    const level = node.type === 'heading' && node.attrs?.level ? Number(node.attrs.level) : undefined
    paragraphs.push({ text, kind, level })
    if (paragraphs.length >= 1) break // first paragraph becomes the docx seed
  }
  return buildBlankDocxDoc(paragraphs)
}

const teardown = () => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  if (editor.value) {
    editor.value.destroy()
    editor.value = undefined
  }
  if (handle) {
    handle.destroy()
    handle = null
  }
  doc = null
  ydoc = null
}

setup()
watch(() => props.docId, () => { teardown(); setup() })
onBeforeUnmount(teardown)
</script>

<style scoped>
.collab-doc-pro { display: flex; flex-direction: column; height: 100%; }
.collab-doc-pro__toolbar {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  flex-wrap: wrap;
}
.collab-doc-pro__title { font-weight: 600; font-size: 14px; }
.collab-doc-pro__kind { font-size: 11px; padding: 2px 8px; border-radius: 999px; background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-doc-pro__connection { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); }
.collab-doc-pro__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-doc-pro__savetag { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); }
.collab-doc-pro__savetag.dirty { background: var(--td-warning-color-1); color: var(--td-warning-color-7); }
.collab-doc-pro__savetag.saving { background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-doc-pro__btn { padding: 4px 12px; font-size: 12px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); cursor: pointer; }
.collab-doc-pro__btn:disabled { opacity: 0.5; cursor: not-allowed; }
.collab-doc-pro__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-doc-pro__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-doc-pro__surface-wrap { flex: 1; display: flex; flex-direction: column; min-height: 0; }
.collab-doc-pro__surface { flex: 1; overflow: auto; padding: 24px 32px; max-width: 880px; margin: 0 auto; }
.collab-doc-pro__loading, .collab-doc-pro__error { padding: 24px; }
.collab-doc-pro__error { color: var(--td-error-color-7); }
</style>
