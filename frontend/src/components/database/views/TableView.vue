<template>
  <div class="table-view">
    <table class="db-table">
      <thead>
        <tr>
          <th
            v-for="field in fields"
            :key="field.id"
            :style="{ width: field.width + 'px' }"
          >
            {{ field.name }}
            <span class="field-type">{{ field.type }}</span>
          </th>
          <th class="col-actions" v-if="canWrite">{{ t('db.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.id">
          <td v-for="field in fields" :key="field.id">
            <component
              :is="cellComponentFor(field)"
              v-model="cellValue(row, field)"
              :placeholder="t('db.cell.empty')"
              @update:modelValue="onCellUpdate(row, field, $event)"
            />
          </td>
          <td class="col-actions" v-if="canWrite">
            <button class="link" @click="$emit('delete-row', row)">{{ t('db.delete') }}</button>
          </td>
        </tr>
        <tr v-if="rows.length === 0" class="empty-row">
          <td :colspan="fields.length + (canWrite ? 1 : 0)">
            {{ t('db.empty') }}
          </td>
        </tr>
      </tbody>
      <tfoot v-if="canWrite">
        <tr>
          <td :colspan="fields.length" class="add-field">
            <button class="link" @click="$emit('add-field')">+ {{ t('db.field.add') }}</button>
          </td>
          <td v-if="canWrite"></td>
        </tr>
      </tfoot>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, h } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DatabaseField, DatabaseRow } from '@/api/database'

const props = defineProps<{
  database: any
  fields: DatabaseField[]
  rows: DatabaseRow[]
  canWrite?: boolean
}>()

const emit = defineEmits<{
  'add-row': []
  'update-row': [row: DatabaseRow, patch: Record<string, any>]
  'delete-row': [row: DatabaseRow]
  'add-field': []
}>()

const { t } = useI18n()

function rowData(row: DatabaseRow): Record<string, any> {
  if (typeof row.data === 'string') {
    try { return JSON.parse(row.data) } catch { return {} }
  }
  return row.data ?? {}
}

function cellValue(row: DatabaseRow, field: DatabaseField) {
  return rowData(row)[field.id] ?? ''
}

function onCellUpdate(row: DatabaseRow, field: DatabaseField, value: any) {
  emit('update-row', row, { [field.id]: value })
}

// Cell components — render different inputs per field type.
function cellComponentFor(field: DatabaseField) {
  // Use plain inputs wrapped via render function for now.
  return (props: any) => {
    if (field.type === 'checkbox') {
      return h('input', { type: 'checkbox', checked: !!props.modelValue, onChange: (e: any) => props['onUpdate:modelValue'](e.target.checked) })
    }
    if (field.type === 'number') {
      return h('input', { type: 'number', value: props.modelValue ?? '', onInput: (e: any) => props['onUpdate:modelValue'](e.target.value) })
    }
    if (field.type === 'date') {
      return h('input', { type: 'date', value: props.modelValue ?? '', onInput: (e: any) => props['onUpdate:modelValue'](e.target.value) })
    }
    return h('input', { type: 'text', value: props.modelValue ?? '', placeholder: props.placeholder, onInput: (e: any) => props['onUpdate:modelValue'](e.target.value) })
  }
}
</script>

<style scoped>
.table-view { background: var(--app-surface-bg, #181a1d); border-radius: 8px; border: 1px solid var(--app-border, #30343a); overflow: auto; }
.db-table { width: 100%; border-collapse: collapse; }
.db-table th, .db-table td {
  padding: 8px 10px;
  border-bottom: 1px solid var(--app-border, #30343a);
  border-right: 1px solid var(--app-border, #30343a);
  text-align: left;
  font-size: 13px;
}
.db-table th {
  background: var(--app-surface-raised, #202327);
  font-weight: 600;
  color: var(--app-text, #f3f4f6);
  position: sticky; top: 0;
}
.db-table .field-type { color: var(--app-text-muted, #a1a1aa); font-weight: 400; font-size: 11px; margin-left: 6px; }
.db-table input { width: 100%; border: 0; padding: 4px 6px; background: transparent; color: var(--app-text, #f3f4f6); font-size: 13px; }
.db-table input:focus { outline: 2px solid var(--app-brand, #06b04d); border-radius: 4px; }
.col-actions { width: 80px; }
.empty-row td { text-align: center; color: var(--app-text-muted, #a1a1aa); padding: 32px; }
.add-field .link { background: transparent; border: 0; color: var(--app-brand, #06b04d); cursor: pointer; padding: 4px 6px; }
.link { background: transparent; border: 0; color: var(--app-brand, #06b04d); cursor: pointer; padding: 2px 6px; }
</style>
