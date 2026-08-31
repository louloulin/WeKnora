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
        <t-radio-group v-model="viewMode" size="small" variant="default-filled">
          <t-radio-button value="table">
            <t-icon name="table" size="14px" />
            <span>{{ $t('knowledgeEditor.wikiDatabaseView.viewTable') }}</span>
          </t-radio-button>
          <t-radio-button value="board">
            <t-icon name="kanban" size="14px" />
            <span>{{ $t('knowledgeEditor.wikiDatabaseView.viewBoard') }}</span>
          </t-radio-button>
          <t-radio-button value="calendar">
            <t-icon name="calendar" size="14px" />
            <span>{{ $t('knowledgeEditor.wikiDatabaseView.viewCalendar') }}</span>
          </t-radio-button>
        </t-radio-group>
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

    <div v-else-if="viewMode === 'table'" class="wiki-database-view__table-wrap">
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
    <div v-else-if="viewMode === 'calendar'" class="wiki-database-view__calendar-wrap">
      <div class="wiki-database-view__calendar-toolbar">
        <span class="wiki-database-view__calendar-label">{{ $t('knowledgeEditor.wikiDatabaseView.calendarDateLabel') }}</span>
        <t-select
          v-model="calendarDateProp"
          size="small"
          :placeholder="$t('knowledgeEditor.wikiDatabaseView.calendarDatePlaceholder')"
          clearable
        >
          <t-option
            v-for="prop in calendarDateProps"
            :key="prop.id"
            :value="prop.id"
            :label="prop.name"
          />
        </t-select>
        <div class="wiki-database-view__calendar-nav">
          <t-button size="small" variant="text" @click="shiftCalendarMonth(-1)">
            <t-icon name="chevron-left" />
          </t-button>
          <span class="wiki-database-view__calendar-month">{{ calendarMonthLabel }}</span>
          <t-button size="small" variant="text" @click="shiftCalendarMonth(1)">
            <t-icon name="chevron-right" />
          </t-button>
        </div>
      </div>
      <div v-if="!calendarDateProp" class="wiki-database-view__calendar-empty">
        {{ $t('knowledgeEditor.wikiDatabaseView.calendarEmpty') }}
      </div>
      <div v-else class="wiki-database-view__calendar-grid">
        <div v-for="cell in calendarCells" :key="cell.key" class="wiki-database-view__calendar-cell" :class="{ 'wiki-database-view__calendar-cell--outside': cell.outside }">
          <div class="wiki-database-view__calendar-cell-header">
            <span class="wiki-database-view__calendar-cell-date">{{ cell.day }}</span>
            <span v-if="cell.pages.length > 0" class="wiki-database-view__calendar-cell-count">{{ cell.pages.length }}</span>
          </div>
          <div class="wiki-database-view__calendar-cell-body">
            <article
              v-for="page in cell.pages.slice(0, 3)"
              :key="page.id"
              class="wiki-database-view__calendar-card"
              @click="$emit('select', page.slug)"
            >
              <h5 class="wiki-database-view__calendar-card-title">{{ page.title }}</h5>
            </article>
            <div v-if="cell.pages.length > 3" class="wiki-database-view__calendar-cell-more">
              +{{ cell.pages.length - 3 }} {{ $t('knowledgeEditor.wikiDatabaseView.calendarMore') }}
            </div>
          </div>
        </div>
      </div>
    </div>
    <div v-else-if="viewMode === 'board'" class="wiki-database-view__board-wrap">
      <div class="wiki-database-view__board-toolbar">
        <span class="wiki-database-view__board-label">{{ $t('knowledgeEditor.wikiDatabaseView.boardGroupBy') }}</span>
        <t-select
          v-model="boardGroupBy"
          size="small"
          :placeholder="$t('knowledgeEditor.wikiDatabaseView.boardGroupByPlaceholder')"
          clearable={false}
        >
          <t-option
            v-for="prop in boardGroupableProperties"
            :key="prop.id"
            :value="prop.id"
            :label="prop.name"
          />
        </t-select>
      </div>
      <div v-if="boardColumns.length === 0" class="wiki-database-view__board-empty">
        {{ $t('knowledgeEditor.wikiDatabaseView.boardEmpty') }}
      </div>
      <div v-else class="wiki-database-view__board-scroll">
        <div
          v-for="col in boardColumns"
          :key="col.key"
          class="wiki-database-view__board-col"
        >
          <header class="wiki-database-view__board-col-header">
            <span class="wiki-database-view__board-col-title">{{ col.label }}</span>
            <span class="wiki-database-view__board-col-count">{{ col.pages.length }}</span>
          </header>
          <div class="wiki-database-view__board-col-body">
            <article
              v-for="page in col.pages"
              :key="page.id"
              class="wiki-database-view__board-card"
              @click="$emit('select', page.slug)"
            >
              <h4 class="wiki-database-view__board-card-title">{{ page.title }}</h4>
              <p v-if="page.summary" class="wiki-database-view__board-card-summary">{{ page.summary }}</p>
              <footer class="wiki-database-view__board-card-meta">
                <span v-if="page.page_type" class="wiki-database-view__chip">{{ page.page_type }}</span>
                <span class="wiki-database-view__board-card-time">{{ formatRelativeTime(page.updated_at) }}</span>
              </footer>
            </article>
            <div v-if="col.pages.length === 0" class="wiki-database-view__board-col-empty">
              {{ $t('knowledgeEditor.wikiDatabaseView.boardColEmpty') }}
            </div>
          </div>
        </div>
      </div>
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

