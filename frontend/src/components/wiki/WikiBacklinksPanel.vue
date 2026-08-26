<template>
  <section
    class="wiki-backlinks-panel"
    :data-loading="loading || graphLoading"
  >
    <button
      type="button"
      class="wiki-backlinks-panel__header"
      :aria-expanded="expanded"
      :aria-controls="bodyId"
      @click="toggle"
    >
      <span class="wiki-backlinks-panel__title">
        {{ $t('wiki.backlinks.title') }}
        <span v-if="summaryCounts" class="wiki-backlinks-panel__counts">
          <span class="wiki-backlinks-panel__chip wiki-backlinks-panel__chip--direct">
            {{ $t('wiki.backlinksGraph.sections.direct') }} {{ stats.direct_count }}
          </span>
          <span class="wiki-backlinks-panel__chip wiki-backlinks-panel__chip--indirect">
            {{ $t('wiki.backlinksGraph.sections.indirect') }} {{ stats.indirect_count }}
          </span>
          <span class="wiki-backlinks-panel__chip wiki-backlinks-panel__chip--related">
            {{ $t('wiki.backlinksGraph.sections.related') }} {{ stats.related_count }}
          </span>
          <span class="wiki-backlinks-panel__chip wiki-backlinks-panel__chip--broken">
            {{ $t('wiki.backlinksGraph.sections.broken') }} {{ stats.broken_count }}
          </span>
        </span>
      </span>
      <t-icon
        :name="expanded ? 'chevron-up' : 'chevron-down'"
        size="16px"
        class="wiki-backlinks-panel__chevron"
      />
    </button>

    <div v-if="expanded" :id="bodyId" class="wiki-backlinks-panel__body">
      <!-- Graph load degraded: fall back to Build #11 flat list -->
      <template v-if="graphError && !graph">
        <div class="wiki-backlinks-panel__load-failed">
          {{ $t('wiki.backlinksGraph.loadFailedToast') }}
        </div>
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
      </template>

      <!-- Graph view (Build #20): 4 collapsible sections -->
      <template v-else-if="graph">
        <div
          v-for="sid in sectionIds"
          :key="sid"
          class="wiki-backlinks-panel__section"
          :data-section="sid"
        >
          <button
            type="button"
            class="wiki-backlinks-panel__section-header"
            :aria-expanded="!collapse[sid]"
            :aria-controls="sectionBodyId(sid)"
            @click="toggleSection(sid)"
          >
            <span class="wiki-backlinks-panel__section-title">
              {{ $t(`wiki.backlinksGraph.sections.${sid}`) }}
              <span class="wiki-backlinks-panel__section-count">({{ countFor(sid) }})</span>
            </span>
            <t-icon
              :name="collapse[sid] ? 'chevron-down' : 'chevron-up'"
              size="14px"
              class="wiki-backlinks-panel__section-chevron"
            />
          </button>
          <div
            v-show="!collapse[sid]"
            :id="sectionBodyId(sid)"
            class="wiki-backlinks-panel__section-body"
          >
            <!-- Direct -->
            <template v-if="sid === 'direct'">
              <ul v-if="graph.direct.length > 0" class="wiki-backlinks-panel__list">
                <li
                  v-for="b in graph.direct"
                  :key="`direct-${b.slug}`"
                  class="wiki-backlinks-panel__item"
                  @click="onItemClick(b.slug)"
                >
                  <span class="wiki-backlinks-panel__item-title">{{ formatTitle(b) }}</span>
                  <span class="wiki-backlinks-panel__item-meta">
                    <span class="wiki-backlinks-panel__item-type">{{ b.page_type }}</span>
                    <span class="wiki-backlinks-panel__item-time">{{ formatTime(b.updated_at) }}</span>
                  </span>
                </li>
              </ul>
              <div v-else class="wiki-backlinks-panel__empty">
                {{ $t('wiki.backlinks.empty') }}
              </div>
            </template>
            <!-- Indirect: click navigates to `via` (D5) -->
            <template v-else-if="sid === 'indirect'">
              <ul v-if="graph.indirect.length > 0" class="wiki-backlinks-panel__list">
                <li
                  v-for="row in graph.indirect"
                  :key="`indirect-${row.slug}-${row.via}`"
                  class="wiki-backlinks-panel__item"
                  @click="onItemClick(row.via)"
                >
                  <span class="wiki-backlinks-panel__item-title">{{ formatTitle(row) }}</span>
                  <span class="wiki-backlinks-panel__item-meta">
                    <span class="wiki-backlinks-panel__item-via">
                      {{ $t('wiki.backlinksGraph.via', { slug: row.via }) }}
                    </span>
                  </span>
                </li>
              </ul>
              <div v-else class="wiki-backlinks-panel__empty">
                {{ $t('wiki.backlinks.empty') }}
              </div>
            </template>
            <!-- Related: jaccard chip, click navigates to slug -->
            <template v-else-if="sid === 'related'">
              <ul v-if="graph.related.length > 0" class="wiki-backlinks-panel__list">
                <li
                  v-for="row in graph.related"
                  :key="`related-${row.slug}`"
                  class="wiki-backlinks-panel__item"
                  @click="onItemClick(row.slug)"
                >
                  <span class="wiki-backlinks-panel__item-title">{{ formatTitle(row) }}</span>
                  <span class="wiki-backlinks-panel__item-meta">
                    <span class="wiki-backlinks-panel__item-jaccard">
                      {{ formatJaccard(row.jaccard) }}
                    </span>
                  </span>
                </li>
              </ul>
              <div v-else class="wiki-backlinks-panel__empty">
                {{ $t('wiki.backlinks.empty') }}
              </div>
            </template>
            <!-- Broken: read-only list with hint (no click handler) -->
            <template v-else-if="sid === 'broken'">
              <ul v-if="graph.broken.length > 0" class="wiki-backlinks-panel__list wiki-backlinks-panel__list--broken">
                <li
                  v-for="b in graph.broken"
                  :key="`broken-${b.target_slug}`"
                  class="wiki-backlinks-panel__item wiki-backlinks-panel__item--broken"
                >
                  <span class="wiki-backlinks-panel__item-broken-slug">
                    [[{{ b.target_slug }}]]
                  </span>
                  <span class="wiki-backlinks-panel__item-broken-hint">
                    {{ $t('wiki.backlinksGraph.brokenHint') }}
                  </span>
                </li>
              </ul>
              <div v-else class="wiki-backlinks-panel__empty">
                {{ $t('wiki.backlinks.empty') }}
              </div>
            </template>
          </div>
        </div>
        <div class="wiki-backlinks-panel__footer">
          <div class="wiki-backlinks-panel__cache-status">
            <span
              v-if="cacheStatusLabel"
              class="wiki-backlinks-panel__cache-status-time"
              :title="cacheStatusFullTime"
            >
              {{ $t('wiki.backlinksGraph.lastComputed') }}: {{ cacheStatusLabel }}
            </span>
            <span
              v-else
              class="wiki-backlinks-panel__cache-status-empty"
            >
              {{ $t('wiki.backlinksGraph.neverComputed') }}
            </span>
          </div>
          <button
            type="button"
            class="wiki-backlinks-panel__view-full-graph"
            @click="onViewFullGraph"
          >
            {{ $t('wiki.backlinksGraph.viewFullGraph') }}
          </button>
        </div>
      </template>

      <!-- Initial load before either request resolves -->
      <div
        v-else-if="(loading || graphLoading) && !graph && !hasCache"
        class="wiki-backlinks-panel__loading"
      >
        <t-skeleton :row="2" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import {
  type Backlink,
  backlinksVisibility,
} from '../../api/wiki/backlinksHelpers'
import type {
  WikiBacklinkGraph,
  WikiBacklinkGraphStats,
} from '../../api/wiki/backlinksGraphTypes'
import { useWikiBacklinksStore } from '../../stores/wikiBacklinks'
import {
  GRAPH_SECTION_IDS,
  backlinksBodyId,
  backlinksCountLabel,
  emptyStateHint,
  formatBacklinkTimestamp,
  formatJaccard,
  graphCollapseStorageKey,
  normaliseCollapseState,
  readGraphCollapseState,
  relativeTime,
  relativeTimeKey,
  writeGraphCollapseState,
  type GraphSectionId,
} from './wikiBacklinksPanelLogic'

const props = defineProps<{
  kbId: string
  slug: string
}>()

const emit = defineEmits<{
  (e: 'navigate', slug: string): void
  (e: 'view-full-graph', slug: string): void
}>()

const store = useWikiBacklinksStore()
const { t } = useI18n()

const bodyId = computed(() => backlinksBodyId(props.kbId, props.slug))
function sectionBodyId(sid: GraphSectionId): string {
  return `${bodyId.value}-${sid}`
}

// Build #11 state (preserved for the load-failed fallback path).
const list = computed<Backlink[] | undefined>(() => store.backlinksFor(props.kbId, props.slug))
const loading = computed(() => store.isLoading(props.kbId, props.slug))
const visibility = computed(() => backlinksVisibility(list.value))
const hasCache = computed(() => list.value !== undefined)

// Build #20 state.
const graph = computed<WikiBacklinkGraph | null>(() => store.graphFor(props.kbId, props.slug))
const graphLoading = computed(() => store.isGraphLoading(props.kbId, props.slug))
const graphError = computed(() => store.graphErrorFor(props.kbId, props.slug))

// Build #21 state — cache-status (last computed footer). Mirrors the
// stale-while-revalidate pattern from the graph layer above: we read
// whatever the store has and only re-fetch when the slug changes.
const cacheStatus = computed(() => store.cacheStatusFor(props.kbId, props.slug))
const cacheStatusLoading = computed(() => store.isCacheStatusLoading(props.kbId, props.slug))
const cacheStatusError = computed(() => store.cacheStatusErrorFor(props.kbId, props.slug))

/** i18n-friendly "5 minutes ago" / "3 days ago" form of computed_at.
 * Falls back to '' so the template can render the "never computed"
 * hint without an extra `v-if`. */
const cacheStatusLabel = computed(() => {
  const iso = cacheStatus.value?.computed_at
  if (!iso) return ''
  const rel = relativeTime(iso)
  if (!rel) return ''
  return t(`wiki.backlinksGraph.lastComputedUnits.${relativeTimeKey(rel.unit)}`, {
    n: rel.count,
  })
})

/** Absolute timestamp used as the title tooltip on the relative-time
 * chip — lets curious users hover to see the exact moment. */
const cacheStatusFullTime = computed(() => {
  const iso = cacheStatus.value?.computed_at
  if (!iso) return ''
  const ts = Date.parse(iso)
  if (!Number.isFinite(ts)) return ''
  return new Date(ts).toISOString()
})

// Reference cacheStatusError / cacheStatusLoading so Vue's reactivity
// tracks them — the footer re-renders if either flips, even though
// neither is rendered visually today.
void cacheStatusError
void cacheStatusLoading

const stats = computed<WikiBacklinkGraphStats>(() => {
  return (
    graph.value?.stats ?? {
      direct_count: 0,
      indirect_count: 0,
      related_count: 0,
      broken_count: 0,
      out_link_count: 0,
    }
  )
})

const summaryCounts = computed(() => Boolean(graph.value))

const sectionIds = GRAPH_SECTION_IDS

// Per-section collapse state, persisted to localStorage. Default:
// `direct` open, the other three collapsed so the strongest signal
// shows without flooding the sidebar.
const collapse = ref<Record<GraphSectionId, boolean>>(
  normaliseCollapseState(undefined),
)

const storageKey = graphCollapseStorageKey()

function loadCollapse(): void {
  if (typeof window === 'undefined') return
  try {
    collapse.value = readGraphCollapseState(window.localStorage)
  } catch {
    collapse.value = normaliseCollapseState(undefined)
  }
}

function saveCollapse(): void {
  if (typeof window === 'undefined') return
  try {
    writeGraphCollapseState(window.localStorage, collapse.value)
  } catch {
    // quota / private mode — silently ignore
  }
}

const expanded = ref(false)

const countLabel = computed(() => backlinksCountLabel(list.value))
const slugHint = computed(() => emptyStateHint(props.slug))

function toggle(): void {
  expanded.value = !expanded.value
  if (expanded.value) {
    void refresh()
  }
}

function toggleSection(sid: GraphSectionId): void {
  collapse.value = { ...collapse.value, [sid]: !collapse.value[sid] }
  saveCollapse()
}

function countFor(sid: GraphSectionId): number {
  switch (sid) {
    case 'direct':
      return stats.value.direct_count
    case 'indirect':
      return stats.value.indirect_count
    case 'related':
      return stats.value.related_count
    case 'broken':
      return stats.value.broken_count
  }
}

function formatTitle(b: { title?: string; slug: string }): string {
  const t = (b.title ?? '').trim()
  return t || b.slug
}

function formatTime(iso: string): string {
  return formatBacklinkTimestamp(iso)
}

function onItemClick(targetSlug: string): void {
  emit('navigate', targetSlug)
}

function onViewFullGraph(): void {
  emit('view-full-graph', props.slug)
}

async function refresh(): Promise<void> {
  // Always fetch the graph view; keep the flat list as a stale fallback
  // so a graph failure doesn't blank the panel. Build #21 also fans
  // out the cache-status call so the "last computed at" footer can
  // show its absolute timestamp.
  const graphPromise = store.loadBacklinkGraph(props.kbId, props.slug)
  const cacheStatusPromise = store.loadBacklinksCacheStatus(props.kbId, props.slug)
  if (!hasCache.value) {
    await store.loadBacklinks(props.kbId, props.slug)
  } else {
    void store.loadBacklinks(props.kbId, props.slug)
  }
  await graphPromise
  await cacheStatusPromise
}

watch(
  () => [props.kbId, props.slug],
  ([kb, slug]) => {
    if (!kb || !slug) return
    expanded.value = false
    void store.loadBacklinkGraph(kb, slug)
    void store.loadBacklinks(kb, slug)
    void store.loadBacklinksCacheStatus(kb, slug)
  },
  { immediate: true },
)

onMounted(() => {
  loadCollapse()
})
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
  flex-wrap: wrap;
}

