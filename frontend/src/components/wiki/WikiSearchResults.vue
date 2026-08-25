<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { WikiSearchResult } from '../../api/wiki/search'
import { highlight, buildSnippet } from '../../utils/wikiSearch'

const props = defineProps<{
  results: WikiSearchResult[]
  loading: boolean
  error: string | null
  query: string
}>()

const emit = defineEmits<{
  (e: 'select', result: WikiSearchResult): void
}>()

const router = useRouter()
const { t } = useI18n()

const keywords = computed(() =>
  props.query
    .trim()
    .toLowerCase()
    .split(/\s+/u)
    .filter((kw) => kw.length > 0),
)

function onSelect(result: WikiSearchResult) {
  emit('select', result)
  void router.push(`/wiki/page/${encodeURIComponent(result.slug)}`)
}

function highlightedTitle(r: WikiSearchResult): string {
  return highlight(r.title, keywords.value)
}

function highlightedSnippet(r: WikiSearchResult): string {
  // Prefer the backend-provided snippet, fall back to the local one.
  return highlight(r.snippet || buildSnippet(r.title + ' ' + (r.path?.join(' ') ?? ''), keywords.value), keywords.value)
}

function breadcrumbs(path: string[]): string {
  return path.join(' / ')
}
</script>

<template>
  <div class="wiki-search-results" role="listbox"
    :aria-label="t('wiki.search.resultsLabel')">
    <!-- loading -->
    <div v-if="loading" class="wiki-search-state wiki-search-state--loading">
      <t-loading size="small" />
      <span class="wiki-search-state-text">{{ t('wiki.search.loading') }}</span>
    </div>

    <!-- error -->
    <div v-else-if="error" class="wiki-search-state wiki-search-state--error">
      <t-icon name="error-circle" />
      <span class="wiki-search-state-text">{{ t('wiki.search.error') }}</span>
    </div>

    <!-- empty -->
    <div v-else-if="results.length === 0"
      class="wiki-search-state wiki-search-state--empty">
      <t-icon name="search" />
      <span class="wiki-search-state-text">{{ t('wiki.search.empty') }}</span>
    </div>

    <!-- list -->
    <ul v-else class="wiki-search-list">
      <li v-for="r in results" :key="r.pageId" class="wiki-search-item"
        role="option" tabindex="0"
        :aria-selected="false"
        @click="onSelect(r)"
        @keydown.enter.prevent="onSelect(r)"
        @keydown.space.prevent="onSelect(r)">
        <div class="wiki-search-item-path">
          <t-icon name="folder" size="12px" />
          <span>{{ breadcrumbs(r.path) }}</span>
        </div>
        <div class="wiki-search-item-title" v-html="highlightedTitle(r)" />
        <div class="wiki-search-item-snippet" v-html="highlightedSnippet(r)" />
        <div class="wiki-search-item-score" :title="`score: ${r.score}`">
          <t-icon name="star" size="12px" />
          <span>{{ r.score }}</span>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.wiki-search-results {
  width: 100%;
  max-height: 420px;
  overflow-y: auto;
  background: var(--td-bg-color-container, #fff);
  border-radius: 6px;
  box-shadow: var(--td-shadow-3, 0 4px 16px rgba(0, 0, 0, 0.08));
  padding: 4px 0;
}

.wiki-search-state {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 20px;
  color: var(--td-text-color-secondary, #666);
  font-size: 13px;
}

.wiki-search-state--error {
  color: var(--td-error-color, #d54941);
}

.wiki-search-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.wiki-search-item {
  position: relative;
  padding: 10px 16px;
  cursor: pointer;
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
  transition: background 0.15s ease;
}

.wiki-search-item:last-child {
  border-bottom: none;
}

.wiki-search-item:hover,
.wiki-search-item:focus {
  background: var(--td-bg-color-container-hover, #f3f3f3);
  outline: none;
}

.wiki-search-item-path {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
  margin-bottom: 4px;
}

.wiki-search-item-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary, #181818);
  margin-bottom: 4px;
  line-height: 1.4;
}

.wiki-search-item-snippet {
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
  line-height: 1.5;
}

.wiki-search-item-score {
  position: absolute;
  top: 10px;
  right: 16px;
  display: flex;
  align-items: center;
  gap: 2px;
  font-size: 11px;
  color: var(--td-text-color-placeholder, #aaa);
}

:deep(mark) {
  background: var(--td-warning-color-1, #fff3e0);
  color: var(--td-text-color-primary, #181818);
  font-weight: 600;
  padding: 0 2px;
  border-radius: 2px;
}
</style>