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
      <button class="collab-doc-pro__btn collab-doc-pro__btn--ai" :disabled="!aiOriginal" @click="onOpenAi" type="button">
        问 AI
      </button>
      <button class="collab-doc-pro__btn" :disabled="!doc" @click="onToggleHistory" type="button">
        {{ historyOpen ? '关闭历史' : '版本历史' }}
      </button>
      <span class="collab-doc-pro__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-doc-pro__peer"
          :class="{ 'collab-doc-pro__peer--selecting': remoteSelectionFor(p.clientId) }"
          :style="{ backgroundColor: p.color }"
          :title="peerTitle(p)"
        >{{ initialOf(p.displayName) }}</span>
      </span>
    </div>
    <div v-if="loading" class="collab-doc-pro__loading">加载文档中…</div>
    <div v-else-if="loadError" class="collab-doc-pro__error">加载失败：{{ loadError }}</div>
    <template v-else>
      <div class="collab-doc-pro__formatbar" v-if="editor">
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('bold') }" @click="runMark('bold')" type="button" title="粗体 (Ctrl+B)"><b>B</b></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('italic') }" @click="runMark('italic')" type="button" title="斜体 (Ctrl+I)"><i>I</i></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('strike') }" @click="runMark('strike')" type="button" title="删除线"><s>S</s></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('code') }" @click="runMark('code')" type="button" title="行内代码"><code>&lt;/&gt;</code></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('underline') }" @click="runMark('underline')" type="button" title="下划线"><u>U</u></button>
        <span class="collab-doc-pro__sep"></span>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('heading', { level: 1 }) }" @click="runHeading(1)" type="button" title="一级标题">H1</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('heading', { level: 2 }) }" @click="runHeading(2)" type="button" title="二级标题">H2</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('heading', { level: 3 }) }" @click="runHeading(3)" type="button" title="三级标题">H3</button>
        <span class="collab-doc-pro__sep"></span>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('bulletList') }" @click="runNode('toggleBulletList')" type="button" title="无序列表">• 列表</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('orderedList') }" @click="runNode('toggleOrderedList')" type="button" title="有序列表">1. 列表</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('taskList') }" @click="runNode('toggleTaskList')" type="button" title="任务列表">☑</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('blockquote') }" @click="runNode('toggleBlockquote')" type="button" title="引用">❝</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('codeBlock') }" @click="runNode('toggleCodeBlock')" type="button" title="代码块">{}</button>
        <span class="collab-doc-pro__sep"></span>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('link') }" @click="onSetLink" type="button" title="链接">🔗</button>
        <button class="collab-doc-pro__fmt" @click="onInsertTable" type="button" title="插入表格">⊞</button>
        <button class="collab-doc-pro__fmt" @click="onInsertImageUrl" type="button" title="插入图片">🖼</button>
      <button class="collab-doc-pro__btn" @click="onInsertImageFile" type="button" title="插入本地图片">📁 图片</button>
      <input ref="fileImageInput" type="file" accept="image/*" style="display:none" @change="onImageFileChosen" />
        <button class="collab-doc-pro__fmt" @click="onAlign('left')" type="button" title="左对齐">⇤</button>
        <button class="collab-doc-pro__fmt" @click="onAlign('center')" type="button" title="居中">≡</button>
        <button class="collab-doc-pro__fmt" @click="onAlign('right')" type="button" title="右对齐">⇥</button>
        <span class="collab-doc-pro__sep"></span>
        <button class="collab-doc-pro__fmt" @click="runNode('undo')" type="button" title="撤销 (Ctrl+Z)">↶</button>
        <button class="collab-doc-pro__fmt" @click="runNode('redo')" type="button" title="重做 (Ctrl+Y)">↷</button>
      </div>
      <div class="collab-doc-pro__surface-wrap">
      <EditorContent :editor="editor" class="collab-doc-pro__surface" />
      <CollabAiPolishDialog
        v-if="aiOpen"
        :open="aiOpen"
        :anchor="aiAnchor"
        :original="aiOriginal"
        @close="aiOpen = false"
        @accept="onAcceptAi"
      />
      <div v-if="historyOpen" class="collab-doc-pro__history">
        <div class="collab-doc-pro__history-head">版本历史</div>
        <div v-if="versions.length === 0" class="collab-doc-pro__history-empty">暂无历史版本</div>
        <div v-for="v in versions" :key="v.version" class="collab-doc-pro__history-row">
          <div class="collab-doc-pro__history-meta">
            <strong>v{{ v.version }}</strong>
            <span>{{ formatBytes(v.size_bytes) }}</span>
            <span>{{ formatTime(v.created_at) }}</span>
          </div>
          <div class="collab-doc-pro__history-actions">
            <button class="collab-doc-pro__btn" @click="onDownloadVersion(v.version)">下载</button>
            <button class="collab-doc-pro__btn primary" @click="onRestoreVersion(v.version)">恢复</button>
          </div>
        </div>
      </div>
    </div>
    </template>
    <!-- v0.7.29 — comments side panel -->
    <CollabCommentsPanel
      :doc-id="docId"
      :token="token"
      :anchor="commentAnchor"
      anchor-label="段落选区"
      placeholder="对选中的段落添加评论…"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { Editor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Table from '@tiptap/extension-table'
import TableRow from '@tiptap/extension-table-row'
import TableCell from '@tiptap/extension-table-cell'
import TableHeader from '@tiptap/extension-table-header'
import Image from '@tiptap/extension-image'
import TaskList from '@tiptap/extension-task-list'
import TaskItem from '@tiptap/extension-task-item'
import Underline from '@tiptap/extension-underline'
import TextAlign from '@tiptap/extension-text-align'
import Highlight from '@tiptap/extension-highlight'
import Color from '@tiptap/extension-color'
import { Collaboration } from '@tiptap/extension-collaboration'
import { CollaborationCursor } from '@tiptap/extension-collaboration-cursor'
import * as Y from 'yjs'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'
import CollabCommentsPanel from '@/components/collab/CollabCommentsPanel.vue'
import {
  openDocx,
  saveDocxBytes,
  saveDocxBytesWithImages,
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
import CollabAiPolishDialog from './CollabAiPolishDialog.vue'

const props = defineProps<{
  docId: string
  title: string
  token: string
  displayName: string
}>()

const editor = shallowRef<Editor | undefined>(undefined)
const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string; selection?: { from: number; to: number } | null }>>([])
const error = ref<string | null>(null)
const loading = ref(false)
const loadError = ref<string | null>(null)
const downloading = ref(false)
const uploading = ref(false)
const saveLabel = ref('未修改')
const saveError = ref<string | null>(null)
const aiOpen = ref(false)
const aiAnchor = ref({ x: 0, y: 0 })
const aiOriginal = ref('')
let aiTargetIndex: number | null = null
const historyOpen = ref(false)
const versions = ref<Array<{ version: number; size_bytes: number; created_at: string }>>([])

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

// v0.7.38 — remote selection broadcast
const remoteSelections = ref<Array<{
  clientId: number
  displayName: string
  color: string
  selection?: { from: number; to: number } | null
}>>([])

const remoteSelectionFor = (clientId: number) => {
  return remoteSelections.value.find((p) => p.clientId === clientId)?.selection || null
}

const peerTitle = (p: { displayName: string; clientId: number; color: string }) => {
  const sel = remoteSelectionFor(p.clientId)
  const range = sel ? ` — 选区 ${sel.from}–${sel.to}` : ''
  return `${p.displayName}${range}`
}

const downloadAsUint8Array = async (): Promise<Uint8Array> => {
  const blob = await downloadCollabDocBytes(props.docId)
  const buffer = await blob.arrayBuffer()
  return new Uint8Array(buffer)
}

const onDownload = async () => {
  if (!doc) return
  if (!doc) return
  const curDoc = doc
  downloading.value = true
  try {
    const patched = Array.from(patchedMap.entries()).map(([docxIndex, xml]) => ({
      docxIndex, xml, text: curDoc.paragraphs[docxIndex]?.text || '',
    }))
    const bytes = await saveDocxBytes(curDoc, patched)
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

const isMarkActive = (name: string): boolean => {
  if (!editor.value) return false
  return editor.value.isActive(name)
}
const isNodeActive = (name: string, attrs?: Record<string, unknown>): boolean => {
  if (!editor.value) return false
  return editor.value.isActive(name, attrs)
}
const runMark = (name: string) => {
  if (!editor.value) return
  const chain = editor.value.chain().focus()
  if (name === 'bold') chain.toggleBold().run()
  else if (name === 'italic') chain.toggleItalic().run()
  else if (name === 'strike') chain.toggleStrike().run()
  else if (name === 'code') chain.toggleCode().run()
  else if (name === 'link') chain.toggleLink({ href: '' }).run()
  else chain.run()
}
const runNode = (cmd: string) => {
  if (!editor.value) return
  const chain = editor.value.chain().focus()
  if (cmd === 'toggleBulletList') chain.toggleBulletList().run()
  else if (cmd === 'toggleOrderedList') chain.toggleOrderedList().run()
  else if (cmd === 'toggleBlockquote') chain.toggleBlockquote().run()
  else if (cmd === 'toggleCodeBlock') chain.toggleCodeBlock().run()
  else if (cmd === 'undo') chain.undo().run()
  else if (cmd === 'redo') chain.redo().run()
  else chain.run()
}
const runHeading = (level: 1 | 2 | 3) => {
  if (!editor.value) return
  editor.value.chain().focus().toggleHeading({ level }).run()
}
const onSetLink = () => {
  if (!editor.value) return
  const prev = editor.value.getAttributes('link').href as string | undefined
  const url = window.prompt('链接地址（留空取消）', prev || 'https://')
  if (url === null) return
  if (url === '') {
    editor.value.chain().focus().extendMarkRange('link').unsetLink().run()
    return
  }
  editor.value.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
}

const onInsertTable = () => {
  if (!editor.value) return
  const rowsStr = window.prompt('行数（含表头，默认 3）', '3')
  const colsStr = window.prompt('列数（默认 3）', '3')
  if (rowsStr === null || colsStr === null) return
  const rows = Math.max(1, Math.min(20, Number(rowsStr) || 3))
  const cols = Math.max(1, Math.min(10, Number(colsStr) || 3))
  editor.value
    .chain()
    .focus()
    .insertTable({ rows, cols, withHeaderRow: true })
    .run()
}

// --- v0.7.29 — comments anchor (set when the user selects a range) ---
const commentAnchor = ref<{ type: 'doc' | 'slide' | 'sheet'; ref: string } | null>(null)

const onInsertImageUrl = () => {
  if (!editor.value) return
  const url = window.prompt('图片 URL（留空取消）', 'https://')
  if (!url) return
  editor.value.chain().focus().setImage({ src: url }).run()
}

const onInsertImageFile = () => {
  fileImageInput.value?.click()
}

const fileImageInput = ref<HTMLInputElement | null>(null)
const onImageFileChosen = async (e: Event) => {
  if (!editor.value) return
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    MessagePlugin.error('请选择图片文件 (PNG/JPEG/GIF/WebP)')
    return
  }
  // Convert to dataURL (TipTap image extension accepts it; the
  // saveDocxBytesWithImages flow will then write the bytes into the .docx).
  const reader = new FileReader()
  reader.onload = () => {
    const dataUrl = String(reader.result || '')
    if (!dataUrl.startsWith('data:')) return
    const img = new window.Image()
    img.onload = () => {
      editor.value!
        .chain()
        .focus()
        .setImage({ src: dataUrl, alt: file.name })
        .run()
    }
    img.src = dataUrl
  }
  reader.readAsDataURL(file)
  if (fileImageInput.value) fileImageInput.value.value = ''
}

const onAlign = (align: 'left' | 'center' | 'right') => {
  if (!editor.value) return
  ;(editor.value.chain().focus() as any)[`setTextAlign`](align).run()
}

const onToggleHistory = async () => {
  historyOpen.value = !historyOpen.value
  if (historyOpen.value) await loadVersions()
}

const loadVersions = async () => {
  try {
    const r = await fetch(`/api/v1/collaborative-docs/${encodeURIComponent(props.docId)}/files`, {
      headers: { Authorization: `Bearer ${props.token}` },
    })
    if (!r.ok) return
    const json = await r.json()
    versions.value = (json.data || []) as Array<{ version: number; size_bytes: number; created_at: string }>
  } catch {
    versions.value = []
  }
}

const formatBytes = (n: number) => {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

const formatTime = (s: string) => (s ? new Date(s).toLocaleString() : '—')

const onDownloadVersion = async (v: number) => {
  try {
    const r = await fetch(`/api/v1/collaborative-docs/${encodeURIComponent(props.docId)}/download/${v}`, {
      headers: { Authorization: `Bearer ${props.token}` },
    })
    if (!r.ok) throw new Error(`status ${r.status}`)
    const blob = await r.blob()
    const ab = await blob.arrayBuffer()
    const buf = new Uint8Array(ab)
    const url = URL.createObjectURL(new Blob([buf], {
      type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${props.title || 'collab-doc'}-v${v}.docx`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e: any) {
    MessagePlugin.error(`下载失败：${e?.message || e}`)
  }
}

const onRestoreVersion = async (v: number) => {
  if (!confirm(`确认将文档恢复到 v${v}？当前未保存的修改会丢失。`)) return
  try {
    const r = await fetch(`/api/v1/collaborative-docs/${encodeURIComponent(props.docId)}/download/${v}`, {
      headers: { Authorization: `Bearer ${props.token}` },
    })
    if (!r.ok) throw new Error(`status ${r.status}`)
    const blob = await r.blob()
    const bytes = new Uint8Array(await blob.arrayBuffer())
    doc = await openDocx(bytes)
    if (editor.value) {
      editor.value.commands.setContent(paragraphsToContent(doc.paragraphs), false)
    }
    patchedMap.clear()
    MessagePlugin.success(`已恢复到 v${v}`)
    scheduleSave()
  } catch (e: any) {
    MessagePlugin.error(`恢复失败：${e?.message || e}`)
  }
}

const refreshAiSelection = () => {
  // Capture the current TipTap selection's text + position so the AI popover
  // can show the selected paragraph and we know which block.docxIndex to
  // patch when the user accepts.
  if (!editor.value) {
    aiOriginal.value = ''
    return
  }
  const { from, to } = editor.value.state.selection
  if (from === to) {
    aiOriginal.value = ''
    aiTargetIndex = null
    return
  }
  const text = editor.value.state.doc.textBetween(from, to, '\n')
  if (!text.trim()) {
    aiOriginal.value = ''
    aiTargetIndex = null
    return
  }
  aiOriginal.value = text
  // Walk back to find the closest paragraph's docx-index attribute.
  const $from = editor.value.state.doc.resolve(from)
  for (let d = $from.depth; d >= 0; d--) {
    const node = $from.node(d)
    if (node?.type?.name === 'paragraph' || node?.type?.name === 'heading') {
      const idx = node.attrs?.['data-docx-index']
      aiTargetIndex = typeof idx === 'number' ? idx : idx ? Number(idx) : null
      break
    }
  }
}

const onOpenAi = () => {
  refreshAiSelection()
  if (!aiOriginal.value) {
    MessagePlugin.warning('请先在文档中选中要润色的段落')
    return
  }
  aiAnchor.value = { x: window.innerWidth / 2 - 240, y: 120 }
  aiOpen.value = true
}

const onAcceptAi = (replacement: string) => {
  aiOpen.value = false
  if (!editor.value || aiTargetIndex == null || !doc) return
  // Apply the replacement to the matching docx-engine block and re-sync
  // the editor's underlying paragraph via editor.commands.
  const targetIdx: number = aiTargetIndex
  patchedMap.set(targetIdx, replacement)
  doc.paragraphs[targetIdx].text = replacement
  // Replace selection text in TipTap.
  const ed = editor.value
  const { from, to } = ed.state.selection
  ed.chain().focus().insertContentAt({ from, to }, replacement).run()
  scheduleSave()
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
  // v0.7.38 — remote selection range awareness (DOC per-paragraph).
  if (handle.remoteSelections) {
    remoteSelections.value = handle.remoteSelections.value as any
    watch(handle.remoteSelections, (v) => (remoteSelections.value = (v ?? []) as any))
  }
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
      Underline,
      Highlight.configure({ multicolor: true }),
      Color,
      TextAlign.configure({ types: ['heading', 'paragraph'] }),
      Table.configure({ resizable: true, HTMLAttributes: { class: 'collab-doc-pro__table' } }),
      TableRow,
      TableHeader,
      TableCell,
      Image.configure({ inline: false, allowBase64: true, HTMLAttributes: { class: 'collab-doc-pro__image' } }),
      TaskList,
      TaskItem.configure({ nested: true }),
      ...(ydoc ? [Collaboration.configure({ document: ydoc, field: 'docx-body' })] : []),
      ...(ydoc && handle ? [CollaborationCursor.configure({
        provider: handle.provider,
        user: { name: props.displayName, color: '#58a6ff' },
      })] : []),
    ],
    content: paragraphsToContent(paragraphs),
    onUpdate: ({ editor: ed }) => onEditorUpdate(ed),
    onSelectionUpdate: ({ editor: ed }) => {
      // ed is the tiptap core Editor which exposes the same .state API
      // as the vue-3 wrapper we hold in editor.value.
      void ed
      refreshAiSelection()
      // v0.7.38 — broadcast the local selection range so other
      // collaborators can render a highlight rectangle over the
      // selected text. publishSelection is idempotent on identical
      // {from,to}; the awareness layer merges via y-protocols.
      try {
        const sel = ed.state.selection
        if (sel && handle) {
          handle.publishSelection(sel.from, sel.to)
        }
      } catch (e) {
        // never block the editor on a wire failure
        // eslint-disable-next-line no-console
        console.warn('[CollabDocProEditor] publishSelection failed', e)
      }
    },
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

const onEditorUpdate = (ed: import('@tiptap/core').Editor) => {
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
  const curDoc = doc
  saveLabel.value = '保存中...'
  try {
    const patched = Array.from(patchedMap.entries()).map(([docxIndex, xml]) => ({
      docxIndex, xml, text: curDoc.paragraphs[docxIndex]?.text || '',
    }))
    const bytes = editor.value
      ? await saveDocxBytesWithImages(curDoc, editor.value.getJSON() as any)
      : await saveDocxBytes(curDoc, patched)
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
.collab-doc-pro__peer {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 999px;
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border: 2px solid transparent;
  transition: border-color 120ms ease, box-shadow 120ms ease;
}
.collab-doc-pro__peer--selecting {
  border-color: rgba(255, 255, 255, 0.85);
  box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.18);
}
.collab-doc-pro__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-doc-pro__surface-wrap { flex: 1; display: flex; flex-direction: column; min-height: 0; }
.collab-doc-pro__surface { flex: 1; overflow: auto; padding: 24px 32px; max-width: 880px; margin: 0 auto; }
.collab-doc-pro__loading, .collab-doc-pro__error { padding: 24px; }
.collab-doc-pro__error { color: var(--td-error-color-7); }
</style>
.collab-doc-pro__table { border-collapse: collapse; margin: 12px 0; width: 100%; table-layout: fixed; }
.collab-doc-pro__table th, .collab-doc-pro__surface :deep(table th),
.collab-doc-pro__surface :deep(table td) { border: 1px solid var(--td-component-stroke, #e7e7e7); padding: 6px 10px; vertical-align: top; min-width: 60px; }
.collab-doc-pro__surface :deep(table th) { background: var(--td-bg-color-container, #f7f7f7); font-weight: 600; }
.collab-doc-pro__surface :deep(.selectedCell) { background: rgba(88, 166, 255, 0.12); }
.collab-doc-pro__image, .collab-doc-pro__surface :deep(img) { max-width: 100%; height: auto; border-radius: 4px; margin: 8px 0; }
.collab-doc-pro__surface :deep(ul[data-type="taskList"]) { list-style: none; padding-left: 0; }
.collab-doc-pro__surface :deep(ul[data-type="taskList"] li) { display: flex; gap: 6px; align-items: flex-start; }
.collab-doc-pro__surface :deep(ul[data-type="taskList"] li > label) { flex: 0 0 auto; margin-top: 4px; }
.collab-doc-pro__surface :deep(ul[data-type="taskList"] li > div) { flex: 1 1 auto; }
.collab-doc-pro__surface :deep(mark) { background: #fff3a3; padding: 0 2px; border-radius: 2px; }
