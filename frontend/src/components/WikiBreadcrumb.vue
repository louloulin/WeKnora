<template>
  <nav v-if="visible" class="wiki-breadcrumb" :aria-label="ariaLabel">
    <span v-if="knowledgeBaseName" class="wiki-breadcrumb__root">
      <t-icon name="folder" size="12px" />
      <span class="wiki-breadcrumb__root-name">{{ knowledgeBaseName }}</span>
    </span>
    <span v-for="(segment, idx) in categoryPath" :key="`s-${idx}`" class="wiki-breadcrumb__segment"
      :aria-current="idx === categoryPath.length - 1 ? 'page' : undefined">
      <t-icon name="chevron-right" size="12px" class="wiki-breadcrumb__sep" />
      <button v-if="idx < categoryPath.length - 1" type="button" class="wiki-breadcrumb__link"
        :title="segment" @click="onSegmentClick(segment, idx)">
        {{ segment }}
      </button>
      <span v-else class="wiki-breadcrumb__text" :title="segment">{{ segment }}</span>
    </span>
    <span v-if="currentTitle" class="wiki-breadcrumb__current">
      <t-icon name="chevron-right" size="12px" class="wiki-breadcrumb__sep" />
      <span class="wiki-breadcrumb__text wiki-breadcrumb__text--current" :title="currentTitle">
        {{ currentTitle }}
      </span>
    </span>
    <span v-if="copyable" class="wiki-breadcrumb__actions">
      <t-tooltip :content="copyTip" placement="top">
        <button type="button" class="wiki-breadcrumb__action-btn"
          :aria-label="$t ? $t('wikiBreadcrumb.copyLink') : 'Copy link'"
          @click="onCopyLink">
          <t-icon :name="copyState === 'copied' ? 'check' : 'link'" size="12px" />
        </button>
      </t-tooltip>
    </span>
  </nav>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon as TIcon, Tooltip as TTooltip, MessagePlugin } from 'tdesign-vue-next'

/**
 * Wiki breadcrumb.
 *
 * Addresses audit finding §16.4 P1:
 * "页面树与面包屑: 顶级产品支持多面包屑层级 + 当前位置可复制链接"
 *
 * Renders:
 *   <KB name>  ›  <category[0]>  ›  <category[1]>  …  <current title>
 * The category segments come from `WikiPage.category_path` (already
 * provided by the server in the page object, so we do not need a
 * recursive folder-tree fetch). The current title is rendered
 * non-interactively; earlier segments are buttons that emit
 * `navigate` so the host can either open that ancestor page
 * or scroll to / focus the folder in the tree.
 *
 * A small copy-link action is appended so users can share the
 * deep link to the current page.
 */
const props = withDefaults(
  defineProps<{
    knowledgeBaseName?: string
    categoryPath?: string[]
    currentTitle?: string
    copyable?: boolean
  }>(),
  {
    knowledgeBaseName: '',
    categoryPath: () => [],
    currentTitle: '',
    copyable: true,
  },
)

const emit = defineEmits<{
  (e: 'navigate', payload: { segment: string; index: number }): void
}>()

const visible = computed(() =>
  Boolean(
    props.knowledgeBaseName ||
      (props.categoryPath && props.categoryPath.length > 0) ||
      props.currentTitle,
  ),
)

const ariaLabel = 'Wiki page breadcrumb'

const onSegmentClick = (segment: string, index: number) => {
  emit('navigate', { segment, index })
}

const copyState = ref<'idle' | 'copied'>('idle')
const copyTip = computed(() => {
  if (copyState.value === 'copied') return '已复制'
  return '复制当前页深链'
})

const onCopyLink = async () => {
  if (typeof window === 'undefined') return
  try {
    // Compose a shareable URL. The slug-style is the same path the Wiki
    // browser pushes when opening a page, so the link is round-trippable.
    const path = window.location.pathname
    const url = `${window.location.origin}${path}${window.location.search}`
    await navigator.clipboard.writeText(url)
    copyState.value = 'copied'
    MessagePlugin.success('已复制当前页链接')
    setTimeout(() => {
      copyState.value = 'idle'
    }, 1500)
  } catch (err: any) {
    MessagePlugin.error(err?.message || '复制失败')
  }
}
</script>

<style scoped>
.wiki-breadcrumb {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  font-size: 12px;
  line-height: 20px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  min-height: 32px;
  max-width: 100%;
  overflow-x: auto;
  scrollbar-width: thin;
}

.wiki-breadcrumb__root {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--td-text-color-secondary);
  flex-shrink: 0;
}

.wiki-breadcrumb__root-name {
  font-weight: 600;
}

.wiki-breadcrumb__segment {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.wiki-breadcrumb__current {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.wiki-breadcrumb__sep {
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

.wiki-breadcrumb__link {
  background: transparent;
  border: 0;
  padding: 2px 4px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 18px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: background 0.12s ease, color 0.12s ease;
  font-family: inherit;
}

.wiki-breadcrumb__link:hover {
  background: var(--td-bg-color-container-hover);
  color: var(--td-brand-color);
}

.wiki-breadcrumb__link:focus-visible {
  outline: 2px solid var(--td-brand-color);
  outline-offset: 1px;
}

.wiki-breadcrumb__text {
  padding: 2px 4px;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.wiki-breadcrumb__text--current {
  color: var(--td-text-color-primary);
  font-weight: 500;
}

.wiki-breadcrumb__actions {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.wiki-breadcrumb__action-btn {
  background: transparent;
  border: 0;
  padding: 4px;
  border-radius: 4px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background 0.12s ease, color 0.12s ease;
}

.wiki-breadcrumb__action-btn:hover {
  background: var(--td-bg-color-container-hover);
  color: var(--td-brand-color);
}

.wiki-breadcrumb__action-btn:focus-visible {
  outline: 2px solid var(--td-brand-color);
  outline-offset: 1px;
}
</style>
