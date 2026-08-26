<script setup lang="ts">
/**
 * Build #19.x — WikiKBChipRow.
 *
 * KB chip row used by the wiki search bar to scope a query to a subset
 * of tenant KBs. Renders up to `MAX_VISIBLE` chips inline; any surplus
 * collapses behind a `+N more` toggle so the toolbar stays compact in
 * tenants with many KBs. When the row is fully expanded, the toggle
 * flips to `-collapse` so the user can shrink it back.
 *
 * The row is fully presentational — it does not call any API. The
 * parent (WikiBrowser / WikiSearchBarV2) owns KB fetching, selection
 * state, and the actual search re-run.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

export interface KBOption {
  id: string
  name: string
}

const MAX_VISIBLE = 6

const props = defineProps<{
  options: KBOption[]
  selectedIds: string[]
  /** Optional aria-label override for the row container. */
  ariaLabel?: string
}>()

const emit = defineEmits<{
  (e: 'toggle', id: string): void
}>()

const { t } = useI18n()

const expanded = ref(false)

const visibleOptions = (opts: KBOption[]) => {
  if (expanded.value || opts.length <= MAX_VISIBLE) return opts
  return opts.slice(0, MAX_VISIBLE)
}

const hiddenCount = (opts: KBOption[]) => {
  if (expanded.value) return 0
  if (opts.length <= MAX_VISIBLE) return 0
  return opts.length - MAX_VISIBLE
}

function onToggle(id: string) {
  emit('toggle', id)
}

function toggleExpand() {
  expanded.value = !expanded.value
}

const labelText = computed(() => props.ariaLabel ?? t('wiki.searchV2.kbChips.label'))
const moreLabel = computed(() => t('wiki.searchV2.kbChips.more', { count: hiddenCount(props.options) }))
const collapseLabel = computed(() => t('wiki.searchV2.kbChips.collapse'))
</script>

<template>
  <div class="wiki-kb-chip-row" role="group" :aria-label="labelText">
    <button
      v-for="kb in visibleOptions(options)"
      :key="kb.id"
      type="button"
      class="wiki-kb-chip"
      :class="{ 'is-selected': selectedIds.includes(kb.id) }"
      :aria-pressed="selectedIds.includes(kb.id)"
      :aria-label="kb.name"
      @click="onToggle(kb.id)"
    >
      {{ kb.name }}
    </button>
    <button
      v-if="hiddenCount(options) > 0"
      type="button"
      class="wiki-kb-chip wiki-kb-chip-toggle"
      :aria-expanded="false"
      @click="toggleExpand"
    >
      {{ moreLabel }}
    </button>
    <button
      v-else-if="expanded && options.length > MAX_VISIBLE"
      type="button"
      class="wiki-kb-chip wiki-kb-chip-toggle"
      :aria-expanded="true"
      @click="toggleExpand"
    >
      {{ collapseLabel }}
    </button>
  </div>
</template>

<style scoped>
.wiki-kb-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}

.wiki-kb-chip {
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  background: var(--td-bg-color-container, #fff);
  color: var(--td-text-color-secondary, #666);
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.wiki-kb-chip:hover {
  border-color: var(--td-brand-color, #1d6cb1);
  color: var(--td-brand-color, #1d6cb1);
}

.wiki-kb-chip.is-selected {
  background: var(--td-brand-color-light, #e6f4ff);
  border-color: var(--td-brand-color, #1d6cb1);
  color: var(--td-brand-color, #1d6cb1);
  font-weight: 600;
}

.wiki-kb-chip-toggle {
  font-style: italic;
  color: var(--td-text-color-placeholder, #999);
}
</style>