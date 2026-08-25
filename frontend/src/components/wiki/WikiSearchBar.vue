<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { useWikiSearchStore } from '../../stores/wikiSearch'
import WikiSearchResults from './WikiSearchResults.vue'
import type { WikiSearchResult } from '../../api/wiki/search'

const props = defineProps<{
  kbId: string
}>()

const store = useWikiSearchStore()
const { query, results, loading, error, showResults } = storeToRefs(store)

const { t } = useI18n()

const inputRef = ref<HTMLInputElement | null>(null)
const wrapperRef = ref<HTMLElement | null>(null)

const visibleResults = computed(() => (showResults.value ? results.value : []))
const visibleError = computed(() => (showResults.value ? error.value : null))

function onInput(value: string) {
  store.setQuery(value)
  store.openResults()
  store.scheduleSearch(props.kbId)
}

function onClear() {
  store.reset()
}

function onKeyDown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (showResults.value) {
      store.closeResults()
      e.preventDefault()
    }
  }
}

function onSelect(result: WikiSearchResult) {
  store.closeResults()
  void nextTick(() => {
    inputRef.value?.focus()
  })
  // The router push happens inside WikiSearchResults. We only need to
  // log to history here for the click event; navigation is handled.
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

watch(() => props.kbId, (next) => {
  store.setKbId(next)
})

const placeholder = computed(() => t('wiki.search.placeholder'))
const ariaLabel = computed(() => t('wiki.search.ariaLabel'))
</script>

<template>
  <div ref="wrapperRef" class="wiki-search-bar">
    <t-input :value="query" :placeholder="placeholder" :aria-label="ariaLabel" clearable
      ref="inputRef" @input="(v: string) => onInput(v)" @clear="onClear" @keydown="onKeyDown"
      class="wiki-search-bar-input">
      <template #prefixIcon>
        <t-icon name="search" />
      </template>
    </t-input>

    <div v-if="showResults" class="wiki-search-bar-popup">
      <WikiSearchResults :results="visibleResults" :loading="loading" :error="visibleError"
        :query="query" @select="onSelect" />
    </div>
  </div>
</template>

<style scoped>
.wiki-search-bar {
  position: relative;
  width: 100%;
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