<template>
  <section
    v-if="visible"
    class="wiki-recent-pages"
    :aria-label="$t('knowledgeEditor.wikiRecentPages.regionLabel')"
  >
    <header class="wiki-recent-pages__header">
      <t-icon name="history" size="14px" class="wiki-recent-pages__icon" />
      <span class="wiki-recent-pages__title">{{ $t('knowledgeEditor.wikiRecentPages.title') }}</span>
      <span class="wiki-recent-pages__count" aria-hidden="true">{{ pages.length }}</span>
    </header>
    <ul v-if="pages.length > 0" class="wiki-recent-pages__list">
      <li
        v-for="page in pages"
        :key="page.id"
        class="wiki-recent-pages__item"
        :class="{ 'wiki-recent-pages__item--active': page.slug === activeSlug }"
        @click="$emit('select', page.slug)"
      >
        <span class="wiki-recent-pages__item-title" :title="page.title">{{ truncateTitle(page.title) }}</span>
        <span v-if="page.category_path && page.category_path.length > 0" class="wiki-recent-pages__item-path">
          {{ page.category_path.join(' / ') }}
        </span>
      </li>
    </ul>
    <p v-else class="wiki-recent-pages__empty">
      {{ $t('knowledgeEditor.wikiRecentPages.empty') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const MAX_ITEMS = 5
const MAX_TITLE_LENGTH = 28

interface RecentPage {
  id: string
  slug: string
  title: string
  category_path?: string[]
}

const props = defineProps<{
  pages: RecentPage[]
  activeSlug?: string
}>()

defineEmits<{
  (e: 'select', slug: string): void
}>()

const { t } = useI18n()

// Cap to MAX_ITEMS most recent so the rail never grows unbounded.
const visible = computed(() => props.pages.length > 0)
const pages = computed(() => props.pages.slice(0, MAX_ITEMS))

function truncateTitle(title: string): string {
  if (!title) return ''
  return title.length > MAX_TITLE_LENGTH ? title.slice(0, MAX_TITLE_LENGTH - 1) + '…' : title
}
</script>

<style scoped>
.wiki-recent-pages {
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  background: var(--td-bg-color-container, #fff);
  margin: 8px 0;
  padding: 8px 10px 6px;
}
.wiki-recent-pages__header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  color: var(--td-text-color-secondary, #666);
  font-size: 12px;
  font-weight: 600;
}
.wiki-recent-pages__icon {
  flex: 0 0 auto;
}
.wiki-recent-pages__title {
  flex: 1 1 auto;
}
.wiki-recent-pages__count {
  background: var(--td-bg-color-secondarycontainer, #f3f3f3);
  color: var(--td-text-color-secondary, #666);
  border-radius: 999px;
  padding: 0 6px;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}
.wiki-recent-pages__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.wiki-recent-pages__item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 6px;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 120ms ease;
}
.wiki-recent-pages__item:hover {
  background: var(--td-bg-color-container-hover, #f5f5f5);
}
.wiki-recent-pages__item--active {
  background: var(--td-brand-color-light, #e6f4ff);
  color: var(--td-brand-color, #1677ff);
}
.wiki-recent-pages__item-title {
  font-size: 13px;
  font-weight: 500;
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.wiki-recent-pages__item-path {
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
  line-height: 1.2;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.wiki-recent-pages__empty {
  margin: 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder, #999);
}
</style>
