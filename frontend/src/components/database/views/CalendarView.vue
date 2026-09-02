<template>
  <div class="calendar-view">
    <header class="calendar-header">
      <button class="link" @click="shiftMonth(-1)">←</button>
      <h3>{{ monthLabel }}</h3>
      <button class="link" @click="shiftMonth(1)">→</button>
    </header>
    <div class="calendar-grid">
      <div v-for="dow in ['S','M','T','W','T','F','S']" :key="dow + Math.random()" class="calendar-dow">{{ dow }}</div>
      <div v-for="cell in cells" :key="cell.key" :class="['calendar-cell', { muted: !cell.inMonth }]">
        <div class="calendar-cell-date">{{ cell.day }}</div>
        <div v-for="row in cell.rows" :key="row.id" class="calendar-event">
          {{ getPrimaryValue(row) || t('db.card.untitled') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DatabaseField, DatabaseRow } from '@/api/database'

const props = defineProps<{
  database: any
  fields: DatabaseField[]
  rows: DatabaseRow[]
  dateField?: string
}>()

const { t } = useI18n()

const today = new Date()
const viewYear = ref(today.getFullYear())
const viewMonth = ref(today.getMonth()) // 0-indexed

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

function getRowDate(row: DatabaseRow): Date | null {
  if (!props.dateField) return null
  const v = rowData(row)[props.dateField]
  if (!v) return null
  const d = new Date(v)
  return isNaN(d.getTime()) ? null : d
}

const monthLabel = computed(() => {
  const d = new Date(viewYear.value, viewMonth.value, 1)
  return d.toLocaleString('default', { month: 'long', year: 'numeric' })
})

function shiftMonth(delta: number) {
  let m = viewMonth.value + delta
  let y = viewYear.value
  if (m < 0) { m = 11; y-- }
  if (m > 11) { m = 0; y++ }
  viewMonth.value = m
  viewYear.value = y
}

interface Cell { key: string; day: number; inMonth: boolean; rows: DatabaseRow[] }

const cells = computed<Cell[]>(() => {
  const firstOfMonth = new Date(viewYear.value, viewMonth.value, 1)
  const lastOfMonth = new Date(viewYear.value, viewMonth.value + 1, 0)
  const startWeekday = firstOfMonth.getDay()
  const totalCells = Math.ceil((startWeekday + lastOfMonth.getDate()) / 7) * 7
  const out: Cell[] = []
  for (let i = 0; i < totalCells; i++) {
    const dayOffset = i - startWeekday + 1
    const cellDate = new Date(viewYear.value, viewMonth.value, dayOffset)
    const inMonth = cellDate.getMonth() === viewMonth.value
    const cellRows = props.rows.filter((row) => {
      const rd = getRowDate(row)
      if (!rd) return false
      return rd.getFullYear() === cellDate.getFullYear() &&
             rd.getMonth() === cellDate.getMonth() &&
             rd.getDate() === cellDate.getDate()
    })
    out.push({ key: cellDate.toISOString(), day: cellDate.getDate(), inMonth, rows: cellRows })
  }
  return out
})
</script>

<style scoped>
.calendar-view { background: var(--app-surface-bg, #181a1d); border: 1px solid var(--app-border, #30343a); border-radius: 8px; padding: 16px; }
.calendar-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.calendar-header h3 { margin: 0; }
.calendar-grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 4px; }
.calendar-dow { font-size: 11px; color: var(--app-text-muted, #a1a1aa); text-align: center; padding: 4px; }
.calendar-cell { min-height: 80px; padding: 4px; border: 1px solid var(--app-border, #30343a); border-radius: 4px; font-size: 12px; }
.calendar-cell.muted { background: var(--app-surface-raised, #202327); color: var(--app-text-muted, #a1a1aa); }
.calendar-cell-date { font-weight: 600; margin-bottom: 4px; }
.calendar-event { background: color-mix(in srgb, var(--app-brand, #06b04d) 20%, transparent); color: var(--app-brand, #06b04d); padding: 2px 4px; border-radius: 3px; margin-bottom: 2px; font-size: 11px; }
.link { background: transparent; border: 0; color: var(--app-brand, #06b04d); cursor: pointer; padding: 4px 8px; }
</style>
