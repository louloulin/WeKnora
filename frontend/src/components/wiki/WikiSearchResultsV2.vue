<script setup lang="ts">
/**
 * Build #19 / P2.x.a — WikiSearchResultsV2.
 *
 * Renders hits from the v2 search endpoint. The `snippet` field already
 * contains server-rendered `<mark>` HTML from `ts_headline`, so the
 * component renders it directly via `v-html` rather than re-highlighting
 * client-side. The title still uses the v2 hit's text verbatim because
 * the server ranks title hits higher.
 *
 * Cross-KB results carry `kb_id` / `kb_name` — clicking a hit navigates
 * to the wiki page within the right KB.
 */
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { WikiSearchV2Hit } from '../../api/wiki/searchV2Types'

const props = defineProps<{
  hits: WikiSearchV2Hit[]
  loading: boolean
  error: string | null
  query: string
  usedLegacy?: boolean
}>()

const emit = defineEmits<{
  (e: 'select', hit: WikiSearchV2Hit): void
}>()

const router = useRouter()
const { t } = useI18n()

function onSelect(hit: WikiSearchV2Hit) {
  emit('select', hit)
  // Cross-KB results need to navigate into the right KB first.
  const target = `/wiki/page/${encodeURIComponent(hit.slug)}?kb_id=${encodeURIComponent(hit.kb_id)}`
  void router.push(target)
}

function pageTypeLabel(type: string): string {
  const key = `wiki.searchV2.pageType.${type}`
  const translated = t(key)
  return translated === key ? type : translated
}
</script>

<template>
  <div class="wiki-search-results-v2" role="listbox"
    :aria-label="t('wiki.searchV2.resultsLabel')">
    <!-- loading -->
    <div v-if="loading" class="wiki-search-state wiki-search-state--loading">
      <t-loading size="small" />
      <span class="wiki-search-state-text">{{ t('wiki.searchV2.loading') }}</span>
    </div>

    <!-- error -->
    <div v-else-if="error" class="wiki-search-state wiki-search-state--error">
      <t-icon name="error-circle" />
      <span class="wiki-search-state-text">{{ t('wiki.searchV2.error') }}</span>
    </div>

    <!-- soft fallback hint -->
    <div v-else-if="usedLegacy" class="wiki-search-state wiki-search-state--fallback">
      <t-icon name="info-circle" />
      <span class="wiki-search-state-text">{{ t('wiki.searchV2.fallback') }}</span>
    </div>

    <!-- empty -->
    <div v-else-if="hits.length === 0"
      class="wiki-search-state wiki-search-state--empty">
      <t-icon name="search" />
      <span class="wiki-search-state-text">{{ t('wiki.searchV2.empty') }}</span>
    </div>

    <!-- list -->
    <ul v-else class="wiki-search-list">
      <li v-for="(hit, idx) in hits" :key="`${hit.kb_id}:${hit.slug}:${idx}`"
        class="wiki-search-item"
        role="option" tabindex="0"
        :aria-selected="false"
        @click="onSelect(hit)"
        @keydown.enter.prevent="onSelect(hit)"
        @keydown.space.prevent="onSelect(hit)">
        <div class="wiki-search-item-meta">
          <span class="wiki-search-item-kb" v-if="hit.kb_name">
            <t-icon name="root-list" size="12px" />
            <span>{{ hit.kb_name }}</span>
          </span>
          <span class="wiki-search-item-type" :data-type="hit.page_type">
            {{ pageTypeLabel(hit.page_type) }}
          </span>
          <span class="wiki-search-item-score" :title="`score: ${hit.score.toFixed(4)}`">
            <t-icon name="star" size="12px" />
            <span>{{ hit.score.toFixed(3) }}</span>
          </span>
        </div>
        <div class="wiki-search-item-title">{{ hit.title }}</div>
        <!-- snippet carries server-rendered <mark>; v-html is intentional -->
        <div class="wiki-search-item-snippet" v-html="hit.snippet" />
      </li>
    </ul>
  </div>
</template>

<style scoped>
.wiki-search-results-v2 {
  width: 100%;
  max-height: 480px;
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

.wiki-search-state--fallback {
  color: var(--td-text-color-placeholder, #999);
  font-style: italic;
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

.wiki-search-item-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
}

.wiki-search-item-kb {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  background: var(--td-bg-color-secondarycontainer, #f3f3f3);
  padding: 1px 6px;
  border-radius: 3px;
}

.wiki-search-item-type {
  background: var(--td-brand-color-light, #e6f4ff);
  color: var(--td-brand-color, #1d6cb1);
  padding: 1px 6px;
  border-radius: 3px;
  text-transform: capitalize;
}

.wiki-search-item-score {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 2px;
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

:deep(mark) {
  background: var(--td-warning-color-1, #fff3e0);
  color: var(--td-text-color-primary, #181818);
  font-weight: 600;
  padding: 0 2px;
  border-radius: 2px;
}
</style>