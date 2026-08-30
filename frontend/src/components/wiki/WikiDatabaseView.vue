<template>
  <div class="wiki-database-view">
    <header class="wiki-database-view__header">
      <div class="wiki-database-view__title-row">
        <t-icon name="table" size="16px" />
        <h2 class="wiki-database-view__title">{{ $t('knowledgeEditor.wikiDatabaseView.title') }}</h2>
        <span class="wiki-database-view__count">{{ $t('knowledgeEditor.wikiDatabaseView.countLabel', {
          count: filteredPages.length,
          total: pages.length,
        }) }}</span>
      </div>
      <div class="wiki-database-view__toolbar">
        <t-input
          v-model="filterText"
          :placeholder="$t('knowledgeEditor.wikiDatabaseView.filterPlaceholder')"
          clearable
          size="small"
        >
          <template #prefixIcon><t-icon name="search" /></template>
        </t-input>
        <t-select
          v-model="filterType"
          size="small"
          :placeholder="$t('knowledgeEditor.wikiDatabaseView.typeFilter')"
          clearable
        >
          <t-option v-for="t in knownPageTypes" :key="t" :value="t" :label="t" />
        </t-select>
      </div>
    </header>

    <div v-if="loading && pages.length === 0" class="wiki-database-view__skeleton">
      <div v-for="n in 5" :key="'skel-' + n" class="wiki-database-view__skel-row">
        <t-skeleton :row-col="[{ width: '40%' }, { width: '20%' }, { width: '20%' }]" />
      </div>
    </div>

    <EmptyState
      v-else-if="pages.length === 0"
      icon="table"
      :title="$t('knowledgeEditor.wikiDatabaseView.emptyTitle')"
      :description="$t('knowledgeEditor.wikiDatabaseView.emptyDesc')"
    />

    <EmptyState
      v-else-if="filteredPages.length === 0"
      icon="search"
      :title="$t('knowledgeEditor.wikiDatabaseView.noMatchTitle')"
      :description="$t('knowledgeEditor.wikiDatabaseView.noMatchDesc')"
    />

    <div v-else class="wiki-database-view__table-wrap">
      <table class="wiki-database-view__table" role="grid">
        <thead>
          <tr>
            <th
              v-for="col in columns"
              :key="col.id"
              :class="['wiki-database-view__th', { 'wiki-database-view__th--active': sortKey === col.id }]"
              :aria-sort="sortKey === col.id ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'"
              @click="onSort(col.id)"
            >
              <span>{{ col.label }}</span>
              <t-icon
                v-if="sortKey === col.id"
                :name="sortDir === 'asc' ? 'chevron-up' : 'chevron-down'"
                size="12px"
              />
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="page in filteredPages"
            :key="page.id"
            class="wiki-database-view__tr"
            @click="$emit('select', page.slug)"
          >
            <td class="wiki-database-view__td wiki-database-view__td--title">
              <span class="wiki-database-view__title-link">{{ page.title }}</span>
              <span v-if="page.category_path && page.category_path.length > 0" class="wiki-database-view__path">
                {{ page.category_path.join(' / ') }}
              </span>
            </td>
            <td class="wiki-database-view__td">
              <span class="wiki-database-view__chip">{{ page.page_type }}</span>
            </td>
            <td class="wiki-database-view__td">
              <span class="wiki-database-view__chip wiki-database-view__chip--status">
                {{ page.status }}
              </span>
            </td>
            <td
              v-for="col in propertyColumns"
              :key="col.id"
              class="wiki-database-view__td"
            >
              <span v-if="getCellValue(page, col.id) === null || getCellValue(page, col.id) === undefined" class="wiki-database-view__empty-cell">—</span>
              <span v-else class="wiki-database-view__cell">
                {{ formatCell(col, getCellValue(page, col.id)) }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/EmptyState.vue'
import {
  DEFAULT_PROPERTY_SCHEMA,
  formatPropertyValue,
  readPropertyValues,
  type PropertyValue,
  type WikiProperty,
} from './wikiPropertySchema'
import type { WikiPage } from '@/api/wiki'

type SortDir = 'asc' | 'desc'

interface ColumnDef {
  id: string
  label: string
}

const props = defineProps<{
  pages: WikiPage[]
  loading?: boolean
}>()

const emit = defineEmits<{
  (e: 'select', slug: string): void
}>()

const { t } = useI18n()

const filterText = ref('')
const filterType = ref<string>('')
const sortKey = ref<string>('title')
const sortDir = ref<SortDir>('asc')

const knownPageTypes = computed(() => {
  const set = new Set<string>()
  for (const p of props.pages) {
    if (p.page_type) set.add(p.page_type)
  }
  return Array.from(set).sort()
})

const propertyColumns = computed<WikiProperty[]>(() => DEFAULT_PROPERTY_SCHEMA)

const columns = computed<ColumnDef[]>(() => [
  { id: 'title', label: t('knowledgeEditor.wikiDatabaseView.colTitle') },
  { id: 'page_type', label: t('knowledgeEditor.wikiDatabaseView.colType') },
  { id: 'status', label: t('knowledgeEditor.wikiDatabaseView.colStatus') },
  ...propertyColumns.value.map((p) => ({ id: p.id, label: p.name })),
])

function matchesFilter(page: WikiPage): boolean {
  if (filterType.value && page.page_type !== filterType.value) return false
  if (!filterText.value.trim()) return true
  const q = filterText.value.trim().toLowerCase()
  if (page.title.toLowerCase().includes(q)) return true
  if (page.slug.toLowerCase().includes(q)) return true
  if (page.summary && page.summary.toLowerCase().includes(q)) return true
  // Property values
  const values = readPropertyValues(propertyColumns.value, page.page_metadata || {})
  for (const v of Object.values(values)) {
    if (v === null || v === undefined) continue
    if (Array.isArray(v)) {
      if (v.some((x) => String(x).toLowerCase().includes(q))) return true
    } else if (typeof v === 'string' || typeof v === 'number') {
      if (String(v).toLowerCase().includes(q)) return true
    } else if (typeof v === 'boolean') {
      if ((v ? 'true' : 'false').includes(q)) return true
    }
  }
  return false
}

function getCellValue(page: WikiPage, propertyId: string): PropertyValue {
  const values = readPropertyValues(propertyColumns.value, page.page_metadata || {})
  return values[propertyId] ?? null
}

function compare(a: WikiPage, b: WikiPage): number {
  const k = sortKey.value
  let av: unknown
  let bv: unknown
  if (k === 'title') { av = a.title; bv = b.title }
  else if (k === 'page_type') { av = a.page_type; bv = b.page_type }
  else if (k === 'status') { av = a.status; bv = b.status }
  else {
    av = getCellValue(a, k)
    bv = getCellValue(b, k)
  }
  // Null handling is direction-independent (always last) so users see
  // populated rows first regardless of sort direction.
  const aNull = av === null || av === undefined
  const bNull = bv === null || bv === undefined
  if (aNull && bNull) return 0
  if (aNull) return 1
  if (bNull) return -1
  // Both non-null: direction-aware.
  let cmp: number
  if (typeof av === 'number' && typeof bv === 'number') cmp = av - bv
  else if (typeof av === 'boolean' && typeof bv === 'boolean') cmp = Number(av) - Number(bv)
  else cmp = String(av).localeCompare(String(bv))
  return sortDir.value === 'asc' ? cmp : -cmp
}

const filteredPages = computed(() => {
  const filtered = props.pages.filter(matchesFilter)
  const dir = sortDir.value === 'asc' ? 1 : -1
  return [...filtered].sort((a, b) => dir * compare(a, b))
})

function onSort(id: string) {
  if (sortKey.value === id) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = id
    sortDir.value = 'asc'
  }
}

