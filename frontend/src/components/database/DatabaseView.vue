<template>
  <div class="database-view">
    <!-- Toolbar: view switcher + add row + settings -->
    <header class="db-toolbar">
      <div class="db-tabs">
        <button
          v-for="view in views"
          :key="view.id"
          :class="['db-tab', { active: view.id === activeViewId }]"
          @click="activeViewId = view.id"
        >
          <span class="db-tab-icon">{{ viewIcon(view.type) }}</span>
          <span class="db-tab-label">{{ view.name || viewLabel(view.type) }}</span>
        </button>
      </div>
      <div class="db-toolbar-actions">
        <button class="btn-primary" @click="onAddRow" v-if="canWrite">+ {{ t('db.row.add') }}</button>
        <div class="db-view-picker" v-if="canWrite">
          <select v-model="newViewType" @change="onAddView">
            <option value="">{{ t('db.view.add') }}</option>
            <option v-for="vt in viewTypes" :key="vt" :value="vt">{{ viewLabel(vt) }}</option>
          </select>
        </div>
      </div>
    </header>

    <!-- View renderer -->
    <main class="db-body">
      <TableView
        v-if="activeViewType === 'table'"
        :database="database"
        :fields="fields"
        :rows="rows"
        :can-write="canWrite"
        @add-row="onAddRow"
        @update-row="onUpdateRow"
        @delete-row="onDeleteRow"
        @add-field="onAddField"
      />
      <BoardView
        v-else-if="activeViewType === 'board'"
        :database="database"
        :fields="fields"
        :rows="rows"
        :can-write="canWrite"
        :group-field="activeViewConfig.board_group_field_id || firstSelectFieldId"
        @add-row="onAddRow"
        @update-row="onUpdateRow"
      />
      <GalleryView
        v-else-if="activeViewType === 'gallery'"
        :database="database"
        :fields="fields"
        :rows="rows"
        :can-write="canWrite"
        @add-row="onAddRow"
      />
      <CalendarView
        v-else-if="activeViewType === 'calendar'"
        :database="database"
        :fields="fields"
        :rows="rows"
        :date-field="activeViewConfig.calendar_date_field_id || firstDateFieldId"
      />
      <TimelineView
        v-else-if="activeViewType === 'timeline'"
        :database="database"
        :fields="fields"
        :rows="rows"
        :start-field="activeViewConfig.timeline_start_field_id || firstDateFieldId"
        :end-field="activeViewConfig.timeline_end_field_id || firstDateFieldId"
      />
      <TableView
        v-else
        :database="database"
        :fields="fields"
        :rows="rows"
        :can-write="canWrite"
        @add-row="onAddRow"
        @update-row="onUpdateRow"
        @delete-row="onDeleteRow"
        @add-field="onAddField"
      />
    </main>

    <!-- Field editor dialog (Add Field) -->
    <div v-if="showFieldDialog" class="modal-backdrop" @click.self="showFieldDialog = false">
      <div class="modal">
        <h4>{{ t('db.field.add') }}</h4>
        <label>{{ t('db.field.name') }}</label>
        <input v-model="newField.name" />
        <label>{{ t('db.field.type') }}</label>
        <select v-model="newField.type">
          <option v-for="ft in fieldTypes" :key="ft" :value="ft">{{ fieldLabel(ft) }}</option>
        </select>
        <div class="modal-actions">
          <button class="btn-link" @click="showFieldDialog = false">{{ t('db.cancel') }}</button>
          <button class="btn-primary" @click="onSubmitField">{{ t('db.submit') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import TableView from './views/TableView.vue'
import BoardView from './views/BoardView.vue'
import GalleryView from './views/GalleryView.vue'
import CalendarView from './views/CalendarView.vue'
import TimelineView from './views/TimelineView.vue'
import { listDatabases, getDatabase, createRow, updateRow, deleteRow, createField, createView } from '@/api/database'
import type { Database, DatabaseField, DatabaseRow, DatabaseView } from '@/api/database'

const props = defineProps<{
  knowledgeBaseId: string
  databaseId?: string
  canWrite?: boolean
}>()

const { t } = useI18n()

// View + field type enumerations (mirror types/database.go).
const viewTypes = ['table', 'board', 'gallery', 'calendar', 'timeline', 'list'] as const
const fieldTypes = ['text', 'number', 'select', 'multi_select', 'date', 'person', 'checkbox', 'url', 'email', 'phone'] as const

// State.
const database = ref<Database | null>(null)
const fields = ref<DatabaseField[]>([])
const views = ref<DatabaseView[]>([])
const rows = ref<DatabaseRow[]>([])
const activeViewId = ref<string>('')
const newViewType = ref<string>('')
const showFieldDialog = ref(false)
const newField = ref<{ name: string; type: string }>({ name: '', type: 'text' })

const activeView = computed(() => views.value.find((v) => v.id === activeViewId.value))
const activeViewType = computed<string>(() => activeView.value?.type ?? 'table')
const activeViewConfig = computed<Record<string, any>>(() => {
  try {
    return activeView.value?.config ? JSON.parse(activeView.value.config as any) : {}
  } catch {
    return {}
  }
})

const firstSelectFieldId = computed<string>(() => {
  return fields.value.find((f) => f.type === 'select')?.id ?? ''
})
const firstDateFieldId = computed<string>(() => {
  return fields.value.find((f) => f.type === 'date')?.id ?? ''
})

function viewIcon(type: string): string {
  const icons: Record<string, string> = {
    table: '▦',
    board: '▥',
    gallery: '▤',
    calendar: '▣',
    timeline: '▢',
    list: '☰',
  }
  return icons[type] ?? '▦'
}

function viewLabel(type: string): string {
  return (t(`db.view.${type}` as any) as string) ?? type
}

function fieldLabel(type: string): string {
  return (t(`db.field.type.${type}` as any) as string) ?? type
}

// --- lifecycle ---

async function loadDatabaseDetail(id: string) {
  const detail = await getDatabase(props.knowledgeBaseId, id)
  database.value = detail.database
  fields.value = detail.fields
  views.value = detail.views
  rows.value = [] // rows loaded separately per view
  const def = views.value.find((v) => v.is_default) ?? views.value[0]
  if (def) {
    activeViewId.value = def.id
  }
  await loadRows()
}

async function loadRows() {
  if (!database.value) return
  const result = await fetch(`/api/v1/databases/${database.value.id}/rows?limit=200`)
    .then((r) => r.json())
  rows.value = result.items ?? []
}

// --- row handlers ---

async function onAddRow() {
  if (!database.value) return
  const data: Record<string, any> = {}
  // Pre-fill the primary Name field so the new row has a visible title.
  const primary = fields.value.find((f) => f.is_primary)
  if (primary) {
    data[primary.id] = ''
  }
  await createRow(database.value.id, JSON.stringify(data))
  await loadRows()
}

async function onUpdateRow(row: DatabaseRow, patch: Record<string, any>) {
  const merged = { ...JSON.parse(row.data as any), ...patch }
  await updateRow(row.database_id, row.id, JSON.stringify(merged))
  await loadRows()
}

async function onDeleteRow(row: DatabaseRow) {
  if (!confirm(t('db.row.confirmDelete'))) return
  await deleteRow(row.database_id, row.id)
  await loadRows()
}

// --- field handlers ---

function onAddField() {
  showFieldDialog.value = true
  newField.value = { name: '', type: 'text' }
}

async function onSubmitField() {
  if (!database.value) return
  if (!newField.value.name.trim()) return
  await createField(database.value.id, {
    name: newField.value.name,
    type: newField.value.type,
    options: {},
    width: 160,
    sort_order: fields.value.length,
    is_primary: false,
  })
  showFieldDialog.value = false
  await loadDatabaseDetail(database.value.id)
}

// --- view handlers ---

async function onAddView() {
  if (!database.value || !newViewType.value) return
  await createView(database.value.id, {
    type: newViewType.value,
    name: viewLabel(newViewType.value),
    config: {},
    sort_order: views.value.length,
    is_default: false,
  })
  newViewType.value = ''
  await loadDatabaseDetail(database.value.id)
}

// --- bootstrap ---

onMounted(async () => {
  if (props.databaseId) {
    await loadDatabaseDetail(props.databaseId)
  } else {
    // No specific database → load first one for the KB.
    const list = await listDatabases(props.knowledgeBaseId)
    if (list.items.length > 0) {
      await loadDatabaseDetail(list.items[0].id)
    }
  }
})
</script>

<style scoped>
.database-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-secondary, #f7f8fa);
  border-radius: 8px;
  overflow: hidden;
}
.db-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  background: var(--bg-primary, #fff);
  border-bottom: 1px solid var(--border-color, #e6e8eb);
}
.db-tabs {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.db-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary, #666);
  cursor: pointer;
  font-size: 13px;
}
.db-tab.active {
  background: var(--primary-light, #e8f3ff);
  color: var(--primary-color, #2b6fd6);
  border-color: var(--primary-color, #2b6fd6);
}
.db-tab-icon { font-size: 14px; }
.db-toolbar-actions { display: flex; gap: 8px; }
.db-view-picker select {
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid var(--border-color, #e6e8eb);
  background: var(--bg-primary, #fff);
  color: var(--text-primary, #222);
}
.btn-primary {
  padding: 6px 12px;
  border-radius: 6px;
  background: var(--primary-color, #2b6fd6);
  color: #fff;
  border: 0;
  cursor: pointer;
  font-size: 13px;
}
.btn-link {
  padding: 6px 12px;
  border-radius: 6px;
  background: transparent;
  border: 0;
  color: var(--text-secondary, #666);
  cursor: pointer;
}
.db-body { flex: 1; overflow: auto; padding: 16px; }
.modal-backdrop {
  position: fixed; inset: 0;
  background: rgba(0,0,0,0.4);
  display: flex; align-items: center; justify-content: center;
  z-index: 1000;
}
.modal {
  background: var(--bg-primary, #fff);
  border-radius: 12px;
  padding: 24px;
  width: 360px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.modal h4 { margin: 0; }
.modal label { font-size: 12px; color: var(--text-secondary, #666); }
.modal input, .modal select {
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid var(--border-color, #e6e8eb);
}
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
</style>
