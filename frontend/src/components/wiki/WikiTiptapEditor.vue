<template>
  <div class="wiki-tiptap-editor" :class="{ 'is-empty': isEmpty }">
    <div v-if="editor" class="wiki-tiptap-toolbar">
      <button type="button" class="wiki-tiptap-btn"
        :class="{ 'is-active': editor.isActive('bold') }"
        :aria-pressed="editor.isActive('bold')"
        @click.prevent="run('bold')" :title="$t('knowledgeEditor.wikiBrowser.editor.bold')">
        <span class="wiki-tiptap-btn-label">B</span>
      </button>
      <button type="button" class="wiki-tiptap-btn wiki-tiptap-btn--italic"
        :class="{ 'is-active': editor.isActive('italic') }"
        :aria-pressed="editor.isActive('italic')"
        @click.prevent="run('italic')" :title="$t('knowledgeEditor.wikiBrowser.editor.italic')">
        <span class="wiki-tiptap-btn-label"><em>I</em></span>
      </button>
      <span class="wiki-tiptap-divider" aria-hidden="true" />
      <button v-for="level in headingLevels" :key="level"
        type="button" class="wiki-tiptap-btn wiki-tiptap-btn--heading"
        :class="{ 'is-active': editor.isActive('heading', { level }) }"
        :aria-pressed="editor.isActive('heading', { level })"
        @click.prevent="runHeading(level)" :title="$t('knowledgeEditor.wikiBrowser.editor.heading', { level })">
        H{{ level }}
      </button>
      <span class="wiki-tiptap-divider" aria-hidden="true" />
      <button type="button" class="wiki-tiptap-btn"
        :class="{ 'is-active': editor.isActive('orderedList') }"
        :aria-pressed="editor.isActive('orderedList')"
        @click.prevent="run('orderedList')" :title="$t('knowledgeEditor.wikiBrowser.editor.orderedList')">
        <span class="wiki-tiptap-btn-label">1.</span>
      </button>
      <button type="button" class="wiki-tiptap-btn"
        :class="{ 'is-active': editor.isActive('bulletList') }"
        :aria-pressed="editor.isActive('bulletList')"
        @click.prevent="run('bulletList')" :title="$t('knowledgeEditor.wikiBrowser.editor.bulletList')">
        <span class="wiki-tiptap-btn-label">•</span>
      </button>
      <span class="wiki-tiptap-divider" aria-hidden="true" />
      <button type="button" class="wiki-tiptap-btn"
        :class="{ 'is-active': editor.isActive('code') }"
        :aria-pressed="editor.isActive('code')"
        @click.prevent="run('code')" :title="$t('knowledgeEditor.wikiBrowser.editor.inlineCode')">
        <span class="wiki-tiptap-btn-label wiki-tiptap-btn-label--mono">&lt;/&gt;</span>
      </button>
      <button type="button" class="wiki-tiptap-btn"
        :class="{ 'is-active': editor.isActive('codeBlock') }"
        :aria-pressed="editor.isActive('codeBlock')"
        @click.prevent="run('codeBlock')" :title="$t('knowledgeEditor.wikiBrowser.editor.codeBlock')">
        <span class="wiki-tiptap-btn-label wiki-tiptap-btn-label--mono">{ }</span>
      </button>
      <span class="wiki-tiptap-divider" aria-hidden="true" />
      <button type="button" class="wiki-tiptap-btn"
        :class="{ 'is-active': editor.isActive('link') }"
        :aria-pressed="editor.isActive('link')"
        @click.prevent="promptForLink" :title="$t('knowledgeEditor.wikiBrowser.editor.link')">
        <span class="wiki-tiptap-btn-label">🔗</span>
      </button>
      <span v-if="collab" class="wiki-tiptap-divider" aria-hidden="true" />
      <WikiCollabPresence
        v-if="collab"
        :status="collab.status.value"
        :peer-list="collab.peerList.value"
        :recent-collaborators="collab.recentCollaborators.value"
        :self-name="props.displayName || props.userId"
        class="wiki-tiptap-presence"
        @reconnect="collab.reconnect()"
      />
    </div>

    <EditorContent v-if="editor" :editor="editor" class="wiki-tiptap-content" />

    <div v-else class="wiki-tiptap-fallback">
      <t-textarea v-model="fallbackMarkdown" class="wiki-tiptap-fallback-textarea"
        :autosize="{ minRows: 16, maxRows: 40 }"
        :placeholder="placeholder" @input="emitFromFallback" />
      <p class="wiki-tiptap-fallback-hint">
        {{ $t('knowledgeEditor.wikiBrowser.editor.fallbackHint') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { Editor } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import { Markdown } from 'tiptap-markdown'
import { EditorContent } from '@tiptap/vue-3'
import { useWikiCollab } from '@/composables/useWikiCollab'
import WikiCollabPresence from './WikiCollabPresence.vue'
// sanitizeWysiwygHTML is the spec-shaped exit sanitizer (returns '' on
// DOMPurify throw + console.warn) — distinct from utils/security.ts's
// generic sanitizeHTML which falls back to escapeHTML.
import { sanitizeWysiwygHTML } from '@/utils/sanitize/wysiwyg'

/**
 * WikiTiptapEditor — Tiptap v2 wrapper for the Wiki page WYSIWYG flow.
 *
 * Model contract (`v-model`): receives `{ html, markdown }` from the parent
 * (where `markdown` is the canonical persisted value, `html` is the cached
 * sanitized render) and emits the same shape on every change. The parent is
 * responsible for persisting both; on save it sends `content` (markdown) and
 * `content_html` to the backend.
 *
 * Sanitization policy (Build #2b Decision 2):
 *   the editor emits sanitized HTML via `editor.getHTML()` at every change,
 *   NOT raw HTML. `tiptap-markdown` parses the markdown form via
 *   `editor.storage.markdown.parser` and `getMarkdown()` reverses it. We
 *   feed the markdown parser sanitized HTML on mount so a cached
 *   `content_html` from another author can't smuggle scripts into the
 *   editor surface.
 *
 * Failure mode: if Tiptap fails to construct (older browser, missing
 * dependencies), we render a plain `<t-textarea>` bound to the markdown
 * value so the user never loses the ability to save. This is the same
 * fail-open posture as the feature-flag store.
 */

interface WikiEditorValue {
  html: string
  markdown: string
}

const props = withDefaults(defineProps<{
  modelValue: WikiEditorValue
  placeholder?: string
  /** Real-time collaboration identifiers. When `collabEnabled` is true,
   *  the editor binds to a Y.js document + awareness through the
   *  `useWikiCollab` composable. When false / omitted, behaviour matches
   *  the existing solo-editing path (no network deps loaded). */
  kbId?: string
  slug?: string
  userId?: string
  displayName?: string
  collabEnabled?: boolean
}>(), {
  placeholder: '',
  kbId: '',
  slug: '',
  userId: '',
  displayName: '',
  collabEnabled: false,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: WikiEditorValue): void
}>()

const headingLevels = [1, 2, 3] as const

const editor = shallowRef<Editor | null>(null)
const isEmpty = ref(true)
const fallbackMarkdown = ref<string>(props.modelValue?.markdown ?? '')

// Build #8 — Y.js CRDT collaboration. The composable is called eagerly
// when `collabEnabled` is true so its lifecycle hooks attach before the
// Editor is constructed; the resulting `ydoc` / `provider` are then
// passed to the Collaboration + CollaborationCursor extensions below.
const collab = props.collabEnabled && props.kbId && props.slug
  ? useWikiCollab({
      kbId: props.kbId,
      slug: props.slug,
      userId: props.userId || 'anonymous',
      displayName: props.displayName || props.userId || 'Anonymous',
    })
  : null

watch(
  () => props.modelValue,
  (next) => {
    const nextMarkdown = next?.markdown ?? ''
    if (editor.value && editor.value.storage.markdown?.getMarkdown() !== nextMarkdown) {
      // External change (load from server, conflict reload, undo): rewrite
      // the document via the markdown parser so the editor stays consistent.
      // Tiptap v2 signature: setContent(content, emitUpdate, parseOptions).
      editor.value.commands.setContent(sanitizeWysiwygHTML(next?.html ?? ''), false)
    }
    fallbackMarkdown.value = nextMarkdown
  },
  { deep: true },
)

const placeholder = computed(() => props.placeholder)

function emitUpdate() {
  if (!editor.value) return
  const html = sanitizeWysiwygHTML(editor.value.getHTML())
  // tiptap-markdown exposes getMarkdown() via storage; fall back to empty
  // string when the extension isn't initialised yet.
  const markdown =
    (typeof editor.value.storage.markdown?.getMarkdown === 'function'
      ? editor.value.storage.markdown.getMarkdown()
      : editor.value.getText()) || ''
  isEmpty.value = editor.value.isEmpty
  emit('update:modelValue', { html, markdown })
}

function emitFromFallback() {
  emit('update:modelValue', { html: '', markdown: fallbackMarkdown.value })
}

function run(name: 'bold' | 'italic' | 'code' | 'orderedList' | 'bulletList' | 'codeBlock') {
  if (!editor.value) return
  const chain = editor.value.chain().focus()
  switch (name) {
    case 'bold': chain.toggleBold().run(); break
    case 'italic': chain.toggleItalic().run(); break
    case 'code': chain.toggleCode().run(); break
    case 'orderedList': chain.toggleOrderedList().run(); break
    case 'bulletList': chain.toggleBulletList().run(); break
    case 'codeBlock': chain.toggleCodeBlock().run(); break
  }
}

function runHeading(level: 1 | 2 | 3) {
  if (!editor.value) return
  editor.value.chain().focus().toggleHeading({ level }).run()
}

function promptForLink() {
  if (!editor.value) return
  const previous = editor.value.getAttributes('link').href as string | undefined
  // eslint-disable-next-line no-alert
  const url = window.prompt('URL', previous ?? 'https://')
  if (url === null) return
  if (url === '') {
    editor.value.chain().focus().unsetLink().run()
    return
  }
  editor.value.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
}

try {
  const initialMarkdown = props.modelValue?.markdown ?? ''
  const initialHTML = sanitizeWysiwygHTML(props.modelValue?.html ?? '')
  // When collaboration is enabled we disable StarterKit's history
  // extension — the CRDT manages undo / redo via Y.UndoManager, and
  // having both causes state divergence between peers.
  const extensions: Array<unknown> = [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      history: collab ? false : {},
    }),
    Link.configure({
      // Restrict to safe protocols — Tiptap's default allows javascript:
      // URLs which DOMPurify would later strip anyway, but rejecting
      // them at insertion time keeps the editor in a clean state.
      openOnClick: false,
      autolink: true,
      protocols: ['http', 'https', 'mailto'],
    }),
    Markdown.configure({
      // tiptap-markdown parses round-trip markdown; transformPastedText
      // lets paste-as-markdown flow through. We keep the default copy-
      // paste behaviour (HTML) for compatibility with rich clipboard
      // sources.
      transformPastedText: true,
      breaks: true,
      linkify: false,
    }),
  ]
  if (collab && collab.provider.value) {
    // Lazy import the CRDT extensions so the no-collab path doesn't pay
    // the y-prosemirror bundle cost.
    const [{ Collaboration }, { CollaborationCursor }] = await Promise.all([
      import('@tiptap/extension-collaboration'),
      import('@tiptap/extension-collaboration-cursor'),
    ])
    extensions.push(
      Collaboration.configure({
        document: collab.ydoc,
      }),
      CollaborationCursor.configure({
        provider: collab.provider.value,
        user: {
          name: props.displayName || props.userId || 'Anonymous',
          color: collab.color.value,
        },
      }),
    )
  }
  editor.value = new Editor({
    content: initialHTML,
    extensions: extensions as Parameters<typeof Editor>[0]['extensions'],
    editorProps: {
      attributes: {
        class: 'wiki-tiptap-surface',
      },
    },
    onUpdate: emitUpdate,
    onCreate({ editor: ed }) {
      isEmpty.value = ed.isEmpty
      // If we were constructed with HTML but the parent expects the
      // markdown round-trip to be canonical, sync once on mount.
      if (initialMarkdown && ed.storage.markdown?.getMarkdown() !== initialMarkdown) {
        // Tiptap v2 signature: setContent(content, emitUpdate, parseOptions).
        // Pass `false` to skip the onUpdate callback during programmatic
        // sync — the parent already has the markdown value.
        ed.commands.setContent(initialHTML, false)
        emitUpdate()
      }
    },
  })
} catch (error) {
  // Tiptap failed to construct; drop to the textarea fallback. The parent
  // keeps working and the user retains the markdown content path.
  // eslint-disable-next-line no-console
  console.error('[WikiTiptapEditor] Tiptap init failed, falling back to textarea:', error)
  editor.value = null
  fallbackMarkdown.value = props.modelValue?.markdown ?? ''
}