// View mode: 'table' (rows × cols) or 'board' (grouped by select property)
const viewMode = ref<'table' | 'board' | 'calendar'>('table')
// Selectable properties for board grouping: must be 'select' or 'multi-select'
const boardGroupBy = ref<string>('status')

const knownPageTypes = computed(() => {
  const set = new Set<string>()
  for (const p of props.pages) {
    if (p.page_type) set.add(p.page_type)
  }
  return Array.from(set).sort()
})

const propertyColumns = computed<WikiProperty[]>(() => DEFAULT_PROPERTY_SCHEMA)

/** Selectable properties that can act as board grouping axis. */
const boardGroupableProperties = computed<WikiProperty[]>(() =>
  propertyColumns.value.filter(p => p.type === 'select' || p.type === 'multi-select')
)

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

/**
 * Group pages by the selected board property (single select for now; multi-select
 * pages appear in every group whose value they include, so a page with all 4
 * status values shows up in all 4 columns). Order respects the property's
 * declared options so users see Draft → Review → Published → Archived.
 */
interface BoardColumn {
  key: string
  label: string
  pages: WikiPage[]
}

const boardColumns = computed<BoardColumn[]>(() => {
  const propId = boardGroupBy.value
  if (!propId) return []
  const prop = boardGroupableProperties.value.find(p => p.id === propId)
  const options = prop?.options ?? []
  const filtered = props.pages.filter(matchesFilter)
  // Build a bucket per option plus an ungrouped bucket for empty values.
  const buckets: Record<string, WikiPage[]> = {}
  for (const opt of options) buckets[opt] = []
  buckets['__ungrouped__'] = []
  for (const page of filtered) {
    const raw = readPropertyValues(propertyColumns.value, page.page_metadata || {})[propId]
    const keys: string[] = []
    if (Array.isArray(raw)) keys.push(...raw.filter((x): x is string => typeof x === 'string'))
    else if (typeof raw === 'string' && raw) keys.push(raw)
    if (keys.length === 0) buckets['__ungrouped__'].push(page)
    else {
      const seen = new Set<string>()
      for (const k of keys) {
        if (!(k in buckets) || seen.has(k)) continue
        seen.add(k)
        buckets[k].push(page)
      }
    }
  }
  const columns: BoardColumn[] = options.map(opt => ({
    key: opt, label: opt, pages: buckets[opt] ?? [],
  }))
  if (buckets['__ungrouped__'].length > 0) {
    columns.push({
      key: '__ungrouped__',
      label: t('knowledgeEditor.wikiDatabaseView.boardUngrouped', 'Ungrouped'),
      pages: buckets['__ungrouped__'],
    })
  }
  return columns
})

interface CalendarCell {
  key: string
  day: number
  outside: boolean
  date: Date
  pages: WikiPage[]
}

const calendarDateProp = ref<string>('')
const calendarCursor = ref<Date>(new Date())

const calendarDateProps = computed(() =>
  propertyColumns.value.filter(p => p.type === 'date')
)

const calendarMonthLabel = computed(() => {
  const d = calendarCursor.value
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
})

function shiftCalendarMonth(delta: number): void {
  const next = new Date(calendarCursor.value)
  next.setMonth(next.getMonth() + delta)
  calendarCursor.value = next
}

const calendarCells = computed<CalendarCell[]>(() => {
  const propId = calendarDateProp.value
  const cursor = calendarCursor.value
  const year = cursor.getFullYear()
  const month = cursor.getMonth()
  // First day of the month, then back up to Monday (ISO).
  const first = new Date(year, month, 1)
  const offset = (first.getDay() + 6) % 7  // Monday = 0
  const gridStart = new Date(year, month, 1 - offset)
  const cells: CalendarCell[] = []
  const filtered = props.pages.filter(matchesFilter)
  // Bucket pages by their date string (yyyy-mm-dd). Only include pages
  // where the date is set; pages without a date are excluded.
  const buckets: Record<string, WikiPage[]> = {}
  if (propId) {
    for (const p of filtered) {
      const raw = readPropertyValues(propertyColumns.value, p.page_metadata || {})[propId]
      if (typeof raw !== 'string' || !raw) continue
      const key = raw.slice(0, 10)  // yyyy-mm-dd
      if (!buckets[key]) buckets[key] = []
      buckets[key].push(p)
    }
  }
  for (let i = 0; i < 42; i++) {
    const d = new Date(gridStart)
    d.setDate(gridStart.getDate() + i)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    cells.push({
      key,
      day: d.getDate(),
      outside: d.getMonth() !== month,
      date: d,
      pages: buckets[key] || [],
    })
  }
  return cells
})

