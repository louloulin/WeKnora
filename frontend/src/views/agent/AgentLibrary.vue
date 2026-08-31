<template>
  <div class="agent-library">
    <header class="agent-library__header">
      <div class="agent-library__title-row">
        <t-icon name="app" size="20px" class="agent-library__title-icon" />
        <div class="agent-library__title-text">
          <h2 class="agent-library__title">{{ $t('agent.library.title') }}</h2>
          <p class="agent-library__subtitle">{{ $t('agent.library.subtitle') }}</p>
        </div>
        <span class="agent-library__count" :title="$t('agent.library.countLabel')">
          {{ filteredAgents.length }} / {{ agents.length }}
        </span>
      </div>

      <div class="agent-library__toolbar">
        <t-input
          v-model="filterText"
          :placeholder="$t('agent.library.filterPlaceholder')"
          clearable
          size="medium"
        >
          <template #prefixIcon><t-icon name="search" /></template>
        </t-input>
        <t-select
          v-model="filterType"
          size="medium"
          :placeholder="$t('agent.library.filterByType')"
          clearable
        >
          <t-option v-for="t in knownAgentTypes" :key="t.value" :value="t.value" :label="t.label" />
        </t-select>
        <t-select
          v-model="filterMode"
          size="medium"
          :placeholder="$t('agent.library.filterByMode')"
          clearable
        >
          <t-option :value="'quick-answer'" :label="$t('agent.library.modeQuickAnswer')" />
          <t-option :value="'smart-reasoning'" :label="$t('agent.library.modeSmartReasoning')" />
        </t-select>
        <t-select
          v-model="filterSource"
          size="medium"
          :placeholder="$t('agent.library.filterBySource')"
          clearable
        >
          <t-option :value="'builtin'" :label="$t('agent.library.sourceBuiltin')" />
          <t-option :value="'custom'" :label="$t('agent.library.sourceCustom')" />
        </t-select>
      </div>
    </header>

    <div v-if="loading && agents.length === 0" class="agent-library__skeleton">
      <div v-for="n in 6" :key="'skel-' + n" class="agent-library__skel-card">
        <t-skeleton :row-col="[{ width: '40%', height: '18px' }, { width: '100%' }, { width: '60%' }]" />
      </div>
    </div>

    <EmptyState
      v-else-if="agents.length === 0"
      icon="app"
      :title="$t('agent.library.emptyTitle')"
      :description="$t('agent.library.emptyDesc')"
    />

    <EmptyState
      v-else-if="filteredAgents.length === 0"
      icon="search"
      :title="$t('agent.library.noMatchTitle')"
      :description="$t('agent.library.noMatchDesc')"
    />

    <div v-else class="agent-library__grid">
      <article
        v-for="agent in filteredAgents"
        :key="agent.id"
        class="agent-library__card"
        :class="{ 'agent-library__card--builtin': agent.is_builtin }"
      >
        <header class="agent-library__card-header">
          <div class="agent-library__card-avatar" :data-mode="getMode(agent)">
            <t-icon :name="getAvatarIcon(agent)" size="20px" />
          </div>
          <div class="agent-library__card-titles">
            <h3 class="agent-library__card-name">
              {{ agent.name }}
              <span v-if="agent.is_builtin" class="agent-library__badge">
                {{ $t('agent.library.badgeBuiltin') }}
              </span>
            </h3>
            <p v-if="agent.description" class="agent-library__card-desc">
              {{ truncate(agent.description, 100) }}
            </p>
          </div>
        </header>

        <ul class="agent-library__meta">
          <li class="agent-library__meta-item">
            <t-icon name="category" size="12px" />
            <span>{{ getTypeLabel(agent) }}</span>
          </li>
          <li class="agent-library__meta-item">
            <t-icon name="tools" size="12px" />
            <span>{{ $t('agent.library.metaTools', { count: getToolCount(agent) }) }}</span>
          </li>
          <li class="agent-library__meta-item" v-if="getKbLabel(agent)">
            <t-icon name="books" size="12px" />
            <span>{{ getKbLabel(agent) }}</span>
          </li>
          <li class="agent-library__meta-item" v-if="agent.creator_name">
            <t-icon name="user" size="12px" />
            <span>{{ agent.creator_name }}</span>
          </li>
        </ul>

        <footer class="agent-library__card-footer">
          <button
            type="button"
            class="agent-library__btn agent-library__btn--primary"
            @click="$emit('start', agent.id)"
          >
            <t-icon name="chat" size="14px" />
            <span>{{ $t('agent.library.startSession') }}</span>
          </button>
          <button
            v-if="!agent.is_builtin"
            type="button"
            class="agent-library__btn"
            @click="$emit('edit', agent.id)"
          >
            <t-icon name="edit" size="14px" />
            <span>{{ $t('agent.library.edit') }}</span>
          </button>
          <button
            v-if="!agent.is_builtin"
            type="button"
            class="agent-library__btn"
            @click="$emit('copy', agent.id)"
          >
            <t-icon name="copy" size="14px" />
            <span>{{ $t('agent.library.copy') }}</span>
          </button>
        </footer>
      </article>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/EmptyState.vue'
import type { CustomAgent } from '@/api/agent'

const props = defineProps<{
  agents: CustomAgent[]
  loading?: boolean
}>()

defineEmits<{
  (e: 'start', agentId: string): void
  (e: 'edit', agentId: string): void
  (e: 'copy', agentId: string): void
}>()

const { t } = useI18n()

const filterText = ref('')
const filterType = ref<string>('')
const filterMode = ref<string>('')
const filterSource = ref<string>('')

