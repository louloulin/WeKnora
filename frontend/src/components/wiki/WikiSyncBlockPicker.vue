<!--
  WikiSyncBlockPicker — v0.7.20 picker for inserting synced blocks.

  Flow:
    1. List canonical blocks for the current KB
    2. User picks one (or clicks "Create new" to draft a fresh block)
    3. Insert the chosen block's marker `[[sync:UUID]]` at the current
       editor cursor position

  Emits:
    insert(blockId, title) — emit the chosen block
    create — user wants to create a new block
    close — user dismissed the picker
-->
<template>
  <div class="wiki-sync-block-picker" role="dialog" aria-label="Insert synced block">
    <div class="wiki-sync-block-picker-header">
      <h3>{{ pickerTitle }}</h3>
      <button type="button" class="wiki-sync-block-picker-close" @click="$emit('close')">×</button>
    </div>
    <div class="wiki-sync-block-picker-toolbar">
      <input
        v-model="filter"
        type="search"
        :placeholder="searchPlaceholder"
        class="wiki-sync-block-picker-search"
      />
      <button type="button" class="wiki-sync-block-picker-create" @click="$emit('create')">
        + {{ createNewLabel }}
      </button>
    </div>
    <div v-if="loading" class="wiki-sync-block-picker-loading">{{ loadingLabel }}</div>
    <div v-else-if="!filtered.length" class="wiki-sync-block-picker-empty">{{ emptyLabel }}</div>
    <ul v-else class="wiki-sync-block-picker-list" role="listbox">
      <li
        v-for="block in filtered"
        :key="block.block_id"
        class="wiki-sync-block-picker-item"
        :class="{ 'is-selected': selectedBlockId === block.block_id }"
        @click="selectedBlockId = block.block_id"
        @dblclick="emitInsert(block)"
        role="option"
        :aria-selected="selectedBlockId === block.block_id"
      >
        <div class="wiki-sync-block-picker-item-title">{{ block.title || block.block_id }}</div>
        <div class="wiki-sync-block-picker-item-preview">{{ block.content_md.slice(0, 120) || '—' }}</div>
        <div class="wiki-sync-block-picker-item-meta">
          <span class="wiki-sync-block-picker-version">v{{ block.version }}</span>
          <span class="wiki-sync-block-picker-updated">{{ formatTime(block.updated_at) }}</span>
        </div>
      </li>
    </ul>
    <div class="wiki-sync-block-picker-footer">
      <button type="button" @click="$emit('close')">{{ cancelLabel }}</button>
      <button type="button" :disabled="!selectedBlockId" @click="emitInsert(selectedBlock)">
        {{ insertLabel }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { listSyncBlocks } from '../../api/syncBlock'
import type { WikiSyncBlock } from '../../api/syncBlock/types'

const props = defineProps<{
  kbId: string
}>()

const emit = defineEmits<{
  (e: 'insert', blockId: string, title: string): void
  (e: 'create'): void
  (e: 'close'): void
}>()

const blocks = ref<WikiSyncBlock[]>([])
const loading = ref(false)
const filter = ref('')
const selectedBlockId = ref<string | null>(null)

const pickerTitle = 'Insert synced block'
const searchPlaceholder = 'Search blocks…'
const createNewLabel = 'Create new'
const emptyLabel = 'No synced blocks yet. Create one to get started.'
const loadingLabel = 'Loading…'
const cancelLabel = 'Cancel'
const insertLabel = 'Insert'

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return blocks.value
  return blocks.value.filter((b) =>
    (b.title || '').toLowerCase().includes(q) || b.content_md.toLowerCase().includes(q)
  )
})

const selectedBlock = computed(() => blocks.value.find((b) => b.block_id === selectedBlockId.value) ?? null)

function formatTime(iso: string): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

function emitInsert(block: WikiSyncBlock | null): void {
  if (!block) return
  emit('insert', block.block_id, block.title || block.block_id)
}

onMounted(async () => {
  loading.value = true
  try {
    blocks.value = await listSyncBlocks(props.kbId, { limit: 50 })
  } catch (e) {
    // eslint-disable-next-line no-console
    console.error('[sync-block] list failed:', e)
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.wiki-sync-block-picker {
  display: flex;
  flex-direction: column;
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 10px;
  padding: 12px;
  width: 360px;
  max-height: 480px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, .35);
  color: #e6edf3;
  font-family: inherit;
  font-size: 13px;
}
.wiki-sync-block-picker-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.wiki-sync-block-picker-header h3 { margin: 0; font-size: 14px; font-weight: 600; }
.wiki-sync-block-picker-close { background: transparent; border: 0; font-size: 18px; cursor: pointer; color: inherit; }
.wiki-sync-block-picker-toolbar { display: flex; gap: 6px; margin-bottom: 8px; }
.wiki-sync-block-picker-search {
  flex: 1; padding: 4px 8px; border-radius: 6px; border: 1px solid #30363d;
  background: #0e1117; color: inherit; font-size: 12px;
}
.wiki-sync-block-picker-create {
  padding: 4px 10px; border-radius: 6px; border: 1px solid #58a6ff;
  background: rgba(88, 166, 255, .12); color: #58a6ff; font-size: 12px; cursor: pointer;
}
.wiki-sync-block-picker-list { list-style: none; margin: 0; padding: 0; overflow-y: auto; flex: 1; }
.wiki-sync-block-picker-item { padding: 8px 10px; border-radius: 6px; cursor: pointer; border: 1px solid transparent; }
.wiki-sync-block-picker-item:hover { background: rgba(88, 166, 255, .06); }
.wiki-sync-block-picker-item.is-selected {
  background: rgba(88, 166, 255, .12); border-color: #58a6ff;
}
.wiki-sync-block-picker-item-title { font-weight: 600; font-size: 13px; margin-bottom: 2px; }
.wiki-sync-block-picker-item-preview { font-size: 12px; color: #9da7b3; }
.wiki-sync-block-picker-item-meta { display: flex; gap: 8px; margin-top: 4px; font-size: 11px; color: #9da7b3; }
.wiki-sync-block-picker-footer {
  display: flex; justify-content: flex-end; gap: 8px; padding-top: 8px;
  border-top: 1px solid #30363d;
}
.wiki-sync-block-picker-footer button {
  padding: 4px 12px; border-radius: 6px; border: 1px solid #30363d;
  background: #0e1117; color: inherit; font-size: 12px; cursor: pointer;
}
.wiki-sync-block-picker-footer button:disabled { opacity: .5; cursor: not-allowed; }
.wiki-sync-block-picker-loading,
.wiki-sync-block-picker-empty { padding: 24px; text-align: center; font-size: 12px; color: #9da7b3; }
</style>