onBeforeUnmount(() => {
  editor.value?.destroy()
  editor.value = null
})
</script>

<style scoped>
.wiki-tiptap-editor {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--component-stroke, #dcdcdc);
  border-radius: 6px;
  background: var(--component-bg, #fff);
}

.wiki-tiptap-toolbar {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--component-stroke, #e7e7e7);
  flex-wrap: wrap;
}

.wiki-tiptap-btn {
  min-width: 30px;
  height: 28px;
  padding: 0 8px;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--text-primary, #1f1f1f);
  font-size: 13px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.wiki-tiptap-btn:hover {
  background: var(--component-hover, #f3f3f3);
}

.wiki-tiptap-btn.is-active {
  background: var(--brand-color-light, #e6f1ff);
  border-color: var(--brand-color, #3662ec);
  color: var(--brand-color, #3662ec);
}

.wiki-tiptap-btn-label {
  font-weight: 600;
  line-height: 1;
}

.wiki-tiptap-btn-label--mono {
  font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
  font-size: 12px;
  font-weight: 500;
}

.wiki-tiptap-btn--heading {
  font-size: 12px;
  font-weight: 600;
}

.wiki-tiptap-divider {
  width: 1px;
  height: 16px;
  background: var(--component-stroke, #e0e0e0);
  margin: 0 4px;
}

.wiki-tiptap-content {
  padding: 12px 16px;
  min-height: 280px;
  max-height: 600px;
  overflow-y: auto;
  font-size: 14px;
  line-height: 1.6;
}

.wiki-tiptap-content :deep(.wiki-tiptap-surface) {
  outline: none;
  min-height: 240px;
}

.wiki-tiptap-content :deep(.ProseMirror) h1,
.wiki-tiptap-content :deep(.ProseMirror) h2,
.wiki-tiptap-content :deep(.ProseMirror) h3 {
  margin: 1.2em 0 0.4em;
}

.wiki-tiptap-content :deep(.ProseMirror) p {
  margin: 0.6em 0;
}

.wiki-tiptap-content :deep(.ProseMirror) code {
  background: var(--component-hover, #f5f5f5);
  border-radius: 3px;
  padding: 1px 4px;
  font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
  font-size: 0.9em;
}

.wiki-tiptap-content :deep(.ProseMirror) pre {
  background: var(--component-hover, #f5f5f5);
  padding: 10px 12px;
  border-radius: 4px;
  overflow-x: auto;
}

.wiki-tiptap-content :deep(.ProseMirror) ul,
.wiki-tiptap-content :deep(.ProseMirror) ol {
  padding-left: 1.5em;
}

.wiki-tiptap-content :deep(.ProseMirror) a {
  color: var(--brand-color, #3662ec);
  text-decoration: underline;
}

.wiki-tiptap-fallback {
  padding: 12px 16px;
}

.wiki-tiptap-fallback-textarea {
  width: 100%;
}

.wiki-tiptap-fallback-hint {
  margin-top: 8px;
  color: var(--text-secondary, #888);
  font-size: 12px;
}
</style>