<template>
  <div class="board-view">
    <div v-for="group in groupedRows" :key="group.key" class="board-column">
      <header class="board-column-header">
        <span class="board-column-title">{{ group.key || t('db.board.ungrouped') }}</span>
        <span class="board-column-count">{{ group.rows.length }}</span>
      </header>
      <div class="board-cards">
        <div v-for="row in group.rows" :key="row.id" class="board-card" @click="onCardClick(row)">
          <div class="board-card-title">{{ getPrimaryValue(row) || t('db.card.untitled') }}</div>
          <div v-for="field in nonPrimaryFields" :key="field.id" class="board-card-field">
            <span class="board-card-field-name">{{ field.name }}:</span>
            <span class="board-card-field-value">{{ getCellValue(row, field) }}</span>
          </div>
        </div>
      </div>
    </div>
    <div v-if="groupedRows.length === 0" class="board-empty">
      {{ t('db.empty') }}
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
  canWrite?: boolean
  groupField?: string
}>()

const emit = defineEmits<{
  'add-row': []
  'update-row': [row: DatabaseRow, patch: Record<string, any>]
}>()

const { t } = useI18n()

const primaryField = computed(() => props.fields.find((f) => f.is_primary))
const nonPrimaryFields = computed(() => props.fields.filter((f) => !f.is_primary && f.id !== props.groupField).slice(0, 3))

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

function getCellValue(row: DatabaseRow, field: DatabaseField): string {
  const v = rowData(row)[field.id]
  if (v == null) return ''
  return String(v)
}

function onCardClick(row: DatabaseRow) {
  // Toggle the group field value to advance (Kanban-style "move to next").
  if (!props.groupField) return
  const choices = (props.fields.find((f) => f.id === props.groupField)?.options as any)?.choices ?? []
  if (!Array.isArray(choices) || choices.length === 0) return
  const current = rowData(row)[props.groupField]
  const idx = choices.findIndex((c: any) => c.name === current)
  const next = choices[(idx + 1) % choices.length].name
  emit('update-row', row, { [props.groupField]: next })
}

const groupedRows = computed(() => {
  if (!props.groupField) return [{ key: '', rows: props.rows }]
  const groups = new Map<string, DatabaseRow[]>()
  for (const row of props.rows) {
    const v = String(rowData(row)[props.groupField] ?? '')
    if (!groups.has(v)) groups.set(v, [])
    groups.get(v)!.push(row)
  }
  return Array.from(groups.entries()).map(([key, rows]) => ({ key, rows }))
})
</script>

<style scoped>
.board-view { display: flex; gap: 12px; overflow-x: auto; padding-bottom: 8px; }
.board-column {
  flex: 0 0 280px;
  background: var(--app-surface-subtle, var(--bg-secondary, #f7f8fa));
  border: 1px solid var(--td-component-stroke, var(--border-color, #e6e8eb));
  border-radius: 8px;
  padding: 12px;
}
.board-column-header {
  display: flex; justify-content: space-between; align-items: center;
  margin-bottom: 12px; font-weight: 600;
}
.board-column-count { color: var(--text-secondary, #999); font-size: 12px; }
.board-cards { display: flex; flex-direction: column; gap: 8px; }
.board-card {
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, var(--border-color, #e6e8eb));
  border-radius: 6px;
  padding: 10px;
  cursor: pointer;
  transition: box-shadow .15s;
}
.board-card:hover { box-shadow: 0 4px 14px rgba(0,0,0,0.22); }
.board-card-title { font-weight: 500; margin-bottom: 6px; }
.board-card-field { display: flex; gap: 4px; font-size: 12px; color: var(--td-text-color-secondary, var(--text-secondary, #666)); }
.board-card-field-name { color: var(--td-text-color-placeholder, var(--text-secondary, #999)); }
.board-empty { color: var(--td-text-color-secondary, var(--text-secondary, #999)); padding: 32px; text-align: center; width: 100%; }
</style>