const knownAgentTypes = [
  { value: 'rag-qa', label: 'RAG Q&A' },
  { value: 'wiki-qa', label: 'Wiki Q&A' },
  { value: 'hybrid-rag-wiki', label: 'Hybrid RAG+Wiki' },
  { value: 'data-analysis', label: 'Data Analysis' },
  { value: 'custom', label: 'Custom' },
]

function truncate(s: string, n: number): string {
  if (!s) return ''
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}

function getMode(agent: CustomAgent): string {
  return agent.config?.agent_mode || 'quick-answer'
}

function getTypeLabel(agent: CustomAgent): string {
  const type = agent.config?.agent_type || 'custom'
  const found = knownAgentTypes.find((t) => t.value === type)
  return found ? found.label : type
}

function getToolCount(agent: CustomAgent): number {
  return agent.config?.allowed_tools?.length || 0
}

function getKbLabel(agent: CustomAgent): string {
  const mode = agent.config?.kb_selection_mode
  if (!mode || mode === 'none') return ''
  const kbs = agent.config?.knowledge_bases || []
  if (mode === 'all') return t('agent.library.metaKbAll')
  if (kbs.length === 0) return t('agent.library.metaKbNone')
  if (kbs.length === 1) return t('agent.library.metaKbOne')
  return t('agent.library.metaKbMany', { count: kbs.length })
}

function getAvatarIcon(agent: CustomAgent): string {
  const type = agent.config?.agent_type || 'rag-qa'
  if (type === 'wiki-qa') return 'book'
  if (type === 'hybrid-rag-wiki') return 'swap'
  if (type === 'data-analysis') return 'chart'
  if (type === 'custom') return 'edit'
  return 'chat'
}

function matchesFilter(agent: CustomAgent): boolean {
  if (filterSource.value === 'builtin' && !agent.is_builtin) return false
  if (filterSource.value === 'custom' && agent.is_builtin) return false
  if (filterType.value && (agent.config?.agent_type || 'custom') !== filterType.value) return false
  if (filterMode.value && getMode(agent) !== filterMode.value) return false
  if (!filterText.value.trim()) return true
  const q = filterText.value.trim().toLowerCase()
  if (agent.name.toLowerCase().includes(q)) return true
  if (agent.description && agent.description.toLowerCase().includes(q)) return true
  if (agent.creator_name && agent.creator_name.toLowerCase().includes(q)) return true
  return false
}

const filteredAgents = computed(() => props.agents.filter(matchesFilter))
</script>

<style scoped>
.agent-library {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 1280px;
  margin: 0 auto;
}
.agent-library__header {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.agent-library__title-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.agent-library__title-icon {
  color: var(--td-brand-color, #1677ff);
}
.agent-library__title-text {
  flex: 1 1 auto;
}
.agent-library__title {
  margin: 0;
  font-size: 24px;
  font-weight: 700;
}
.agent-library__subtitle {
  margin: 4px 0 0;
  color: var(--td-text-color-secondary, #666);
  font-size: 13px;
}
.agent-library__count {
  background: var(--td-bg-color-secondarycontainer, #f3f3f3);
  color: var(--td-text-color-secondary, #666);
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.agent-library__toolbar {
  display: grid;
  grid-template-columns: 1fr 200px 200px 160px;
  gap: 8px;
}
.agent-library__skeleton {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.agent-library__skel-card {
  padding: 16px;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 12px;
  background: var(--td-bg-color-container, #fff);
}
.agent-library__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.agent-library__card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 12px;
  background: var(--td-bg-color-container, #fff);
  transition: box-shadow 120ms ease, border-color 120ms ease;
}
.agent-library__card:hover {
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.06);
  border-color: var(--td-brand-color, #1677ff);
}
.agent-library__card--builtin {
  background: var(--td-brand-color-light, #f0f8ff);
}
.agent-library__card-header {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.agent-library__card-avatar {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: var(--td-brand-color-light, #e6f4ff);
  color: var(--td-brand-color, #1677ff);
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
}
.agent-library__card-avatar[data-mode='smart-reasoning'] {
  background: var(--td-warning-color-light, #fff7e6);
  color: var(--td-warning-color, #fa8c16);
}
.agent-library__card-titles {
  flex: 1 1 auto;
  min-width: 0;
}
.agent-library__card-name {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 6px;
}
.agent-library__badge {
  background: var(--td-brand-color-light, #e6f4ff);
  color: var(--td-brand-color, #1677ff);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.agent-library__card-desc {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
  line-height: 1.4;
}
.agent-library__meta {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.agent-library__meta-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--td-text-color-secondary, #666);
  background: var(--td-bg-color-secondarycontainer, #f5f5f5);
  padding: 2px 8px;
  border-radius: 999px;
}
.agent-library__card-footer {
  display: flex;
  gap: 6px;
  margin-top: auto;
  padding-top: 8px;
  border-top: 1px solid var(--td-component-stroke, #e7e7e7);
}
.agent-library__btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 6px 10px;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 6px;
  background: var(--td-bg-color-container, #fff);
  color: var(--td-text-color-primary, #1f1f1f);
  cursor: pointer;
  font-size: 12px;
}
.agent-library__btn:hover {
  border-color: var(--td-brand-color, #1677ff);
  color: var(--td-brand-color, #1677ff);
}
.agent-library__btn--primary {
  background: var(--td-brand-color, #1677ff);
  color: white;
  border-color: var(--td-brand-color, #1677ff);
  flex: 1 1 auto;
}
.agent-library__btn--primary:hover {
  background: var(--td-brand-color-hover, #4096ff);
  color: white;
}
</style>
