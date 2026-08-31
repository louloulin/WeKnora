<template>
  <div class="dlp-policy-editor">
    <header class="editor-header">
      <h3>{{ t('dlp.editorTitle') }}</h3>
      <button class="btn-primary" :disabled="saving" @click="onCreate">
        {{ t('dlp.createPolicy') }}
      </button>
    </header>

    <!-- New policy form -->
    <section class="panel">
      <div class="form-row">
        <label>{{ t('dlp.name') }}</label>
        <input v-model="form.name" :placeholder="t('dlp.namePlaceholder')" />
      </div>
      <div class="form-row">
        <label>{{ t('dlp.severity') }}</label>
        <select v-model="form.severity">
          <option value="low">low</option>
          <option value="medium">medium</option>
          <option value="high">high</option>
          <option value="critical">critical</option>
        </select>
      </div>
      <div class="form-row">
        <label>{{ t('dlp.action') }}</label>
        <select v-model="form.action">
          <option value="log">log</option>
          <option value="block">block</option>
          <option value="redact">redact</option>
          <option value="notify_dpo">notify_dpo</option>
        </select>
      </div>
      <div class="form-row">
        <label>{{ t('dlp.resourceScope') }}</label>
        <input v-model="form.resource_scope" placeholder="*" />
      </div>
      <div class="form-row">
        <label>{{ t('dlp.description') }}</label>
        <textarea v-model="form.description" rows="2" />
      </div>
    </section>

    <!-- Existing policies -->
    <section class="panel">
      <h4>{{ t('dlp.existingPolicies') }} ({{ policies.length }})</h4>
      <div v-if="policies.length === 0" class="empty">{{ t('dlp.noPolicies') }}</div>
      <div v-for="p in policies" :key="p.id" class="policy-card">
        <div class="policy-head">
          <span class="name">{{ p.name }}</span>
          <span :class="['pill', p.is_active ? 'pill-active' : 'pill-inactive']">
            v{{ p.version }} · {{ p.is_active ? t('dlp.active') : t('dlp.inactive') }}
          </span>
          <span class="pill pill-sev" :data-sev="p.severity">{{ p.severity }}</span>
          <span class="pill">{{ p.action }}</span>
          <button v-if="!p.is_active" class="btn-link" @click="onActivate(p)">
            {{ t('dlp.activate') }}
          </button>
          <button class="btn-link" @click="onSelect(p)">
            {{ t('dlp.manageRules') }}
          </button>
        </div>
        <div v-if="p.description" class="policy-desc">{{ p.description }}</div>

        <div v-if="selected && selected.id === p.id" class="rule-block">
          <h5>{{ t('dlp.rules') }} ({{ rules.length }})</h5>
          <div v-for="r in rules" :key="r.id" class="rule-row">
            <span class="pill">{{ r.pattern_type }}</span>
            <code class="rule-value">{{ r.pattern_value }}</code>
            <span class="pill pill-sev" :data-sev="r.severity">{{ r.severity }}</span>
            <button class="btn-link danger" @click="onDeleteRule(r)">×</button>
          </div>
          <div class="form-row inline">
            <select v-model="newRule.pattern_type">
              <option value="builtin">builtin</option>
              <option value="regex">regex</option>
              <option value="dictionary">dictionary</option>
            </select>
            <select
              v-if="newRule.pattern_type === 'builtin'"
              v-model="newRule.pattern_value"
            >
              <option
                v-for="b in builtins"
                :key="b.name"
                :value="b.name"
              >
                {{ b.label }}
              </option>
            </select>
            <input
              v-else
              v-model="newRule.pattern_value"
              :placeholder="t('dlp.patternPlaceholder')"
            />
            <select v-model="newRule.severity">
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
              <option value="critical">critical</option>
            </select>
            <button class="btn-primary" @click="onAddRule(p)">+</button>
          </div>
        </div>
      </div>
    </section>

    <!-- Sandbox scanner -->
    <section class="panel">
      <h4>{{ t('dlp.tryScan') }}</h4>
      <textarea v-model="scanText" rows="4" :placeholder="t('dlp.scanPlaceholder')" />
      <button class="btn-primary" :disabled="!scanText" @click="onScan">
        {{ t('dlp.scan') }}
      </button>
      <div v-if="scanResult" class="scan-result">
        <div class="scan-meta">
          {{ t('dlp.scannedChars') }}: {{ scanResult.scanned_chars }} ·
          {{ t('dlp.duration') }}: {{ scanResult.scan_duration_ms }}ms
        </div>
        <div v-if="scanResult.matches.length === 0" class="empty">
          {{ t('dlp.noMatches') }}
        </div>
        <div v-for="(m, idx) in scanResult.matches" :key="idx" class="scan-match">
          <span class="pill pill-sev" :data-sev="m.severity">{{ m.severity }}</span>
          <span class="pill">{{ m.action }}</span>
          <code>{{ m.matched_pattern }}</code>
          <code class="matched-value">{{ redactPreview(m.matched_value) }}</code>
          <div class="context">{{ m.context }}</div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  activateDLPPolicy,
  addDLPRule,
  createDLPPolicy,
  deleteDLPRule,
  listDLPPolicies,
  listDLPRules,
  scanDLP,
} from '@/api/dlpAuthz';
import {
  DLP_BUILTIN_PATTERNS,
  type DLPPolicy,
  type DLPRule,
  type DLPScanResponse,
} from '@/api/dlpAuthz/types';