.wiki-backlinks-panel__counts {
  display: inline-flex;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 11px;
  font-weight: 400;
}

.wiki-backlinks-panel__chip {
  padding: 1px 6px;
  border-radius: 8px;
  background: var(--td-component-hover);
  color: var(--td-text-color-secondary);
}

.wiki-backlinks-panel__chip--direct {
  background: rgba(0, 117, 255, 0.12);
  color: var(--td-brand-color, #0075ff);
}
.wiki-backlinks-panel__chip--indirect {
  background: rgba(0, 178, 162, 0.12);
  color: #00a78e;
}
.wiki-backlinks-panel__chip--related {
  background: var(--td-component-hover);
  color: var(--td-text-color-secondary);
}
.wiki-backlinks-panel__chip--broken {
  background: rgba(255, 153, 0, 0.14);
  color: #d97700;
}

.wiki-backlinks-panel__chevron {
  color: var(--td-text-color-secondary);
}

.wiki-backlinks-panel__body {
  padding-top: 8px;
}

.wiki-backlinks-panel__section {
  border-top: 1px dashed var(--td-component-stroke);
  margin-top: 6px;
  padding-top: 6px;
}

.wiki-backlinks-panel__section:first-child {
  border-top: none;
  margin-top: 0;
  padding-top: 0;
}

.wiki-backlinks-panel__section-header {
  width: 100%;
  background: transparent;
  border: none;
  padding: 2px 0;
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: center;
  font: inherit;
  color: inherit;
}

.wiki-backlinks-panel__section-title {
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
}

.wiki-backlinks-panel__section-count {
  margin-left: 4px;
  color: var(--td-text-color-secondary);
  font-weight: 400;
}

.wiki-backlinks-panel__section-chevron {
  color: var(--td-text-color-secondary);
}

.wiki-backlinks-panel__section-body {
  padding: 4px 0 4px 8px;
}

.wiki-backlinks-panel__empty {
  color: var(--td-text-color-secondary);
  font-size: 12px;
  padding: 4px 0;
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

.wiki-backlinks-panel__list--broken {
  border-left: 2px solid rgba(255, 153, 0, 0.4);
}

.wiki-backlinks-panel__item {
  padding: 6px 4px;
  border-radius: 4px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.wiki-backlinks-panel__item--broken {
  cursor: default;
  opacity: 0.85;
}

.wiki-backlinks-panel__item:hover {
  background: var(--td-component-hover);
}

.wiki-backlinks-panel__item--broken:hover {
  background: transparent;
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

.wiki-backlinks-panel__item-via {
  font-style: italic;
  color: var(--td-text-color-secondary);
}

.wiki-backlinks-panel__item-jaccard {
  background: var(--td-component-hover);
  padding: 1px 6px;
  border-radius: 8px;
  font-family: var(--td-font-family-mono, ui-monospace, monospace);
}

.wiki-backlinks-panel__item-broken-slug {
  font-family: var(--td-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
}

.wiki-backlinks-panel__item-broken-hint {
  font-size: 11px;
  color: var(--td-text-color-secondary);
}

.wiki-backlinks-panel__loading {
  padding: 8px 0;
}

.wiki-backlinks-panel__load-failed {
  font-size: 11px;
  color: var(--td-text-color-secondary);
  background: var(--td-component-hover);
  padding: 4px 6px;
  border-radius: 4px;
  margin-bottom: 6px;
}

.wiki-backlinks-panel__footer {
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 8px;
  padding-top: 6px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.wiki-backlinks-panel__cache-status {
  font-size: 11px;
  color: var(--td-text-color-secondary);
  min-width: 0;
  flex: 1 1 auto;
}

.wiki-backlinks-panel__cache-status-time {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.wiki-backlinks-panel__cache-status-empty {
  font-style: italic;
  opacity: 0.8;
}

.wiki-backlinks-panel__view-full-graph {
  background: transparent;
  border: none;
  cursor: pointer;
  font: inherit;
  color: var(--td-brand-color, #0075ff);
  font-size: 12px;
  padding: 0;
  flex: 0 0 auto;
}

.wiki-backlinks-panel__view-full-graph:hover {
  text-decoration: underline;
}
</style>