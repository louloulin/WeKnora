<template>
  <section class="wiki-backlinks-panel" :data-loading="loading">
    <button
      type="button"
      class="wiki-backlinks-panel__header"
      :aria-expanded="expanded"
      :aria-controls="bodyId"
      @click="toggle"
    >
      <span class="wiki-backlinks-panel__title">
        {{ $t('wiki.backlinks.title') }}
        <span v-if="countLabel" class="wiki-backlinks-panel__count">
          {{ countLabel }}
        </span>
      </span>
      <t-icon
        :name="expanded ? 'chevron-up' : 'chevron-down'"
        size="16px"
        class="wiki-backlinks-panel__chevron"
      />
    </button>

    <div v-if="expanded" :id="bodyId" class="wiki-backlinks-panel__body">
      <div v-if="!hasCache && loading" class="wiki-backlinks-panel__loading">
        <t-skeleton :row="2" />
      </div>

      <div v-else-if="list && list.length === 0" class="wiki-backlinks-panel__empty">
        <div>{{ $t('wiki.backlinks.empty') }}</div>
        <div class="wiki-backlinks-panel__empty-hint">
          {{ $t('wiki.backlinks.emptyHint', { slug: slugHint }) }}
        </div>
      </div>

      <ul v-else-if="list && list.length > 0" class="wiki-backlinks-panel__list">
        <li
          v-for="b in list"
          :key="b.slug"
          class="wiki-backlinks-panel__item"
          @click="onItemClick(b.slug)"
        >
          <span class="wiki-backlinks-panel__item-title">{{ formatTitle(b) }}</span>
          <span class="wiki-backlinks-panel__item-meta">
            <span class="wiki-backlinks-panel__item-type">{{ b.pageType }}</span>
            <span class="wiki-backlinks-panel__item-time">{{ formatTime(b.updatedAt) }}</span>
          </span>
        </li>
      </ul>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import {
  type Backlink,
  backlinksVisibility,
  formatBacklinkTitle,
} from '../../api/wiki/backlinksHelpers'
import { useWikiBacklinksStore } from '../../stores/wikiBacklinks'
import {
  backlinksBodyId,
  backlinksCountLabel,
  emptyStateHint,
  formatBacklinkTimestamp,
} from './wikiBacklinksPanelLogic'

const props = defineProps<{
  kbId: string
  slug: string
}>()

const emit = defineEmits<{
  (e: 'navigate', slug: string): void
}>()

const store = useWikiBacklinksStore()

const bodyId = computed(() => backlinksBodyId(props.kbId, props.slug))

const list = computed<Backlink[] | undefined>(() => store.backlinksFor(props.kbId, props.slug))
const loading = computed(() => store.isLoading(props.kbId, props.slug))
const visibility = computed(() => backlinksVisibility(list.value))
const hasCache = computed(() => list.value !== undefined)

const expanded = ref(false)

const countLabel = computed(() => backlinksCountLabel(list.value))
const slugHint = computed(() => emptyStateHint(props.slug))

function toggle(): void {
  expanded.value = !expanded.value
  if (expanded.value) {
    void refresh()
  }
}

function formatTitle(b: Backlink): string {
  return formatBacklinkTitle(b)
}

function formatTime(iso: string): string {
  return formatBacklinkTimestamp(iso)
}

function onItemClick(targetSlug: string): void {
  emit('navigate', targetSlug)
}

async function refresh(): Promise<void> {
  // Stale-while-revalidate: if we already have a cached list, keep
  // showing it and refresh in the background.
  if (!hasCache.value) {
    // No cache yet — kick a fetch and the computed `list` will
    // populate once it resolves. The loading skeleton handles the
    // empty intermediate state.
    await store.loadBacklinks(props.kbId, props.slug)
  } else {
    void store.loadBacklinks(props.kbId, props.slug)
  }
}

// When the user navigates to a different page, reset expansion and
// fire a fresh load (the cache key changes, so prior state is not
// visible — but we proactively kick a load so the next expand is
// instant).
watch(
  () => [props.kbId, props.slug],
  ([kb, slug]) => {
    if (!kb || !slug) return
    expanded.value = false
    void store.loadBacklinks(kb, slug)
  },
  { immediate: true },
)
</script>

<style scoped>
.wiki-backlinks-panel {
  border-top: 1px solid var(--td-component-stroke);
  padding-top: 12px;
  margin-top: 12px;
}

.wiki-backlinks-panel__header {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: transparent;
  border: none;
  padding: 4px 0;
  cursor: pointer;
  font: inherit;
  color: inherit;
}

.wiki-backlinks-panel__title {
  font-weight: 600;
  font-size: 13px;
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
}

.wiki-backlinks-panel__count {
  color: var(--td-text-color-secondary);
  font-weight: 400;
  font-size: 12px;
}

.wiki-backlinks-panel__chevron {
  color: var(--td-text-color-secondary);
}

.wiki-backlinks-panel__body {
  padding-top: 8px;
}

.wiki-backlinks-panel__empty {
  color: var(--td-text-color-secondary);
  font-size: 12px;
  padding: 8px 0;
}

.wiki-backlinks-panel__empty-hint {
  margin-top: 4px;
  font-family: var(--td-font-family-mono, ui-monospace, monospace);
  font-size: 11px;
  opacity: 0.85;
}

.wiki-backlinks-panel__list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.wiki-backlinks-panel__item {
  padding: 6px 4px;
  border-radius: 4px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.wiki-backlinks-panel__item:hover {
  background: var(--td-component-hover);
}

.wiki-backlinks-panel__item-title {
  font-size: 13px;
  color: var(--td-text-color-primary);
}

.wiki-backlinks-panel__item-meta {
  display: inline-flex;
  gap: 8px;
  font-size: 11px;
  color: var(--td-text-color-secondary);
}

.wiki-backlinks-panel__item-type {
  font-family: var(--td-font-family-mono, ui-monospace, monospace);
}

.wiki-backlinks-panel__loading {
  padding: 8px 0;
}
</style>