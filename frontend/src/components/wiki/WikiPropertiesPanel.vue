<template>
  <section
    v-if="visible"
    class="wiki-properties-panel"
    :aria-label="$t('knowledgeEditor.wikiProperties.regionLabel')"
  >
    <header class="wiki-properties-panel__header">
      <t-icon name="app" size="14px" class="wiki-properties-panel__icon" />
      <span class="wiki-properties-panel__title">{{ $t('knowledgeEditor.wikiProperties.title') }}</span>
      <button
        v-if="canEdit"
        type="button"
        class="wiki-properties-panel__btn"
        :title="dirty ? $t('knowledgeEditor.wikiProperties.save') : $t('knowledgeEditor.wikiProperties.saved')"
        :disabled="!dirty || saving"
        @click="onSave"
      >
        <t-icon :name="dirty ? 'save' : 'check'" size="14px" />
      </button>
    </header>

    <ul class="wiki-properties-panel__list">
      <li
        v-for="prop in schema"
        :key="prop.id"
        class="wiki-properties-panel__row"
      >
        <span class="wiki-properties-panel__label" :title="prop.id">
          {{ prop.name }}
        </span>

        <!-- Read-only display (no canEdit) -->
        <span v-if="!canEdit" class="wiki-properties-panel__value wiki-properties-panel__value--readonly">
          {{ formatDisplay(prop, getValue(prop.id)) }}
        </span>

        <!-- Editable: checkbox -->
        <label v-else-if="prop.type === 'checkbox'" class="wiki-properties-panel__checkbox">
          <input
            type="checkbox"
            :checked="Boolean(getValue(prop.id))"
            @change="onChange(prop.id, ($event.target as HTMLInputElement).checked)"
          />
          <span>{{ getValue(prop.id) ? '✓' : '—' }}</span>
        </label>

        <!-- Editable: select -->
        <select
          v-else-if="prop.type === 'select' && prop.options"
          class="wiki-properties-panel__select"
          :value="(getValue(prop.id) as string) || ''"
          @change="onChange(prop.id, ($event.target as HTMLSelectElement).value)"
        >
          <option value="">—</option>
          <option v-for="opt in prop.options" :key="opt" :value="opt">{{ opt }}</option>
        </select>

        <!-- Editable: multi-select via comma-separated input (v1) -->
        <input
          v-else-if="prop.type === 'multi-select'"
          class="wiki-properties-panel__input"
          type="text"
          :placeholder="$t('knowledgeEditor.wikiProperties.multiSelectPlaceholder')"
          :value="formatMultiSelect(getValue(prop.id))"
          @change="onChange(prop.id, parseMultiSelect(($event.target as HTMLInputElement).value, prop.options))"
        />

        <!-- Editable: date -->
        <input
          v-else-if="prop.type === 'date'"
          class="wiki-properties-panel__input"
          type="date"
          :value="(getValue(prop.id) as string) || ''"
          @change="onChange(prop.id, ($event.target as HTMLInputElement).value)"
        />

        <!-- Editable: url -->
        <input
          v-else-if="prop.type === 'url'"
          class="wiki-properties-panel__input"
          type="url"
          :placeholder="'https://'"
          :value="(getValue(prop.id) as string) || ''"
          @change="onChange(prop.id, ($event.target as HTMLInputElement).value)"
        />

        <!-- Editable: number -->
        <input
          v-else-if="prop.type === 'number'"
          class="wiki-properties-panel__input"
          type="number"
          :value="(getValue(prop.id) as number) ?? ''"
          @change="onChange(prop.id, ($event.target as HTMLInputElement).valueAsNumber)"
        />

        <!-- Editable: text (default) -->
        <input
          v-else
          class="wiki-properties-panel__input"
          type="text"
          :value="(getValue(prop.id) as string) || ''"
          @change="onChange(prop.id, ($event.target as HTMLInputElement).value)"
        />
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  DEFAULT_PROPERTY_SCHEMA,
  type PropertyType,
  type PropertyValue,
  type PropertyValues,
  type WikiProperty,
  formatPropertyValue,
  readPropertyValues,
  writePropertyValues,
} from './wikiPropertySchema'

const props = defineProps<{
  pageMetadata: Record<string, any> | undefined | null
  canEdit?: boolean
}>()

const emit = defineEmits<{
  (e: 'save', newMetadata: Record<string, any>): void
}>()

const { t } = useI18n()

