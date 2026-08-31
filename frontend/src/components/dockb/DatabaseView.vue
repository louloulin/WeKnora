<template>
  <div class="database-view">
    <header class="header">
      <h3>{{ t('dockb.databaseTitle') }}</h3>
      <button class="btn-primary" @click="onCreateDB">
        {{ t('dockb.newDatabase') }}
      </button>
    </header>

    <div v-if="creating" class="modal-backdrop" @click.self="creating = false">
      <div class="modal">
        <h4>{{ t('dockb.createTitle') }}</h4>
        <input v-model="newDB.name" :placeholder="t('dockb.dbNamePlaceholder')" />
        <textarea
          v-model="newDB.description"
          rows="2"
          :placeholder="t('dockb.dbDescPlaceholder')"
        />
        <h5>{{ t('dockb.schemaFields') }}</h5>
        <div v-for="(f, idx) in newDB.schema" :key="idx" class="schema-row">
          <input v-model="f.name" :placeholder="t('dockb.fieldNamePlaceholder')" />
          <select v-model="f.type">
            <option value="text">text</option>
            <option value="number">number</option>
            <option value="checkbox">checkbox</option>
            <option value="date">date</option>
            <option value="select">select</option>
          </select>
          <input
            v-if="f.type === 'select'"
            v-model="f.optionsCsv"
            :placeholder="t('dockb.optionsPlaceholder')"
          />
          <button class="btn-link danger" @click="newDB.schema.splice(idx, 1)">×</button>
        </div>
        <button class="btn-link" @click="addField">+ {{ t('dockb.addField') }}</button>
        <div class="modal-actions">
          <button class="btn-link" @click="creating = false">{{ t('dockb.cancel') }}</button>
          <button class="btn-primary" @click="onSubmitCreate">{{ t('dockb.submit') }}</button>
        </div>
      </div>
    </div>

    <section class="list">
      <div v-if="databases.length === 0" class="empty">{{ t('dockb.noDatabases') }}</div>
      <div v-for="db in databases" :key="db.id" class="db-row">
        <div class="db-head">
          <span class="name">{{ db.name }}</span>
          <span class="muted">{{ db.schema.length }} {{ t('dockb.fields') }}</span>
          <button class="btn-link" @click="onSelectDB(db)">
            {{ t('dockb.open') }}
          </button>
          <button class="btn-link danger" @click="onDeleteDB(db)">
            {{ t('dockb.delete') }}
          </button>
        </div>
      </div>
    </section>

    <section v-if="selected" class="panel">
      <div class="panel-head">
        <h4>{{ selected.name }}</h4>
        <span class="muted">{{ selected.description }}</span>
      </div>
      <div v-if="rows.length === 0" class="empty">{{ t('dockb.noRows') }}</div>
      <table class="grid">
        <thead>
          <tr>
            <th v-for="f in selected.schema" :key="f.name">{{ f.name }}</th>
            <th>{{ t('dockb.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.id">
            <td v-for="f in selected.schema" :key="f.name">
              <span v-if="f.type === 'checkbox'">
                {{ row.values[f.name] ? '☑' : '☐' }}
              </span>
              <span v-else>{{ formatCell(row.values[f.name]) }}</span>
            </td>
            <td>
              <button class="btn-link danger" @click="onDeleteRow(row)">×</button>
            </td>
          </tr>
          <tr class="add-row">
            <td v-for="f in selected.schema" :key="f.name">
              <input
                v-if="f.type === 'text'"
                v-model="newRow[f.name]"
                type="text"
              />
              <input
                v-else-if="f.type === 'number'"
                v-model.number="newRow[f.name]"
                type="number"
              />
              <input
                v-else-if="f.type === 'date'"
                v-model="newRow[f.name]"
                type="date"
              />
              <select
                v-else-if="f.type === 'select'"
                v-model="newRow[f.name]"
              >
                <option v-for="opt in f.options || []" :key="opt" :value="opt">
                  {{ opt }}
                </option>
              </select>
              <input
                v-else-if="f.type === 'checkbox'"
                v-model="newRow[f.name]"
                type="checkbox"
              />
            </td>
            <td>
              <button class="btn-primary" @click="onAddRow">+</button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  createDatabase,
  deleteDatabase,
  deleteRow,
  insertRow,
  listDatabases,
  listRows,
} from '@/api/dockb';
import type {
  DatabaseField,
  WKDatabase,
  WKDatabaseRow,
} from '@/api/dockb/types';

const { t } = useI18n();

const databases = ref<WKDatabase[]>([]);
const selected = ref<WKDatabase | null>(null);
const rows = ref<WKDatabaseRow[]>([]);
const creating = ref(false);

interface DraftField extends DatabaseField {
  optionsCsv?: string;
}

const newDB = reactive<{ name: string; description: string; schema: DraftField[] }>({
  name: '',
  description: '',
  schema: [{ name: 'title', type: 'text' }],
});

const newRow = reactive<Record<string, unknown>>({});

function addField() {
  newDB.schema.push({ name: '', type: 'text' });
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return '';
  return String(v);
}

async function refresh() {
  const r = await listDatabases();
  databases.value = r.databases;
}

function onCreateDB() {
  newDB.name = '';
  newDB.description = '';
  newDB.schema = [{ name: 'title', type: 'text' }];
  creating.value = true;
}

async function onSubmitCreate() {
  if (!newDB.name) return;
  // Convert optionsCsv → options[] for select fields
  const schema: DatabaseField[] = newDB.schema.map((f) => {
    if (f.type === 'select' && f.optionsCsv) {
      return { ...f, options: f.optionsCsv.split(',').map((s) => s.trim()).filter(Boolean) };
    }
    const draft = f as DraftField;
    delete draft.optionsCsv;
    return f;
  });
  await createDatabase({ name: newDB.name, description: newDB.description, schema });
  creating.value = false;
  await refresh();
}

async function onSelectDB(db: WKDatabase) {
  selected.value = db;
  for (const k of Object.keys(newRow)) delete newRow[k];
  const r = await listRows(db.id);
  rows.value = r.rows;
}

async function onDeleteDB(db: WKDatabase) {
  if (!confirm(t('dockb.confirmDeleteDB', { name: db.name }))) return;
  await deleteDatabase(db.id);
  if (selected.value?.id === db.id) {
    selected.value = null;
    rows.value = [];
  }
  await refresh();
}

async function onAddRow() {
  if (!selected.value) return;
  await insertRow(selected.value.id, { values: { ...newRow } });
  const r = await listRows(selected.value.id);
  rows.value = r.rows;
  for (const k of Object.keys(newRow)) delete newRow[k];
}

async function onDeleteRow(row: WKDatabaseRow) {
  if (!selected.value) return;
  await deleteRow(selected.value.id, row.id);
  const r = await listRows(selected.value.id);
  rows.value = r.rows;
}

onMounted(refresh);
</script>

<style scoped>
.database-view {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.btn-primary {
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  border-radius: 4px;
  padding: 6px 12px;
  cursor: pointer;
  font-size: 13px;
}
.btn-link {
  background: transparent;
  border: none;
  color: var(--color-primary, #2563eb);
  cursor: pointer;
  font-size: 12px;
}
.btn-link.danger {
  color: #c0392b;
}
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}
.modal {
  background: var(--color-bg-elevated, #fff);
  padding: 20px 24px;
  border-radius: 8px;
  width: 480px;
  max-width: 90vw;
  max-height: 80vh;
  overflow-y: auto;
}
.modal input, .modal textarea, .modal select {
  width: 100%;
  padding: 6px 8px;
  border: 1px solid var(--color-border, #d0d7de);
  border-radius: 4px;
  font-family: inherit;
  font-size: 13px;
  margin-bottom: 6px;
}
.schema-row {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-bottom: 6px;
}
.schema-row input, .schema-row select {
  margin-bottom: 0;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.db-row {
  border: 1px solid var(--color-border, #e2e6ea);
  border-radius: 6px;
  padding: 8px 12px;
  background: var(--color-bg-elevated, #fafbfc);
}
.db-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.db-head .name {
  font-weight: 600;
  flex: 1;
}
.muted {
  color: var(--color-text-secondary, #5f6b7a);
  font-size: 12px;
}
.empty {
  font-size: 12px;
  color: var(--color-text-secondary, #5f6b7a);
  padding: 8px 0;
}
.panel {
  border: 1px solid var(--color-border, #e2e6ea);
  border-radius: 8px;
  padding: 12px 16px;
  background: var(--color-bg-elevated, #fafbfc);
  overflow-x: auto;
}
.panel-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 8px;
}
.grid {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.grid th, .grid td {
  border: 1px solid var(--color-border, #e2e6ea);
  padding: 4px 8px;
  text-align: left;
}
.grid th {
  background: var(--color-bg-muted, #eef0f3);
  color: var(--color-text-secondary, #5f6b7a);
  font-weight: 600;
}
.grid .add-row td {
  background: var(--color-bg-muted, #f5f6f8);
}
.grid input, .grid select {
  width: 100%;
  padding: 2px 4px;
  border: 1px solid transparent;
  border-radius: 2px;
  font-size: 12px;
}
.grid input:focus, .grid select:focus {
  border-color: var(--color-primary, #2563eb);
  outline: none;
}
</style>
