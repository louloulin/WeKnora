<template>
  <div class="agent-studio">
    <header class="studio-header">
      <h3>{{ t('customAgent.studioTitle', { name: agentName }) }}</h3>
      <div class="header-actions">
        <button class="btn-primary" @click="triggerManual">
          {{ t('customAgent.runNow') }}
        </button>
      </div>
    </header>

    <nav class="studio-tabs">
      <button
        :class="['tab', { active: tab === 'triggers' }]"
        @click="tab = 'triggers'"
      >
        {{ t('customAgent.tabTriggers') }}
        <span class="badge">{{ triggers.length }}</span>
      </button>
      <button
        :class="['tab', { active: tab === 'runs' }]"
        @click="tab = 'runs'"
      >
        {{ t('customAgent.tabRuns') }}
        <span class="badge">{{ runsTotal }}</span>
      </button>
      <button
        :class="['tab', { active: tab === 'credentials' }]"
        @click="tab = 'credentials'"
      >
        {{ t('customAgent.tabCredentials') }}
        <span class="badge">{{ credentials.length }}</span>
      </button>
    </nav>

    <!-- TRIGGERS PANEL -->
    <section v-if="tab === 'triggers'" class="panel">
      <div class="panel-toolbar">
        <input
          v-model="triggerForm.name"
          :placeholder="t('customAgent.triggerNamePlaceholder')"
        />
        <select v-model="triggerForm.trigger_type">
          <option value="cron">{{ t('customAgent.triggerCron') }}</option>
          <option value="event">{{ t('customAgent.triggerEvent') }}</option>
          <option value="webhook">{{ t('customAgent.triggerWebhook') }}</option>
          <option value="manual">{{ t('customAgent.triggerManual') }}</option>
        </select>
        <input
          v-model="triggerForm.trigger_config"
          :placeholder="t('customAgent.triggerConfigPlaceholder')"
        />
        <button class="btn-primary" @click="addTrigger">
          {{ t('customAgent.addTrigger') }}
        </button>
      </div>
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('customAgent.colName') }}</th>
            <th>{{ t('customAgent.colType') }}</th>
            <th>{{ t('customAgent.colStatus') }}</th>
            <th>{{ t('customAgent.colLastFire') }}</th>
            <th>{{ t('customAgent.colNextFire') }}</th>
            <th>{{ t('customAgent.colActions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="trig in triggers" :key="trig.id">
            <td>{{ trig.name }}</td>
            <td><code>{{ trig.trigger_type }}</code></td>
            <td>
              <span :class="['status-pill', trig.status]">{{ trig.status }}</span>
            </td>
            <td>{{ formatDate(trig.last_fired_at) }}</td>
            <td>{{ formatDate(trig.next_fire_at) }}</td>
            <td>
              <button v-if="trig.status === 'active'" @click="pause(trig)">
                {{ t('customAgent.pause') }}
              </button>
              <button v-else @click="resume(trig)">
                {{ t('customAgent.resume') }}
              </button>
              <button class="btn-danger" @click="remove(trig)">
                {{ t('customAgent.delete') }}
              </button>
            </td>
          </tr>
          <tr v-if="!triggers.length">
            <td colspan="6" class="empty">
              {{ t('customAgent.noTriggers') }}
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- RUNS PANEL -->
    <section v-if="tab === 'runs'" class="panel">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('customAgent.colRunId') }}</th>
            <th>{{ t('customAgent.colTriggeredBy') }}</th>
            <th>{{ t('customAgent.colStatus') }}</th>
            <th>{{ t('customAgent.colSteps') }}</th>
            <th>{{ t('customAgent.colTokens') }}</th>
            <th>{{ t('customAgent.colDuration') }}</th>
            <th>{{ t('customAgent.colStarted') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="run in runs" :key="run.id">
            <td>#{{ run.id }}</td>
            <td>{{ run.triggered_by }}</td>
            <td>
              <span :class="['status-pill', run.status]">{{ run.status }}</span>
            </td>
            <td>{{ run.steps_count }}</td>
            <td>{{ run.tokens_used }}</td>
            <td>{{ run.duration_ms }}ms</td>
            <td>{{ formatDate(run.started_at) }}</td>
          </tr>
          <tr v-if="!runs.length">
            <td colspan="7" class="empty">
              {{ t('customAgent.noRuns') }}
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- CREDENTIALS PANEL -->
    <section v-if="tab === 'credentials'" class="panel">
      <div class="panel-toolbar">
        <input
          v-model="credForm.name"
          :placeholder="t('customAgent.credNamePlaceholder')"
        />
        <select v-model="credForm.credential_type">
          <option value="api_key">api_key</option>
          <option value="oauth2">oauth2</option>
          <option value="basic">basic</option>
          <option value="bearer">bearer</option>
          <option value="custom">custom</option>
        </select>
        <input
          v-model="credForm.secret"
          type="password"
          :placeholder="t('customAgent.credSecretPlaceholder')"
        />
        <button class="btn-primary" @click="addCredential">
          {{ t('customAgent.addCredential') }}
        </button>
      </div>
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('customAgent.colName') }}</th>
            <th>{{ t('customAgent.colType') }}</th>
            <th>{{ t('customAgent.colExpires') }}</th>
            <th>{{ t('customAgent.colLastUsed') }}</th>
            <th>{{ t('customAgent.colActions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="cred in credentials" :key="cred.id">
            <td>{{ cred.name }}</td>
            <td><code>{{ cred.credential_type }}</code></td>
            <td>{{ formatDate(cred.expires_at) }}</td>
            <td>{{ formatDate(cred.last_used_at) }}</td>
            <td>
              <button class="btn-danger" @click="removeCredential(cred)">
                {{ t('customAgent.delete') }}
              </button>
            </td>
          </tr>
          <tr v-if="!credentials.length">
            <td colspan="5" class="empty">
              {{ t('customAgent.noCredentials') }}
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import {
  createCredential,
  createTrigger,
  deleteCredential,
  deleteTrigger,
  listCredentials,
  listRuns,
  listTriggers,
  pauseTrigger,
  resumeTrigger,
  runAgent,
} from '../../api/agentStudio';
import type {
  AgentCredential,
  AgentRun,
  AgentTrigger,
  CreateCredentialRequest,
  CreateTriggerRequest,
} from '../../api/agentStudio/types';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  kbId: string;
  agentId: string;
  agentName: string;
}>();

const { t } = useI18n();
const tab = ref<'triggers' | 'runs' | 'credentials'>('triggers');

const triggers = ref<AgentTrigger[]>([]);
const runs = ref<AgentRun[]>([]);
const runsTotal = ref(0);
const credentials = ref<AgentCredential[]>([]);

const triggerForm = reactive<CreateTriggerRequest>({
  name: '',
  trigger_type: 'cron',
  trigger_config: '{"cron":"0 9 * * *"}',
  payload_template: '',
});

const credForm = reactive<CreateCredentialRequest>({
  name: '',
  credential_type: 'api_key',
  secret: '',
});

function formatDate(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  return isNaN(d.getTime()) ? '—' : d.toLocaleString();
}

async function refreshTriggers() {
  const resp = await listTriggers(props.kbId, props.agentId);
  triggers.value = resp.triggers || [];
}

async function refreshRuns() {
  const resp = await listRuns(props.kbId, props.agentId, 50, 0);
  runs.value = resp.runs || [];
  runsTotal.value = resp.total || 0;
}

async function refreshCredentials() {
  const resp = await listCredentials(props.kbId, props.agentId);
  credentials.value = resp.credentials || [];
}

async function refresh() {
  await Promise.all([refreshTriggers(), refreshRuns(), refreshCredentials()]);
}

async function addTrigger() {
  if (!triggerForm.name) return;
  await createTrigger(props.kbId, props.agentId, triggerForm);
  triggerForm.name = '';
  await refreshTriggers();
}

async function addCredential() {
  if (!credForm.name || !credForm.secret) return;
  await createCredential(props.kbId, props.agentId, credForm);
  credForm.name = '';
  credForm.secret = '';
  await refreshCredentials();
}

async function pause(trig: AgentTrigger) {
  await pauseTrigger(props.kbId, props.agentId, trig.id);
  await refreshTriggers();
}

async function resume(trig: AgentTrigger) {
  await resumeTrigger(props.kbId, props.agentId, trig.id);
  await refreshTriggers();
}

async function remove(trig: AgentTrigger) {
  if (!confirm(`Delete trigger "${trig.name}"?`)) return;
  await deleteTrigger(props.kbId, props.agentId, trig.id);
  await refreshTriggers();
}

async function removeCredential(cred: AgentCredential) {
  if (!confirm(`Delete credential "${cred.name}"?`)) return;
  await deleteCredential(props.kbId, props.agentId, cred.name);
  await refreshCredentials();
}

async function triggerManual() {
  await runAgent(props.kbId, props.agentId, { input: { source: 'manual' } });
  await refreshRuns();
}

onMounted(refresh);
</script>

<style scoped>
.agent-studio {
  border: 1px solid var(--td-line-color, #e7e7e7);
  border-radius: 8px;
  background: var(--td-bg-color-container, #fff);
  padding: 16px;
}
.studio-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.studio-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--td-line-color, #e7e7e7);
  margin-bottom: 12px;
}
.tab {
  background: transparent;
  border: 0;
  padding: 8px 16px;
  cursor: pointer;
  border-bottom: 2px solid transparent;
}
.tab.active {
  border-bottom-color: var(--td-brand-color, #0052d9);
  color: var(--td-brand-color, #0052d9);
  font-weight: 600;
}
.badge {
  display: inline-block;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycomponent, #f3f3f3);
  font-size: 11px;
  margin-left: 6px;
}
.panel-toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
  align-items: center;
}
.panel-toolbar input,
.panel-toolbar select {
  padding: 4px 8px;
  border: 1px solid var(--td-line-color, #e7e7e7);
  border-radius: 4px;
}
.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.data-table th,
.data-table td {
  padding: 6px 8px;
  border-bottom: 1px solid var(--td-line-color, #e7e7e7);
  text-align: left;
}
.empty {
  text-align: center;
  color: var(--td-text-color-secondary, #888);
  padding: 24px;
}
.status-pill {
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
}
.status-pill.active,
.status-pill.succeeded {
  background: rgba(63, 185, 80, 0.15);
  color: #3fb950;
}
.status-pill.paused,
.status-pill.failed,
.status-pill.timeout {
  background: rgba(210, 153, 34, 0.15);
  color: #d29922;
}
.status-pill.archived {
  background: rgba(110, 118, 129, 0.15);
  color: #888;
}
.btn-primary {
  background: var(--td-brand-color, #0052d9);
  color: #fff;
  border: 0;
  padding: 4px 12px;
  border-radius: 4px;
  cursor: pointer;
}
.btn-danger {
  background: transparent;
  color: #d54941;
  border: 0;
  cursor: pointer;
}
</style>