const schema = DEFAULT_PROPERTY_SCHEMA
const values = ref<PropertyValues>({})
const saving = ref(false)
const visible = computed(() => schema.length > 0)

// Re-read when the page metadata changes (selection switched).
watch(
  () => props.pageMetadata,
  (meta) => {
    values.value = readPropertyValues(schema, meta || {})
  },
  { immediate: true },
)

function getValue(id: string): PropertyValue {
  return values.value[id] ?? null
}

function onChange(id: string, raw: unknown) {
  const prop = schema.find((p) => p.id === id)
  if (!prop) return
  const coerced = coerceOnClient(prop, raw)
  if (coerced === null) {
    const next = { ...values.value }
    delete next[id]
    values.value = next
  } else {
    values.value = { ...values.value, [id]: coerced }
  }
}

function coerceOnClient(prop: WikiProperty, raw: unknown): PropertyValue {
  // Mirror of coercePropertyValue from the schema module.
  if (raw === null || raw === undefined) return null
  if (raw === '') return null
  switch (prop.type) {
    case 'text':
      return String(raw)
    case 'number': {
      const n = typeof raw === 'number' ? raw : Number(raw)
      return Number.isFinite(n) ? n : null
    }
    case 'date':
      return typeof raw === 'string' && raw.length > 0 ? raw : null
    case 'select':
      if (typeof raw !== 'string') return null
      if (!prop.options || prop.options.length === 0) return raw
      return prop.options.includes(raw) ? raw : null
    case 'multi-select': {
      if (!Array.isArray(raw)) return null
      return raw.filter((v): v is string => typeof v === 'string')
    }
    case 'checkbox':
      return typeof raw === 'boolean' ? raw : null
    case 'url':
      return typeof raw === 'string' && /^https?:\/\//i.test(raw) ? raw : null
    default:
      return null
  }
}

function formatMultiSelect(v: PropertyValue): string {
  return Array.isArray(v) ? v.join(', ') : ''
}

function parseMultiSelect(raw: string, _options?: string[]): string[] {
  return raw
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
}

const dirty = computed(() => {
  const current = readPropertyValues(schema, props.pageMetadata || {})
  return JSON.stringify(current) !== JSON.stringify(values.value)
})

function onSave() {
  if (!dirty.value) return
  saving.value = true
  try {
    const merged = writePropertyValues(props.pageMetadata || {}, values.value)
    emit('save', merged)
  } finally {
    saving.value = false
  }
}

function formatDisplay(prop: WikiProperty, value: PropertyValue): string {
  const s = formatPropertyValue(prop, value)
  return s || t('knowledgeEditor.wikiProperties.empty')
}
</script>

<style scoped>
.wiki-properties-panel {
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  background: var(--td-bg-color-container, #fff);
  margin: 8px 0;
  padding: 8px 10px;
}
.wiki-properties-panel__header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  color: var(--td-text-color-secondary, #666);
  font-size: 12px;
  font-weight: 600;
}
.wiki-properties-panel__icon { flex: 0 0 auto; }
.wiki-properties-panel__title { flex: 1 1 auto; }
.wiki-properties-panel__btn {
  border: 0;
  background: transparent;
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
  color: var(--td-brand-color, #1677ff);
}
.wiki-properties-panel__btn:disabled {
  color: var(--td-text-color-placeholder, #999);
  cursor: not-allowed;
}
.wiki-properties-panel__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.wiki-properties-panel__row {
  display: grid;
  grid-template-columns: 80px 1fr;
  gap: 8px;
  align-items: center;
  font-size: 12px;
}
.wiki-properties-panel__label {
  color: var(--td-text-color-secondary, #666);
  text-align: right;
}
.wiki-properties-panel__value {
  color: var(--td-text-color-primary, #1f1f1f);
}
.wiki-properties-panel__value--readonly {
  color: var(--td-text-color-placeholder, #999);
  font-style: italic;
}
.wiki-properties-panel__input,
.wiki-properties-panel__select {
  width: 100%;
  font-size: 12px;
  padding: 4px 6px;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 4px;
  background: var(--td-bg-color-container, #fff);
  color: var(--td-text-color-primary, #1f1f1f);
}
.wiki-properties-panel__input:focus,
.wiki-properties-panel__select:focus {
  outline: 2px solid var(--td-brand-color, #1677ff);
  outline-offset: 0;
}
.wiki-properties-panel__checkbox {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}
</style>