const { t } = useI18n();

const policies = ref<DLPPolicy[]>([]);
const selected = ref<DLPPolicy | null>(null);
const rules = ref<DLPRule[]>([]);
const saving = ref(false);
const scanText = ref('');
const scanResult = ref<DLPScanResponse | null>(null);
const builtins = DLP_BUILTIN_PATTERNS;

const form = reactive({
  name: '',
  severity: 'medium' as const,
  action: 'log' as const,
  resource_scope: '*',
  description: '',
});

const newRule = reactive({
  pattern_type: 'builtin' as 'regex' | 'dictionary' | 'builtin',
  pattern_value: builtins[0].name,
  severity: 'medium' as 'low' | 'medium' | 'high' | 'critical',
});

async function refresh() {
  const r = await listDLPPolicies();
  policies.value = r.policies;
}

async function onCreate() {
  if (!form.name) return;
  saving.value = true;
  try {
    await createDLPPolicy({ ...form });
    form.name = '';
    form.description = '';
    await refresh();
  } finally {
    saving.value = false;
  }
}

async function onActivate(p: DLPPolicy) {
  await activateDLPPolicy(p.id);
  await refresh();
}

async function onSelect(p: DLPPolicy) {
  if (selected.value && selected.value.id === p.id) {
    selected.value = null;
    rules.value = [];
    return;
  }
  selected.value = p;
  const r = await listDLPRules(p.id);
  rules.value = r.rules;
}

async function onAddRule(p: DLPPolicy) {
  if (!newRule.pattern_value) return;
  await addDLPRule(p.id, { ...newRule });
  const r = await listDLPRules(p.id);
  rules.value = r.rules;
}

async function onDeleteRule(r: DLPRule) {
  await deleteDLPRule(r.id);
  if (selected.value) {
    const list = await listDLPRules(selected.value.id);
    rules.value = list.rules;
  }
}

async function onScan() {
  if (!scanText.value) return;
  scanResult.value = await scanDLP({ text: scanText.value, resource: 'editor' });
}

function redactPreview(v: string): string {
  if (v.length <= 6) return '*'.repeat(v.length);
  return v.slice(0, 2) + '*'.repeat(Math.max(4, v.length - 6)) + v.slice(-4);
}

onMounted(refresh);
</script>

<style scoped>
.dlp-policy-editor {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
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
.form-row label {
  min-width: 100px;
  color: var(--color-text-secondary, #5f6b7a);
}
.form-row.inline {
  margin-top: 8px;
}
input, select, textarea {
  flex: 1;
  padding: 6px 8px;
  border: 1px solid var(--color-border, #d0d7de);
  border-radius: 4px;
  font-family: inherit;
  font-size: 13px;
}
.policy-card {
  border: 1px solid var(--color-border, #e2e6ea);
  border-radius: 6px;
  padding: 8px 12px;
  margin-top: 8px;
  background: var(--color-bg, #fff);
}
.policy-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.policy-head .name {
  font-weight: 600;
  flex: 1;
}
.policy-desc {
  margin-top: 4px;
  font-size: 12px;
  color: var(--color-text-secondary, #5f6b7a);
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
.pill-sev[data-sev='low'] {
  background: #e7f0ff;
  color: #2b5a99;
}
.pill-sev[data-sev='medium'] {
  background: #fff5d4;
  color: #8a6800;
}
.pill-sev[data-sev='high'] {
  background: #ffe2cc;
  color: #a85000;
}
.pill-sev[data-sev='critical'] {
  background: #ffd4d4;
  color: #a30000;
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
.rule-block {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--color-border, #e2e6ea);
}
.rule-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}
.rule-value {
  flex: 1;
  font-family: monospace;
  font-size: 12px;
  color: var(--color-text-secondary, #5f6b7a);
}
.matched-value {
  font-family: monospace;
  font-size: 12px;
  color: #a30000;
}
.context {
  font-size: 11px;
  color: var(--color-text-secondary, #5f6b7a);
  margin-left: 8px;
}
.empty {
  font-size: 12px;
  color: var(--color-text-secondary, #5f6b7a);
  padding: 8px 0;
}
.scan-result {
  margin-top: 8px;
}
.scan-meta {
  font-size: 11px;
  color: var(--color-text-secondary, #5f6b7a);
  margin-bottom: 6px;
}
.scan-match {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 4px 0;
}
</style>
