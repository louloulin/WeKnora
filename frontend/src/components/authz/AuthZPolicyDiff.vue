<template>
  <div class="authz-policy-diff">
    <header class="diff-header">
      <h3>{{ t('authz.policyTitle') }}</h3>
      <div class="header-actions">
        <input
          v-model="newKey"
          :placeholder="t('authz.newKeyPlaceholder')"
          class="key-input"
        />
        <button class="btn-primary" :disabled="!newKey" @click="onCreate">
          {{ t('authz.newPolicy') }}
        </button>
      </div>
    </header>

    <!-- Policy key list -->
    <section class="panel">
      <h4>{{ t('authz.policies') }} ({{ keys.length }})</h4>
      <div v-if="keys.length === 0" class="empty">{{ t('authz.noPolicies') }}</div>
      <div v-for="k in keys" :key="k" class="key-row">
        <code class="key-name">{{ k }}</code>
        <button class="btn-link" @click="onSelect(k)">{{ t('authz.viewHistory') }}</button>
      </div>
    </section>

    <!-- Editor + history for selected key -->
    <section v-if="selectedKey" class="panel">
      <div class="editor-grid">
        <div class="editor-pane">
          <h4>{{ t('authz.publishVersion') }} — {{ selectedKey }}</h4>
          <div class="form-row">
            <label>{{ t('authz.decision') }}</label>
            <select v-model="form.decision">
              <option value="allow">allow</option>
              <option value="deny">deny</option>
              <option value="conditional">conditional</option>
            </select>
          </div>
          <div class="form-row column">
            <label>{{ t('authz.expression') }}</label>
            <textarea
              v-model="form.expression"
              rows="4"
              :placeholder="t('authz.expressionPlaceholder')"
            />
          </div>
          <div class="form-row column">
            <label>{{ t('authz.metadata') }}</label>
            <textarea v-model="form.metadata" rows="2" />
          </div>
          <button class="btn-primary" :disabled="!form.expression" @click="onPublish">
            {{ t('authz.publish') }}
          </button>
        </div>

        <div class="history-pane">
          <h4>{{ t('authz.versionHistory') }} ({{ versions.length }})</h4>
          <div v-if="versions.length === 0" class="empty">
            {{ t('authz.noVersions') }}
          </div>
          <div v-for="v in versions" :key="v.id" class="version-row">
            <span class="pill">v{{ v.version }}</span>
            <span :class="['pill', 'pill-decision', `dec-${v.decision}`]">
              {{ v.decision }}
            </span>
            <span class="muted">{{ formatTime(v.created_at) }}</span>
            <button
              v-if="selectedFrom && selectedFrom.id === v.id"
              class="btn-link"
              disabled
            >
              {{ t('authz.from') }}
            </button>
            <button v-else class="btn-link" @click="onSetFrom(v)">
              {{ t('authz.setFrom') }}
            </button>
            <button
              v-if="selectedTo && selectedTo.id === v.id"
              class="btn-link"
              disabled
            >
              {{ t('authz.to') }}
            </button>
            <button v-else class="btn-link" @click="onSetTo(v)">
              {{ t('authz.setTo') }}
            </button>
            <button class="btn-link" @click="onRollback(v)">
              {{ t('authz.rollback') }}
            </button>
          </div>
        </div>
      </div>

      <!-- Diff summary -->
      <div v-if="diff" class="diff-summary">
        <h4>
          {{ t('authz.diffTitle', {
            from: diff.from.version,
            to: diff.to.version,
          }) }}
        </h4>
        <p>{{ diff.summary }}</p>
        <div class="diff-side-by-side">
          <div class="diff-col">
            <div class="diff-col-title">v{{ diff.from.version }} ({{ diff.from.decision }})</div>
            <pre><code>{{ diff.from.expression }}</code></pre>
          </div>
          <div class="diff-col">
            <div class="diff-col-title">v{{ diff.to.version }} ({{ diff.to.decision }})</div>
            <pre><code>{{ diff.to.expression }}</code></pre>
          </div>
        </div>
      </div>
    </section>

    <!-- Simulator -->
    <section class="panel">
      <h4>{{ t('authz.simulator') }}</h4>
      <div class="form-row inline">
        <label>{{ t('authz.policyKey') }}</label>
        <select v-model="sim.policy_key">
          <option value="">—</option>
          <option v-for="k in keys" :key="k" :value="k">{{ k }}</option>
        </select>
      </div>
      <div class="form-row column">
        <label>{{ t('authz.actorJson') }}</label>
        <textarea v-model="sim.actor_json" rows="3" />
      </div>
      <button class="btn-primary" :disabled="!sim.policy_key" @click="onSimulate">
        {{ t('authz.run') }}
      </button>
      <div v-if="simResult" class="sim-result">
        <span :class="['pill', 'pill-decision', `dec-${simResult}`]">{{ simResult }}</span>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  diffAuthZVersions,
  listAuthZKeys,
  listAuthZVersions,
  publishAuthZPolicy,
  rollbackAuthZ,
  simulateAuthZ,
} from '@/api/dlpAuthz';
import type { AuthZPolicyVersion } from '@/api/dlpAuthz/types';

const { t } = useI18n();

