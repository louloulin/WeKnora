<template>
  <div class="timeline-view">
    <header class="timeline-header">
      <h3>{{ t('db.timeline.title') }}</h3>
      <span class="timeline-count">{{ rows.length }} {{ t('db.timeline.events') }}</span>
    </header>
    <div v-if="rows.length === 0" class="timeline-empty">{{ t('db.empty') }}</div>
    <div v-else class="timeline-rows">
      <div v-for="row in rows" :key="row.id" class="timeline-row">
        <div class="timeline-row-date">
          <div class="timeline-row-date-start">{{ formatDate(getRowStart(row)) }}</div>
          <div v-if="endField !== startField" class="timeline-row-date-end">→ {{ formatDate(getRowEnd(row)) }}</div>
        </div>
        <div class="timeline-row-bar">
          <div class="timeline-row-bar-fill" :style="{ width: barWidth(row) + '%' }"></div>
        </div>
        <div class="timeline-row-title">{{ getPrimaryValue(row) || t('db.card.untitled') }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DatabaseField, DatabaseRow } from '@/api/database'

const props = defineProps<{
  database: any
  fields: DatabaseField[]
  rows: DatabaseRow[]
  startField?: string
  endField?: string
}>()

const { t } = useI18n()

const primaryField = computed(() => props.fields.find((f) => f.is_primary))

function rowData(row: DatabaseRow): Record<string, any> {
  if (typeof row.data === 'string') {
    try { return JSON.parse(row.data) } catch { return {} }
  }
  return row.data ?? {}
}

function getPrimaryValue(row: DatabaseRow): string {
  if (!primaryField.value) return ''
  return String(rowData(row)[primaryField.value.id] ?? '')
}

function getRowStart(row: DatabaseRow): Date | null {
  if (!props.startField) return null
  const v = rowData(row)[props.startField]
  if (!v) return null
  const d = new Date(v)
  return isNaN(d.getTime()) ? null : d
}

function getRowEnd(row: DatabaseRow): Date | null {
  const ef = props.endField ?? props.startField
  if (!ef) return null
  const v = rowData(row)[ef]
  if (!v) return null
  const d = new Date(v)
  return isNaN(d.getTime()) ? null : d
}

function formatDate(d: Date | null): string {
  if (!d) return '—'
  return d.toLocaleDateString('default', { month: 'short', day: 'numeric' })
}

function barWidth(row: DatabaseRow): number {
  const start = getRowStart(row)
  const end = getRowEnd(row)
  if (!start || !end) return 100
  const days = Math.max(1, Math.ceil((end.getTime() - start.getTime()) / 86400000))
  return Math.min(100, days * 4)
}
</script>

<style scoped>
.timeline-view { background: #fff; border: 1px solid var(--border-color, #e6e8eb); border-radius: 8px; padding: 16px; }
.timeline-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.timeline-header h3 { margin: 0; }
.timeline-count { color: var(--text-secondary, #999); font-size: 12px; }
.timeline-rows { display: flex; flex-direction: column; gap: 8px; }
.timeline-row { display: grid; grid-template-columns: 120px 1fr 240px; align-items: center; gap: 12px; padding: 6px 0; border-bottom: 1px solid var(--border-color, #f0f1f3); }
.timeline-row-date-start { font-weight: 600; font-size: 13px; }
.timeline-row-date-end { font-size: 11px; color: var(--text-secondary, #999); }
.timeline-row-bar { background: var(--bg-secondary, #f7f8fa); height: 8px; border-radius: 4px; overflow: hidden; }
.timeline-row-bar-fill { background: var(--primary-color, #2b6fd6); height: 100%; border-radius: 4px; }
.timeline-row-title { font-size: 13px; }
.timeline-empty { color: var(--text-secondary, #999); padding: 32px; text-align: center; }
</style>
