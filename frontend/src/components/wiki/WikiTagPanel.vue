<template>
  <section class="wiki-tag-panel">
    <button
      type="button"
      class="wiki-tag-panel__header"
      :aria-expanded="expanded"
      :aria-controls="bodyId"
      @click="toggle"
    >
      <span class="wiki-tag-panel__title">
        {{ t('wiki.tags.title') }}
        <span v-if="tagCount > 0" class="wiki-tag-panel__count">
          {{ tagCount }}
        </span>
      </span>
      <t-icon
        :name="expanded ? 'chevron-up' : 'chevron-down'"
        size="16px"
        class="wiki-tag-panel__chevron"
      />
    </button>

    <div v-if="expanded" :id="bodyId" class="wiki-tag-panel__body">
      <div class="wiki-tag-panel__toolbar">
        <button
          type="button"
          class="wiki-tag-panel__create"
          data-testid="wiki-tag-create"
          @click="openCreateDialog"
        >
          <t-icon name="add" size="14px" />
          <span>{{ t('wiki.tags.create') }}</span>
        </button>
        <button
          v-if="tagCount > 0"
          type="button"
          class="wiki-tag-panel__refresh"
          :disabled="store.loading"
          @click="refresh"
        >
          <t-icon name="refresh" size="14px" />
        </button>
      </div>

      <div v-if="store.loading && tagCount === 0" class="wiki-tag-panel__loading">
        <t-skeleton :row="2" />
      </div>

      <div v-else-if="tagCount === 0" class="wiki-tag-panel__empty">
        <p>{{ t('wiki.tags.empty') }}</p>
        <p class="wiki-tag-panel__empty-hint">
          {{ t('wiki.tags.emptyHint') }}
        </p>
      </div>

      <ul v-else class="wiki-tag-panel__list">
        <li
          v-for="tag in store.tags"
          :key="tag.id"
          class="wiki-tag-panel__item"
          :data-tag-id="tag.id"
        >
          <t-tag :theme="themeFor(tag.color)" variant="light" class="wiki-tag-panel__chip">
            {{ tag.name }}
            <span class="wiki-tag-panel__chip-count">{{ tag.page_count }}</span>
          </t-tag>

          <div class="wiki-tag-panel__item-actions">
            <button
              type="button"
              class="wiki-tag-panel__icon-btn"
              :aria-label="t('wiki.tags.edit')"
              :title="t('wiki.tags.edit')"
              @click="openEditDialog(tag)"
            >
              <t-icon name="edit" size="14px" />
            </button>
            <t-popconfirm
              :content="t('wiki.tags.deleteConfirm', { name: tag.name })"
              @confirm="confirmDelete(tag.id)"
            >
              <button
                type="button"
                class="wiki-tag-panel__icon-btn wiki-tag-panel__icon-btn--danger"
                :aria-label="t('wiki.tags.delete')"
                :title="t('wiki.tags.delete')"
              >
                <t-icon name="delete" size="14px" />
              </button>
            </t-popconfirm>
          </div>
        </li>
      </ul>
    </div>

    <WikiTagDialog
      v-if="dialogVisible"
      :kb-id="kbId"
      :tag="dialogTag ?? undefined"
      @saved="onSaved"
      @cancel="closeDialog"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Icon as TIcon,
  Skeleton as TSkeleton,
  Tag as TTag,
  Popconfirm as TPopconfirm,
  MessagePlugin,
} from 'tdesign-vue-next'
import { useWikiTagsStore } from '../../stores/wikiTags'
import type { WikiTag, WikiTagColor } from '../../api/wiki/tags'
import WikiTagDialog from './WikiTagDialog.vue'

const props = defineProps<{
  kbId: string
  defaultExpanded?: boolean
}>()

const { t } = useI18n()
const store = useWikiTagsStore()

const expanded = ref(props.defaultExpanded ?? true)
const tagCount = computed(() => store.tags.length)

const bodyId = computed(
  () => `wiki-tag-panel-body-${props.kbId.replace(/[^a-z0-9]/gi, '-')}`,
)

const dialogVisible = ref(false)
const dialogTag = ref<WikiTag | null>(null)

onMounted(async () => {
  if (!props.kbId) return
  await store.fetchTags(props.kbId)
})

function toggle(): void {
  expanded.value = !expanded.value
}

function refresh(): void {
  store.fetchTags(props.kbId)
}

function openCreateDialog(): void {
  dialogTag.value = null
  dialogVisible.value = true
}

function openEditDialog(tag: WikiTag): void {
  dialogTag.value = tag
  dialogVisible.value = true
}

function closeDialog(): void {
  dialogVisible.value = false
  dialogTag.value = null
}

function onSaved(): void {
  // The store already refreshed; nothing else to do besides closing
  // the dialog (the dialog emits `saved` before `cancel`).
  closeDialog()
}

async function confirmDelete(tagId: string): Promise<void> {
  const ok = await store.deleteTag(props.kbId, tagId)
  if (ok) {
    MessagePlugin.success(t('wiki.tags.deleteSuccess'))
  } else {
    MessagePlugin.error(t('wiki.tags.error.deleteFailed'))
  }
}

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
</script>

<style scoped>
.wiki-tag-panel {
  border-top: 1px solid var(--td-component-stroke, #e7e7e7);
  padding: 8px 0;
}
.wiki-tag-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  background: transparent;
  border: none;
  cursor: pointer;
  padding: 4px 0;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}
.wiki-tag-panel__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
}
.wiki-tag-panel__count {
  font-size: 11px;
  background: var(--td-component-color, #f3f3f3);
  color: var(--td-text-color-secondary);
  border-radius: 8px;
  padding: 0 6px;
  height: 16px;
  line-height: 16px;
}
.wiki-tag-panel__chevron {
  color: var(--td-text-color-placeholder, #999);
}
.wiki-tag-panel__body {
  padding-top: 6px;
}
.wiki-tag-panel__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.wiki-tag-panel__create {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--td-component-stroke, #dcdcdc);
  background: transparent;
  border-radius: 4px;
  font-size: 12px;
  padding: 3px 8px;
  cursor: pointer;
  color: var(--td-text-color-secondary);
}
.wiki-tag-panel__refresh {
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--td-text-color-placeholder, #999);
}
.wiki-tag-panel__refresh:disabled {
  cursor: wait;
  opacity: 0.6;
}
.wiki-tag-panel__loading {
  padding: 6px 0;
}
.wiki-tag-panel__empty {
  font-size: 12px;
  color: var(--td-text-color-placeholder, #999);
  padding: 8px 0;
}
.wiki-tag-panel__empty-hint {
  font-size: 11px;
  color: var(--td-text-color-disabled, #bbb);
  margin-top: 4px;
}
.wiki-tag-panel__list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.wiki-tag-panel__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 0;
  gap: 8px;
}
.wiki-tag-panel__chip {
  flex: 1;
  min-width: 0;
}
.wiki-tag-panel__chip-count {
  margin-left: 4px;
  font-size: 11px;
  opacity: 0.7;
}
.wiki-tag-panel__item-actions {
  display: inline-flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.1s;
}
.wiki-tag-panel__item:hover .wiki-tag-panel__item-actions {
  opacity: 1;
}
.wiki-tag-panel__icon-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  padding: 2px;
  border-radius: 3px;
  color: var(--td-text-color-secondary);
}
.wiki-tag-panel__icon-btn:hover {
  background: var(--td-component-color-hover, #f0f0f0);
}
.wiki-tag-panel__icon-btn--danger {
  color: var(--td-error-color, #d54941);
}
</style>