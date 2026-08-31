<template>
  <div class="connector-manager">
    <header class="header">
      <h3>{{ t('connector.title') }}</h3>
      <button class="btn-primary" @click="creating = true">
        {{ t('connector.newConnector') }}
      </button>
    </header>

    <div v-if="creating" class="modal-backdrop" @click.self="creating = false">
      <div class="modal">
        <h4>{{ t('connector.createTitle') }}</h4>
        <input v-model="newConn.name" :placeholder="t('connector.namePlaceholder')" />
        <select v-model="newConn.kind">
          <option v-for="k in knownKinds" :key="k.kind" :value="k.kind">
            {{ k.label }} — {{ k.description }}
          </option>
        </select>
        <textarea
          v-model="newConn.config"
          rows="5"
          :placeholder="t('connector.configPlaceholder')"
        />
        <input
          v-model="newConn.knowledge_base_id"
          :placeholder="t('connector.kbIdPlaceholder')"
        />
        <div class="modal-actions">
          <button class="btn-link" @click="creating = false">{{ t('connector.cancel') }}</button>
          <button class="btn-primary" @click="onSubmitCreate">{{ t('connector.submit') }}</button>
        </div>
      </div>
    </div>

    <section class="list">
      <div v-if="connectors.length === 0" class="empty">
        {{ t('connector.noConnectors') }}
      </div>
      <div v-for="c in connectors" :key="c.id" class="conn-row">
        <div class="conn-head">
          <span class="name">{{ c.name }}</span>
          <span class="pill">{{ c.kind }}</span>
          <span :class="['pill', c.enabled ? 'pill-active' : 'pill-inactive']">
            {{ c.enabled ? t('connector.enabled') : t('connector.disabled') }}
          </span>
          <span class="muted">
            {{ t('connector.lastSync') }}: {{ formatTime(c.last_sync_at) }}
          </span>
          <button class="btn-link" @click="onTrigger(c)">{{ t('connector.syncNow') }}</button>
          <button class="btn-link danger" @click="onDelete(c)">{{ t('connector.delete') }}</button>
        </div>
        <div v-if="c.last_error" class="err">
          {{ t('connector.lastError') }}: {{ c.last_error }}
        </div>
      </div>
    </section>

    <section v-if="lastJob" class="panel">
      <h4>{{ t('connector.lastJob') }}</h4>
      <div class="job-row">
        <span :class="['pill', 'job-' + lastJob.status]">{{ lastJob.status }}</span>
        <span class="muted">
          {{ t('connector.ingested') }}: {{ lastJob.result_count }}
        </span>
        <span v-if="lastJob.error" class="err">{{ lastJob.error }}</span>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  createConnector,
  deleteConnector,
  listConnectors,
  triggerConnector,
} from '@/api/connector';
import {
  CONNECTOR_KINDS,
  type ConnectorKind,
  type IngestConnector,
  type IngestJob,
} from '@/api/connector/types';

const { t } = useI18n();

const connectors = ref<IngestConnector[]>([]);
const lastJob = ref<IngestJob | null>(null);
const creating = ref(false);
const knownKinds = CONNECTOR_KINDS;

const newConn = reactive<{
  name: string;
  kind: ConnectorKind;
  config: string;
  knowledge_base_id: string;
}>({
  name: '',
  kind: 'slack',
  config: '{\n  "channel": "C01234567",\n  "messages": []\n}',
  knowledge_base_id: '',
});

function formatTime(s?: string): string {
  if (!s) return '—';
  try {
    return new Date(s).toLocaleString();
  } catch {
    return s;
  }
}

async function refresh() {
  const r = await listConnectors();
  connectors.value = r.connectors;
}

async function onSubmitCreate() {
  if (!newConn.name) return;
  await createConnector({ ...newConn });
  creating.value = false;
  newConn.name = '';
  await refresh();
}

async function onTrigger(c: IngestConnector) {
  lastJob.value = await triggerConnector(c.id);
  await refresh();
}

async function onDelete(c: IngestConnector) {
  if (!confirm(t('connector.confirmDelete', { name: c.name }))) return;
  await deleteConnector(c.id);
  await refresh();
}

onMounted(refresh);
</script>

<style scoped>
.connector-manager {
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
.modal input, .modal select, .modal textarea {
  width: 100%;
  padding: 6px 8px;
  border: 1px solid var(--color-border, #d0d7de);
  border-radius: 4px;
  font-family: inherit;
  font-size: 13px;
  margin-bottom: 6px;
}
.modal textarea {
  font-family: monospace;
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
.conn-row {
  border: 1px solid var(--color-border, #e2e6ea);
  border-radius: 6px;
  padding: 8px 12px;
  background: var(--color-bg-elevated, #fafbfc);
}
.conn-head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.conn-head .name {
  font-weight: 600;
  flex: 1;
}
.pill {
  padding: 2px 8px;
  border-radius: 12px;
  background: var(--color-bg-muted, #eef0f3);
  font-size: 11px;
  color: var(--color-text-secondary, #5f6b7a);
}
.pill-active {
  background: #d4f8d4;
  color: #1a6f1a;
}
.pill-inactive {
  background: #f6e2e2;
  color: #8a3a3a;
}
.job-succeeded {
  background: #d4f8d4;
  color: #1a6f1a;
}
.job-failed {
  background: #ffd4d4;
  color: #a30000;
}
.job-running {
  background: #fff5d4;
  color: #8a6800;
}
.job-queued {
  background: #e7f0ff;
  color: #2b5a99;
}
.muted {
  color: var(--color-text-secondary, #5f6b7a);
  font-size: 12px;
}
.err {
  color: #a30000;
  font-size: 11px;
  margin-top: 4px;
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
}
.job-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
