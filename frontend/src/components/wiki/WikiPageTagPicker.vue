<template>
  <section class="wiki-page-tag-picker" :data-loading="loading">
    <div v-if="loading && assigned.length === 0" class="wiki-page-tag-picker__loading">
      <t-skeleton :row="1" />
    </div>

    <div v-else class="wiki-page-tag-picker__row">
      <div class="wiki-page-tag-picker__chips">
        <t-tag
          v-for="tag in assigned"
          :key="tag.id"
          :theme="themeFor(tag.color)"
          variant="light"
          :closable="!readonly"
          :data-tag-id="tag.id"
          @close="removeTag(tag.id)"
        >
          {{ tag.name }}
        </t-tag>

        <button
          v-if="!readonly && remainingTags.length > 0"
          type="button"
          class="wiki-page-tag-picker__add"
          :disabled="saving || atLimit"
          :title="atLimit ? t('wiki.tags.error.limitReached') : t('wiki.tags.add')"
          @click="openPicker = !openPicker"
        >
          <t-icon name="add" size="14px" />
          <span>{{ t('wiki.tags.add') }}</span>
        </button>

        <span v-if="assigned.length === 0" class="wiki-page-tag-picker__empty">
          {{ t('wiki.tags.pageEmpty') }}
        </span>
      </div>
    </div>

    <!-- dropdown picker -->
    <div v-if="openPicker" class="wiki-page-tag-picker__popover">
      <div class="wiki-page-tag-picker__popover-header">
        <input
          v-model="filterText"
          type="text"
          class="wiki-page-tag-picker__search"
          :placeholder="t('wiki.tags.selectPlaceholder')"
        />
      </div>
      <ul class="wiki-page-tag-picker__list">
        <li
          v-for="tag in filteredRemaining"
          :key="tag.id"
          class="wiki-page-tag-picker__option"
          @click="addTag(tag.id)"
        >
          <t-tag :theme="themeFor(tag.color)" variant="light">
            {{ tag.name }}
          </t-tag>
          <span class="wiki-page-tag-picker__option-count">
            {{ tag.page_count }}
          </span>
        </li>
        <li v-if="filteredRemaining.length === 0" class="wiki-page-tag-picker__none">
          {{ t('wiki.tags.empty') }}
        </li>
      </ul>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon as TIcon, Skeleton as TSkeleton, Tag as TTag, MessagePlugin } from 'tdesign-vue-next'
import { useWikiTagsStore } from '../../stores/wikiTags'
import type { WikiTag, WikiTagColor, WikiTagWithCount } from '../../api/wiki/tags'

const props = defineProps<{
  kbId: string
  slug: string
  readonly?: boolean
}>()

const { t } = useI18n()
const store = useWikiTagsStore()

const assigned = computed<WikiTag[]>(() => store.tagsFor(props.slug))
const loading = computed(
  () => store.isLoadingPage(props.slug) || (store.loading && assigned.value.length === 0),
)
const saving = computed(() => store.isSavingPage(props.slug))

const openPicker = ref(false)
const filterText = ref('')

// atLimit guards against adding past WikiTagMaxPerPage (10) on the
// client side. The backend also enforces this and returns 400.
const atLimit = computed(() => assigned.value.length >= 10)

const remainingTags = computed<WikiTagWithCount[]>(() => {
  const assignedIds = new Set(assigned.value.map((t) => t.id))
  return store.tags.filter((tag) => !assignedIds.has(tag.id))
})

const filteredRemaining = computed<WikiTagWithCount[]>(() => {
  const q = filterText.value.trim().toLowerCase()
  if (!q) return remainingTags.value
  return remainingTags.value.filter((t) => t.name.toLowerCase().includes(q))
})

// Theme mapping keeps the picker visually consistent with the rest of
// the wiki UI. We avoid passing raw palette names to TTag — TDesign has
// a fixed theme set, so each color lands on the closest match.
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

onMounted(async () => {
  // Pull both the dictionary and the per-page set in parallel — they
  // are independent round-trips and the picker needs the union.
  await Promise.all([
    store.fetchTags(props.kbId),
    store.fetchPageTags(props.kbId, props.slug),
  ])
})

// Re-fetch page tags whenever the slug changes — the parent may reuse
// the component when navigating between pages.
watch(
  () => props.slug,
  () => {
    if (props.slug) {
      store.fetchPageTags(props.kbId, props.slug)
    }
  },
)

async function addTag(tagId: string): Promise<void> {
  if (props.readonly || atLimit.value) return
  const next = [...assigned.value.map((t) => t.id), tagId]
  try {
    await store.setPageTags(props.kbId, props.slug, next)
    openPicker.value = false
    filterText.value = ''
  } catch (e) {
    MessagePlugin.error(
      t('wiki.tags.error.saveFailed', {
        detail: (e as { message?: string }).message ?? String(e),
      }),
    )
  }
}

async function removeTag(tagId: string): Promise<void> {
  if (props.readonly) return
  const next = assigned.value
    .filter((t) => t.id !== tagId)
    .map((t) => t.id)
  try {
    await store.setPageTags(props.kbId, props.slug, next)
  } catch (e) {
    MessagePlugin.error(
      t('wiki.tags.error.saveFailed', {
        detail: (e as { message?: string }).message ?? String(e),
      }),
    )
  }
}
</script>

<style scoped>
.wiki-page-tag-picker {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.wiki-page-tag-picker__row {
  min-height: 24px;
}
.wiki-page-tag-picker__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.wiki-page-tag-picker__add {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  height: 24px;
  border: 1px dashed var(--td-component-stroke, #dcdcdc);
  border-radius: var(--td-radius-small, 4px);
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.wiki-page-tag-picker__add:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
.wiki-page-tag-picker__empty {
  font-size: 12px;
  color: var(--td-text-color-placeholder, #999);
}
.wiki-page-tag-picker__popover {
  position: relative;
  margin-top: 4px;
  padding: 6px;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #dcdcdc);
  border-radius: var(--td-radius-small, 4px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}
.wiki-page-tag-picker__popover-header {
  margin-bottom: 4px;
}
.wiki-page-tag-picker__search {
  width: 100%;
  border: 1px solid var(--td-component-stroke, #dcdcdc);
  border-radius: 3px;
  padding: 4px 6px;
  font-size: 12px;
  outline: none;
}
.wiki-page-tag-picker__list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 200px;
  overflow-y: auto;
}
.wiki-page-tag-picker__option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 6px;
  border-radius: 3px;
  cursor: pointer;
  font-size: 13px;
}
.wiki-page-tag-picker__option:hover {
  background: var(--td-component-color-hover, #f5f5f5);
}
.wiki-page-tag-picker__option-count {
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
}
.wiki-page-tag-picker__none {
  font-size: 12px;
  color: var(--td-text-color-placeholder, #999);
  padding: 6px;
  text-align: center;
}
.wiki-page-tag-picker__loading {
  min-height: 24px;
}
</style>