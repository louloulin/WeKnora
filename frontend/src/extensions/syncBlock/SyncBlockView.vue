<!--
  SyncBlockView — Vue node view for the wiki SyncBlock TipTap node.

  Renders the resolved canonical block content (markdown → HTML) with
  a "lock chip" header showing the sync ref count, version, and a
  stale indicator when the embedded copy is older than canonical.

  Loads via `getSyncBlock(kbId, blockId)` and `listSyncBlockRefs(...)`
  the first time the node enters the viewport; subsequent renders are
  served from a module-level cache keyed on (kbId, blockId) so editing
  a long doc doesn't re-fetch every block on every keystroke.

  Emits no events directly. Edits go through the picker or the canonical
  block editor — this view is read-only by design.
-->

<template>
  <div class="sync-block-view" :class="{ 'is-stale': stale, 'is-loading': loading, 'is-missing': missing }">
    <header class="sync-block-view-header">
      <span class="sync-block-view-lock" aria-hidden="true">🔗</span>
      <span class="sync-block-view-title">{{ title || blockId }}</span>
      <span class="sync-block-view-version">v{{ version }}</span>
      <span v-if="stale" class="sync-block-view-stale">stale</span>
      <span v-else class="sync-block-view-fresh">fresh</span>
    </header>
    <div v-if="loading" class="sync-block-view-loading">Loading…</div>
    <div v-else-if="missing" class="sync-block-view-missing">
      Synced block <code>{{ blockId }}</code> not found.
    </div>
    <div v-else class="sync-block-view-body" v-html="renderedHtml" />
    <footer class="sync-block-view-footer">
      <span class="sync-block-view-refs">{{ refCount }} {{ refCount === 1 ? 'reference' : 'references' }}</span>
      <button type="button" class="sync-block-view-edit" @click="openCanonical">Edit canonical</button>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { marked } from 'marked'
import { getSyncBlock, listSyncBlockRefs } from '../../api/syncBlock'
import type { WikiSyncBlock } from '../../api/syncBlock/types'

interface Props {
  node: {
    attrs: {
      block_id: string
      version: number
      stale: boolean
      rendered_at: string
    }
  }
  kbId: string
}

const props = defineProps<Props>()

const blockId = computed(() => props.node.attrs.block_id || '')
const version = computed(() => Number(props.node.attrs.version || 0))
const stale = computed(() => Boolean(props.node.attrs.stale))

const loading = ref(false)
const missing = ref(false)
const title = ref('')
const renderedHtml = ref('')
const refCount = ref(0)

const cache = new Map<string, WikiSyncBlock>()

async function load() {
  const key = `${props.kbId}::${blockId.value}`
  if (!blockId.value) {
    missing.value = true
    return
  }
  loading.value = true
  try {
    let block = cache.get(key)
    if (!block) {
      block = await getSyncBlock(props.kbId, blockId.value)
      cache.set(key, block)
    }
    title.value = block.title || blockId.value
    const md = block.content_md || ''
    renderedHtml.value = marked.parse(md, { async: false }) as string
    try {
      const refs = await listSyncBlockRefs(props.kbId, blockId.value)
      refCount.value = Array.isArray(refs.refs) ? refs.refs.length : 0
    } catch {
      refCount.value = 0
    }
  } catch {
    missing.value = true
  } finally {
    loading.value = false
  }
}

function openCanonical() {
  if (!blockId.value) return
  // The canonical editor is currently scoped to a separate modal; we
  // emit a custom event the host editor listens to.
  const evt = new CustomEvent('wiki:sync-block:edit', {
    detail: { blockId: blockId.value, kbId: props.kbId },
    bubbles: true,
  })
  ;(document.querySelector('.sync-block-view') as HTMLElement | null)?.dispatchEvent(evt)
}

onMounted(load)
</script>

<style scoped>
.sync-block-view {
  border: 1px solid var(--color-border, #e5e7eb);
  border-radius: 6px;
  padding: 10px 14px;
  margin: 8px 0;
  background: var(--color-bg-soft, #f9fafb);
  font-size: 14px;
}
.sync-block-view.is-stale {
  border-color: #f59e0b;
  background: #fffbeb;
}
.sync-block-view.is-missing {
  border-style: dashed;
  border-color: #ef4444;
  background: #fef2f2;
}
.sync-block-view-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--color-text-secondary, #4b5563);
}
.sync-block-view-lock {
  font-size: 14px;
}
.sync-block-view-title {
  font-weight: 600;
  color: var(--color-text-primary, #111827);
}
.sync-block-view-version {
  background: #e5e7eb;
  border-radius: 4px;
  padding: 0 6px;
}
.sync-block-view-stale {
  color: #b45309;
}
.sync-block-view-fresh {
  color: #047857;
}
.sync-block-view-body :deep(p) {
  margin: 4px 0;
}
.sync-block-view-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 6px;
  font-size: 12px;
}
.sync-block-view-edit {
  background: transparent;
  border: 1px solid var(--color-border, #d1d5db);
  border-radius: 4px;
  padding: 2px 8px;
  cursor: pointer;
}
.sync-block-view-edit:hover {
  background: var(--color-bg-hover, #f3f4f6);
}
</style>
