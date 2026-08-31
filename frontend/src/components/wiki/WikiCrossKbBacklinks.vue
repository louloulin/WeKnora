<!--
  WikiCrossKbBacklinks — Build #28 Cross-KB Backlinks panel section.

  Renders a collapsible per-KB accordion listing every page in any other
  knowledge base owned by the caller's tenant that links to the current
  page. Designed to drop into the existing WikiBacklinksPanel below the
  direct / indirect / related / broken chips; the parent owns the
  accordion open/close state via the `expanded` prop.

  Emits:
    navigate(kbId, slug) — click handler the parent routes to the
      cross-KB reader; we don't navigate from inside the section so
      the parent can keep its analytics / history stack consistent.
-->

<template>
  <section class="wiki-cross-kb-backlinks" :data-loading="loading">
    <header class="wiki-cross-kb-backlinks-header">
      <h4 class="wiki-cross-kb-backlinks-title">
        {{ titleLabel }}
        <span v-if="response" class="wiki-cross-kb-backlinks-total">
          {{ response.total }} {{ response.total === 1 ? oneRefLabel : manyRefsLabel }}
        </span>
      </h4>
      <button
        v-if="response && response.total > 0"
        type="button"
        class="wiki-cross-kb-backlinks-toggle"
        :aria-expanded="expanded"
        @click="$emit('toggle')"
      >
        {{ expanded ? collapseLabel : expandLabel }}
      </button>
    </header>
    <div v-if="loading" class="wiki-cross-kb-backlinks-loading">{{ loadingLabel }}</div>
    <div v-else-if="!response || response.total === 0" class="wiki-cross-kb-backlinks-empty">
      {{ emptyLabel }}
    </div>
    <ul v-else-if="expanded" class="wiki-cross-kb-backlinks-groups">
      <li
        v-for="group in response.groups"
        :key="group.knowledge_base_id"
        class="wiki-cross-kb-backlinks-group"
      >
        <header class="wiki-cross-kb-backlinks-group-header">
          <span class="wiki-cross-kb-backlinks-kb-name">{{ group.kb_name }}</span>
          <span class="wiki-cross-kb-backlinks-kb-count">{{ group.total }}</span>
        </header>
        <ul class="wiki-cross-kb-backlinks-list">
          <li
            v-for="link in group.backlinks"
            :key="group.knowledge_base_id + ':' + link.slug"
            class="wiki-cross-kb-backlinks-item"
          >
            <button
              type="button"
              class="wiki-cross-kb-backlinks-link"
              :title="link.title || link.slug"
              @click="$emit('navigate', group.knowledge_base_id, link.slug)"
            >
              <span class="wiki-cross-kb-backlinks-item-title">{{ link.title || link.slug }}</span>
              <span class="wiki-cross-kb-backlinks-item-slug">/{{ link.slug }}</span>
              <span class="wiki-cross-kb-backlinks-item-status" :data-status="link.status">
                {{ link.status }}
              </span>
            </button>
          </li>
        </ul>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  listCrossKbBacklinks,
  type WikiBacklinkCrossKBResponse,
} from '../../api/wiki/crossKbBacklinks'

const props = defineProps<{
  kbId: string
  slug: string
  expanded: boolean
}>()

defineEmits<{
  (e: 'toggle'): void
  (e: 'navigate', kbId: string, slug: string): void
}>()

const loading = ref(false)
const response = ref<WikiBacklinkCrossKBResponse | null>(null)

const titleLabel = 'Cross-KB backlinks'
const oneRefLabel = 'reference'
const manyRefsLabel = 'references'
const emptyLabel = 'No pages in other knowledge bases reference this page yet.'
const loadingLabel = 'Loading cross-KB backlinks…'
const expandLabel = 'Show'
const collapseLabel = 'Hide'

async function load() {
  if (!props.kbId || !props.slug) return
  loading.value = true
  try {
    response.value = await listCrossKbBacklinks(props.kbId, props.slug, { limit: 50 })
  } catch (e) {
    response.value = { groups: [], total: 0 }
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.wiki-cross-kb-backlinks {
  border-top: 1px solid var(--color-border, #e5e7eb);
  padding-top: 8px;
  margin-top: 8px;
  font-size: 13px;
}
.wiki-cross-kb-backlinks-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.wiki-cross-kb-backlinks-title {
  font-size: 13px;
  font-weight: 600;
  margin: 0;
}
.wiki-cross-kb-backlinks-total {
  color: var(--color-text-secondary, #6b7280);
  font-weight: 400;
  margin-left: 6px;
}
.wiki-cross-kb-backlinks-toggle {
  background: transparent;
  border: 1px solid var(--color-border, #d1d5db);
  border-radius: 4px;
  padding: 0 8px;
  cursor: pointer;
  font-size: 12px;
}
.wiki-cross-kb-backlinks-empty,
.wiki-cross-kb-backlinks-loading {
  color: var(--color-text-secondary, #6b7280);
  padding: 6px 0;
  font-size: 12px;
}
.wiki-cross-kb-backlinks-groups {
  list-style: none;
  padding: 0;
  margin: 4px 0 0 0;
}
.wiki-cross-kb-backlinks-group {
  margin-bottom: 6px;
  border: 1px solid var(--color-border, #e5e7eb);
  border-radius: 4px;
}
.wiki-cross-kb-backlinks-group-header {
  display: flex;
  justify-content: space-between;
  padding: 4px 8px;
  background: var(--color-bg-soft, #f9fafb);
  font-weight: 600;
}
.wiki-cross-kb-backlinks-kb-name {
  font-size: 12px;
}
.wiki-cross-kb-backlinks-kb-count {
  font-size: 11px;
  color: var(--color-text-secondary, #6b7280);
}
.wiki-cross-kb-backlinks-list {
  list-style: none;
  padding: 0;
  margin: 0;
}
.wiki-cross-kb-backlinks-item {
  padding: 2px 0;
}
.wiki-cross-kb-backlinks-link {
  width: 100%;
  text-align: left;
  background: transparent;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
}
.wiki-cross-kb-backlinks-link:hover {
  background: var(--color-bg-hover, #f3f4f6);
}
.wiki-cross-kb-backlinks-item-title {
  font-weight: 500;
}
.wiki-cross-kb-backlinks-item-slug {
  color: var(--color-text-secondary, #6b7280);
  font-size: 11px;
  font-family: monospace;
}
.wiki-cross-kb-backlinks-item-status {
  font-size: 11px;
  color: var(--color-text-secondary, #6b7280);
  border: 1px solid var(--color-border, #e5e7eb);
  border-radius: 3px;
  padding: 0 4px;
}
.wiki-cross-kb-backlinks-item-status[data-status="archived"] {
  border-color: #f59e0b;
  color: #b45309;
}
</style>
