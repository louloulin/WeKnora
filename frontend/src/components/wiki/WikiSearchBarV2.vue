<script setup lang="ts">
/**
 * Build #19 / P2.x.a — WikiSearchBarV2.
 *
 * Toolbar search bar that drives the v2 search endpoint. Adds the
 * cross-KB chip row on top of the input so the user can scope the
 * query to a subset of accessible KBs. Default scope is the current
 * KB only — the cross-KB default ("all tenant KBs") is a Build
 * #19.x concern once KB-level ACL ships.
 *
 * The popup renders `WikiSearchResultsV2`, which consumes server-
 * rendered `<mark>` snippets directly.
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { useWikiSearchV2Store } from '../../stores/wikiSearchV2'
import WikiSearchResultsV2 from './WikiSearchResultsV2.vue'
import type { WikiSearchV2Hit } from '../../api/wiki/searchV2Types'

interface KBOption {
  id: string
  name: string
}

const props = defineProps<{
  kbId: string
  /** Optional KB list for the chip row. Empty / undefined hides chips. */
  kbOptions?: KBOption[]
}>()

const store = useWikiSearchV2Store()
const { query, hits, loading, error, showResults, usedLegacy } = storeToRefs(store)

const { t } = useI18n()

const inputRef = ref<HTMLInputElement | null>(null)
const wrapperRef = ref<HTMLElement | null>(null)

const selectedKBIds = ref<string[]>([])

const visibleHits = computed(() => (showResults.value ? hits.value : []))
const visibleError = computed(() => (showResults.value ? error.value : null))
const visibleLegacy = computed(() => (showResults.value ? usedLegacy.value : false))

const hasKBOptions = computed(() => props.kbOptions && props.kbOptions.length > 0)
const showChips = hasKBOptions

watch(
  () => props.kbId,
  (next) => {
    selectedKBIds.value = next ? [next] : []
  },
  { immediate: true },
)

function toggleKB(id: string) {
  const idx = selectedKBIds.value.indexOf(id)
  if (idx >= 0) {
    selectedKBIds.value.splice(idx, 1)
  } else {
    selectedKBIds.value.push(id)
  }
  // Re-run the search with the new KB scope.
  if (query.value.trim().length > 0) {
    store.scheduleSearch(props.kbId, selectedKBIds.value, [])
  }
}

function onInput(value: string) {
  store.setQuery(value)
  store.openResults()
  store.scheduleSearch(props.kbId, selectedKBIds.value, [])
}

function onClear() {
  store.reset()
}

function onKeyDown(e: KeyboardEvent) {
  if (e.key === 'Escape' && showResults.value) {
    store.closeResults()
    e.preventDefault()
  }
}

function onSelect(_hit: WikiSearchV2Hit) {
  store.closeResults()
  void nextTick(() => {
    inputRef.value?.focus()
  })
}

function onClickOutside(e: MouseEvent) {
  if (!wrapperRef.value) return
  if (!wrapperRef.value.contains(e.target as Node)) {
    store.closeResults()
  }
}

onMounted(() => {
  document.addEventListener('mousedown', onClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onClickOutside)
})

const placeholder = computed(() => t('wiki.searchV2.placeholder'))
const ariaLabel = computed(() => t('wiki.searchV2.ariaLabel'))
const chipLabel = computed(() => t('wiki.searchV2.kbChip', { count: selectedKBIds.value.length }))
</script>

<template>
  <div ref="wrapperRef" class="wiki-search-bar-v2">
    <div v-if="showChips" class="wiki-search-bar-kbs">
      <span class="wiki-search-bar-kbs-label">{{ chipLabel }}</span>
      <div class="wiki-search-bar-kb-chips">
        <button
          v-for="kb in kbOptions"
          :key="kb.id"
          type="button"
          class="wiki-search-bar-kb-chip"
          :class="{ 'is-selected': selectedKBIds.includes(kb.id) }"
          :aria-pressed="selectedKBIds.includes(kb.id)"
          @click="toggleKB(kb.id)"
        >
          {{ kb.name }}
        </button>
      </div>
    </div>

    <t-input
      :value="query"
      :placeholder="placeholder"
      :aria-label="ariaLabel"
      clearable
      ref="inputRef"
      @input="(v: string) => onInput(v)"
      @clear="onClear"
      @keydown="onKeyDown"
      class="wiki-search-bar-input"
    >
      <template #prefixIcon>
        <t-icon name="search" />
      </template>
    </t-input>

    <div v-if="showResults" class="wiki-search-bar-popup">
      <WikiSearchResultsV2
        :hits="visibleHits"
        :loading="loading"
        :error="visibleError"
        :used-legacy="visibleLegacy"
        :query="query"
        @select="onSelect"
      />
    </div>
  </div>
</template>

<style scoped>
.wiki-search-bar-v2 {
  position: relative;
  width: 100%;
}

.wiki-search-bar-kbs {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}

.wiki-search-bar-kbs-label {
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
  white-space: nowrap;
}

.wiki-search-bar-kb-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.wiki-search-bar-kb-chip {
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  background: var(--td-bg-color-container, #fff);
  color: var(--td-text-color-secondary, #666);
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.wiki-search-bar-kb-chip:hover {
  border-color: var(--td-brand-color, #1d6cb1);
  color: var(--td-brand-color, #1d6cb1);
}

.wiki-search-bar-kb-chip.is-selected {
  background: var(--td-brand-color-light, #e6f4ff);
  border-color: var(--td-brand-color, #1d6cb1);
  color: var(--td-brand-color, #1d6cb1);
  font-weight: 600;
}

.wiki-search-bar-input {
  width: 100%;
}

.wiki-search-bar-popup {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  width: 100%;
  z-index: 9999;
}
</style>