function formatRelativeTime(value: string | number | Date | null | undefined): string {
  if (!value) return ''
  const ts = new Date(value).getTime()
  if (Number.isNaN(ts)) return ''
  const diff = Date.now() - ts
  if (diff < 60_000) return t('common.justNow')
  if (diff < 3_600_000) return t('common.minutesAgo', { n: Math.floor(diff / 60_000) })
  if (diff < 86_400_000) return t('common.hoursAgo', { n: Math.floor(diff / 3_600_000) })
  if (diff < 30 * 86_400_000) return t('common.daysAgo', { n: Math.floor(diff / 86_400_000) })
  return new Date(value).toISOString().slice(0, 10)
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
.wiki-database-view__board-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 2px 10px;
}
.wiki-database-view__board-label {
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
}
.wiki-database-view__board-empty {
  padding: 24px;
  text-align: center;
  color: var(--td-text-color-placeholder, #999);
  font-size: 13px;
}
.wiki-database-view__board-scroll {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 8px;
  scroll-snap-type: x proximity;
}
.wiki-database-view__board-col {
  flex: 0 0 260px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--td-bg-color-container, #fafbfc);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 10px;
  padding: 10px;
  scroll-snap-align: start;
  max-height: 65vh;
  overflow: hidden;
}
.wiki-database-view__board-col-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 6px;
  border-bottom: 1px dashed var(--td-component-stroke, #e7e7e7);
}
.wiki-database-view__board-col-title {
  font-weight: 700;
  font-size: 13px;
  color: var(--td-text-color-primary, #222);
}
.wiki-database-view__board-col-count {
  font-size: 11px;
  color: var(--td-text-color-secondary, #666);
  background: var(--td-bg-color-secondarycontainer, #f3f3f3);
  padding: 0 6px;
  border-radius: 999px;
}
.wiki-database-view__board-col-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
  padding-right: 2px;
}
.wiki-database-view__board-col-empty {
  padding: 16px 4px;
  font-size: 12px;
  text-align: center;
  color: var(--td-text-color-placeholder, #999);
}
.wiki-database-view__board-card {
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  cursor: pointer;
  transition: box-shadow .15s ease, transform .15s ease;
}
.wiki-database-view__board-card:hover {
  box-shadow: var(--td-shadow-1, 0 2px 8px rgba(0,0,0,0.08));
  transform: translateY(-1px);
}
.wiki-database-view__board-card-title {
  font-size: 13px;
  font-weight: 600;
  margin: 0;
  color: var(--td-text-color-primary, #222);
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.wiki-database-view__board-card-summary {
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
  margin: 0;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.wiki-database-view__board-card-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--td-text-color-secondary, #666);
}
.wiki-database-view__board-card-time {
  white-space: nowrap;
}
.wiki-database-view__calendar-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 4px 2px 10px;
}
.wiki-database-view__calendar-label {
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
}
.wiki-database-view__calendar-nav {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}
.wiki-database-view__calendar-month {
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  min-width: 80px;
  text-align: center;
}
.wiki-database-view__calendar-empty {
  padding: 24px;
  text-align: center;
  color: var(--td-text-color-placeholder, #999);
  font-size: 13px;
}
.wiki-database-view__calendar-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 4px;
  border-top: 1px solid var(--td-component-stroke, #e7e7e7);
  border-left: 1px solid var(--td-component-stroke, #e7e7e7);
}
.wiki-database-view__calendar-cell {
  background: var(--td-bg-color-container, #fff);
  border-right: 1px solid var(--td-component-stroke, #e7e7e7);
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
  min-height: 90px;
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.wiki-database-view__calendar-cell--outside {
  background: var(--td-bg-color-secondarycontainer, #fafbfc);
  color: var(--td-text-color-placeholder, #999);
}
.wiki-database-view__calendar-cell-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11px;
}
.wiki-database-view__calendar-cell-date {
  font-weight: 700;
}
.wiki-database-view__calendar-cell-count {
  background: var(--td-brand-color-light, #e6f4ff);
  color: var(--td-brand-color, #1677ff);
  padding: 0 5px;
  border-radius: 999px;
  font-size: 10px;
}
.wiki-database-view__calendar-cell-body {
  display: flex;
  flex-direction: column;
  gap: 3px;
  flex: 1;
  overflow: hidden;
}
.wiki-database-view__calendar-card {
  background: var(--td-bg-color-secondarycontainer, #f3f3f3);
  padding: 4px 6px;
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
  border-left: 3px solid var(--td-brand-color, #1677ff);
}
.wiki-database-view__calendar-card:hover {
  background: var(--td-brand-color-light, #e6f4ff);
}
.wiki-database-view__calendar-card-title {
  margin: 0;
  font-size: 11px;
  font-weight: 600;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.wiki-database-view__calendar-cell-more {
  font-size: 10px;
  color: var(--td-text-color-secondary, #666);
  padding: 2px 6px;
}
</style>