function formatCell(prop: WikiProperty, value: PropertyValue): string {
  return formatPropertyValue(prop, value)
}
</script>

<style scoped>
.wiki-database-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.wiki-database-view__header {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.wiki-database-view__title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.wiki-database-view__title {
  font-size: 16px;
  margin: 0;
  flex: 1 1 auto;
}
.wiki-database-view__count {
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
  font-variant-numeric: tabular-nums;
}
.wiki-database-view__toolbar {
  display: grid;
  grid-template-columns: 1fr 200px;
  gap: 8px;
}
.wiki-database-view__skeleton,
.wiki-database-view__skel-row {
  display: block;
}
.wiki-database-view__skel-row {
  padding: 8px 0;
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
}
.wiki-database-view__table-wrap {
  overflow: auto;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
}
.wiki-database-view__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.wiki-database-view__th {
  position: sticky;
  top: 0;
  background: var(--td-bg-color-secondarycontainer, #f5f5f5);
  text-align: left;
  padding: 8px 10px;
  font-weight: 600;
  cursor: pointer;
  user-select: none;
  white-space: nowrap;
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
}
.wiki-database-view__th:hover {
  background: var(--td-bg-color-container-hover, #ececec);
}
.wiki-database-view__th--active {
  color: var(--td-brand-color, #1677ff);
}
.wiki-database-view__tr {
  cursor: pointer;
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
}
.wiki-database-view__tr:hover {
  background: var(--td-bg-color-container-hover, #f5f5f5);
}
.wiki-database-view__td {
  padding: 6px 10px;
  vertical-align: top;
}
.wiki-database-view__td--title {
  min-width: 200px;
}
.wiki-database-view__title-link {
  font-weight: 500;
  display: block;
}
.wiki-database-view__path {
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
  display: block;
  margin-top: 2px;
}
.wiki-database-view__chip {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer, #f3f3f3);
  color: var(--td-text-color-secondary, #666);
  font-size: 11px;
}
.wiki-database-view__chip--status {
  background: var(--td-brand-color-light, #e6f4ff);
  color: var(--td-brand-color, #1677ff);
}
.wiki-database-view__empty-cell {
  color: var(--td-text-color-placeholder, #999);
}
.wiki-database-view__cell {
  display: inline-block;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
