<template>
  <div v-if="visible" class="wiki-bulk-tag-bar" role="toolbar" aria-label="bulk tag">
    <div class="wiki-bulk-tag-bar__header">
      <t-icon name="bookmark" />
      <span>{{ t('wiki.tags.bulkBar', { count }) }}</span>
      <button
        type="button"
        class="wiki-bulk-tag-bar__close"
        :disabled="busy"
        :aria-label="t('common.cancel')"
        @click="emit('close')"
      >
        <t-icon name="close" size="14px" />
      </button>
    </div>

    <div class="wiki-bulk-tag-bar__body">
      <t-select
        v-model="tagId"
        :placeholder="t('wiki.tags.bulkSelectTag')"
        filterable
        clearable
        :disabled="busy || store.tags.length === 0"
      >
        <t-option
          v-for="tag in store.tags"
          :key="tag.id"
          :value="tag.id"
          :label="tag.name"
        >
          <t-tag :theme="themeFor(tag.color)" variant="light" size="small">
            {{ tag.name }}
          </t-tag>
        </t-option>
      </t-select>

      <div class="wiki-bulk-tag-bar__actions">
        <t-button
          variant="outline"
          size="small"
          :disabled="busy || !tagId"
          :loading="busy && op === 'add'"
          @click="onApply('add')"
        >
          <t-icon name="add" />
          {{ t('wiki.tags.bulkAdd') }}
        </t-button>
        <t-button
          variant="outline"
          theme="danger"
          size="small"
          :disabled="busy || !tagId"
          :loading="busy && op === 'remove'"
          @click="onApply('remove')"
        >
          <t-icon name="remove" />
          {{ t('wiki.tags.bulkRemove') }}
        </t-button>
      </div>

      <div v-if="store.tags.length === 0" class="wiki-bulk-tag-bar__hint">
        {{ t('wiki.tags.bulkNoTags') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Button as TButton,
  Icon as TIcon,
  Select as TSelect,
  Option as TOption,
  Tag as TTag,
  MessagePlugin,
} from 'tdesign-vue-next'
import { useWikiTagsStore } from '../../stores/wikiTags'
import type { WikiTagColor } from '../../api/wiki/tags'

const props = defineProps<{
  kbId: string
  selectedSlugs: string[]
  busy?: boolean
}>()

const { t } = useI18n()
const store = useWikiTagsStore()

const tagId = ref<string>('')
const op = ref<'add' | 'remove' | ''>('')

const visible = computed(() => props.selectedSlugs.length > 0)
const count = computed(() => props.selectedSlugs.length)

const emit = defineEmits<{
  (e: 'close'): void
  (
    e: 'apply',
    payload: { tagId: string; op: 'add' | 'remove'; slugs: string[] },
  ): void
}>()

function themeFor(color: WikiTagColor): 'primary' | 'success' | 'warning' | 'danger' | 'default' {
  switch (color) {
    case 'red':
      return 'danger'
    case 'orange':
    case 'gold':
      return 'warning'
    case 'green':
    case 'teal':
      return 'success'
    case 'blue':
    case 'purple':
      return 'primary'
    default:
      return 'default'
  }
}

function onApply(next: 'add' | 'remove'): void {
  if (!tagId.value) {
    MessagePlugin.warning(t('wiki.tags.error.selectFirst'))
    return
  }
  op.value = next
  emit('apply', { tagId: tagId.value, op: next, slugs: [...props.selectedSlugs] })
}
</script>

<style scoped>
.wiki-bulk-tag-bar {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  margin-bottom: 8px;
  border: 1px solid var(--td-component-stroke, #dcdcdc);
  border-radius: 6px;
  background: var(--td-brand-color-light, #f0f6ff);
}
.wiki-bulk-tag-bar__header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}
.wiki-bulk-tag-bar__close {
  margin-left: auto;
  background: transparent;
  border: none;
  cursor: pointer;
  color: var(--td-text-color-secondary);
}
.wiki-bulk-tag-bar__body {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.wiki-bulk-tag-bar__actions {
  display: inline-flex;
  gap: 6px;
}
.wiki-bulk-tag-bar__hint {
  font-size: 12px;
  color: var(--td-text-color-placeholder, #999);
}
</style>