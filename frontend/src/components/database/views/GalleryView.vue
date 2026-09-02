<template>
  <div class="gallery-view">
    <div v-for="row in rows" :key="row.id" class="gallery-card" @click="$emit('add-row')">
      <div class="gallery-card-cover">
        <div class="gallery-card-icon">{{ getIcon(row) }}</div>
      </div>
      <div class="gallery-card-title">{{ getPrimaryValue(row) || t('db.card.untitled') }}</div>
      <div v-for="field in visibleFields" :key="field.id" class="gallery-card-meta">
        <span class="gallery-card-meta-name">{{ field.name }}:</span>
        <span>{{ getCellValue(row, field) }}</span>
      </div>
    </div>
    <div v-if="rows.length === 0" class="gallery-empty">
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
}>()

const emit = defineEmits<{ 'add-row': [] }>()

const { t } = useI18n()

const primaryField = computed(() => props.fields.find((f) => f.is_primary))
const visibleFields = computed(() => props.fields.filter((f) => !f.is_primary).slice(0, 3))

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

function getIcon(row: DatabaseRow): string {
  // Color-coded initial for now; a future iteration renders thumbnails.
  const v = getPrimaryValue(row) || '·'
  return v.slice(0, 1).toUpperCase()
}
</script>

<style scoped>
.gallery-view {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}
.gallery-card {
  background: var(--app-surface-bg, #181a1d);
  border: 1px solid var(--app-border, #30343a);
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  transition: box-shadow .15s;
}
.gallery-card:hover { box-shadow: 0 4px 12px rgba(0,0,0,0.08); }
.gallery-card-cover {
  background: var(--app-surface-raised, #202327);
  height: 96px;
  display: flex; align-items: center; justify-content: center;
}
.gallery-card-icon {
  font-size: 36px; font-weight: 700; color: var(--app-brand, #06b04d);
}
.gallery-card-title { padding: 10px 12px 4px; font-weight: 500; }
.gallery-card-meta {
  padding: 2px 12px;
  font-size: 12px;
  color: var(--app-text-muted, #a1a1aa);
  display: flex; gap: 4px;
}
.gallery-card-meta-name { color: var(--app-text-muted, #a1a1aa); }
.gallery-empty { grid-column: 1 / -1; color: var(--app-text-muted, #a1a1aa); text-align: center; padding: 48px; }
</style>