const keys = ref<string[]>([]);
const selectedKey = ref<string | null>(null);
const versions = ref<AuthZPolicyVersion[]>([]);
const selectedFrom = ref<AuthZPolicyVersion | null>(null);
const selectedTo = ref<AuthZPolicyVersion | null>(null);
const diff = ref<{
  from: AuthZPolicyVersion;
  to: AuthZPolicyVersion;
  summary: string;
} | null>(null);
const newKey = ref('');
const simResult = ref<string | null>(null);

const form = reactive({
  decision: 'allow' as 'allow' | 'deny' | 'conditional',
  expression: '',
  metadata: '{}',
});

const sim = reactive({
  policy_key: '',
  actor_json: '{"role":"editor"}',
});

async function refreshKeys() {
  const r = await listAuthZKeys();
  keys.value = r.keys;
}

async function onSelect(k: string) {
  selectedKey.value = k;
  selectedFrom.value = null;
  selectedTo.value = null;
  diff.value = null;
  const r = await listAuthZVersions(k);
  versions.value = r.versions.sort((a, b) => b.version - a.version);
  if (versions.value.length >= 2) {
    selectedFrom.value = versions.value[versions.value.length - 1];
    selectedTo.value = versions.value[0];
    await refreshDiff();
  } else if (versions.value.length === 1) {
    selectedTo.value = versions.value[0];
  }
}

async function refreshDiff() {
  if (!selectedFrom.value || !selectedTo.value) {
    diff.value = null;
    return;
  }
  diff.value = await diffAuthZVersions(selectedFrom.value.id, selectedTo.value.id);
}

function onSetFrom(v: AuthZPolicyVersion) {
  selectedFrom.value = v;
  refreshDiff();
}

function onSetTo(v: AuthZPolicyVersion) {
  selectedTo.value = v;
  refreshDiff();
}

async function onCreate() {
  if (!newKey.value) return;
  await publishAuthZPolicy({
    policy_key: newKey.value,
    expression: 'true',
    decision: 'allow',
    metadata: '{}',
  });
  await refreshKeys();
  await onSelect(newKey.value);
  newKey.value = '';
}

async function onPublish() {
  if (!selectedKey.value) return;
  await publishAuthZPolicy({
    policy_key: selectedKey.value,
    expression: form.expression,
    decision: form.decision,
    metadata: form.metadata,
  });
  form.expression = '';
  await onSelect(selectedKey.value);
}

async function onRollback(v: AuthZPolicyVersion) {
  if (!selectedKey.value) return;
  if (!confirm(t('authz.rollbackConfirm', { version: v.version }))) return;
  await rollbackAuthZ(selectedKey.value, { version_id: v.id });
  await onSelect(selectedKey.value);
}

async function onSimulate() {
  let actor: Record<string, unknown> = {};
  try {
    actor = JSON.parse(sim.actor_json);
  } catch {
    simResult.value = 'invalid JSON';
    return;
  }
  const r = await simulateAuthZ({
    policy_key: sim.policy_key,
    actor,
    resource: {},
    action: 'test',
  });
  simResult.value = r.decision;
}

function formatTime(s: string): string {
  if (!s) return '';
  try {
    return new Date(s).toLocaleString();
  } catch {
    return s;
  }
}

onMounted(refreshKeys);
</script>

<style scoped>
.authz-policy-diff {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.diff-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.header-actions {
  display: flex;
  gap: 8px;
}
.key-input {
  width: 200px;
}
.panel {
  border: 1px solid var(--color-border, #e2e6ea);
  border-radius: 8px;
  padding: 12px 16px;
  background: var(--color-bg-elevated, #fafbfc);
}
.form-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.form-row.column {
  flex-direction: column;
  align-items: stretch;
}
.form-row label {
  min-width: 100px;
  color: var(--color-text-secondary, #5f6b7a);
}
input, select, textarea {
  flex: 1;
  padding: 6px 8px;
  border: 1px solid var(--color-border, #d0d7de);
  border-radius: 4px;
  font-family: inherit;
  font-size: 13px;
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
  font-size: 11px;
}
.btn-link:disabled {
  color: #999;
  cursor: default;
}
.key-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 4px 0;
}
.key-name {
  flex: 1;
  font-family: monospace;
  background: var(--color-bg-muted, #eef0f3);
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
}
.editor-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}
.version-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 0;
  border-bottom: 1px solid var(--color-border, #f0f1f3);
}
.muted {
  font-size: 11px;
  color: var(--color-text-secondary, #5f6b7a);
  flex: 1;
}
.pill {
  padding: 2px 8px;
  border-radius: 12px;
  background: var(--color-bg-muted, #eef0f3);
  font-size: 11px;
  color: var(--color-text-secondary, #5f6b7a);
}
.pill-decision.dec-allow {
  background: #d4f8d4;
  color: #1a6f1a;
}
.pill-decision.dec-deny {
  background: #ffd4d4;
  color: #a30000;
}
.pill-decision.dec-conditional {
  background: #fff5d4;
  color: #8a6800;
}
.diff-summary {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px dashed var(--color-border, #e2e6ea);
}
.diff-side-by-side {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.diff-col-title {
  font-size: 12px;
  font-weight: 600;
  margin-bottom: 4px;
}
pre {
  background: var(--color-bg-muted, #f5f6f8);
  padding: 8px;
  border-radius: 4px;
  font-size: 12px;
  overflow-x: auto;
}
.sim-result {
  margin-top: 8px;
}
.empty {
  font-size: 12px;
  color: var(--color-text-secondary, #5f6b7a);
  padding: 8px 0;
}
</style>
