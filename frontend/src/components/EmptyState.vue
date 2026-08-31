<template>
  <div class="empty-state" :class="{ 'empty-state--compact': compact, 'empty-state--inline': inline }">
    <div v-if="hasIcon" class="empty-state__icon">
      <slot name="icon">
        <img v-if="imageSrc" class="empty-state__img" :src="imageSrc" :alt="title || ''">
        <t-icon v-else-if="icon" :name="icon" :size="iconSize" />
      </slot>
    </div>
    <div v-if="hasTitle" class="empty-state__title">
      <slot name="title">{{ title }}</slot>
    </div>
    <div v-if="hasDescription" class="empty-state__desc">
      <slot name="description">{{ description }}</slot>
    </div>
    <div v-if="hasActions" class="empty-state__actions">
      <slot name="actions">
        <t-button v-if="actionLabel" :theme="actionTheme" @click="onAction">
          <template v-if="actionIcon" #icon><t-icon :name="actionIcon" /></template>
          {{ actionLabel }}
        </t-button>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Icon as TIcon, Button as TButton } from 'tdesign-vue-next'

/**
 * Unified empty-state primitive.
 *
 * Replaces the per-view bespoke empty markup in
 * KnowledgeBaseList.vue (.empty-state), WikiBrowser.vue
 * (.wiki-empty-state / .wiki-reader-empty), AgentList.vue
 * (.empty-state), etc. — all of which diverged in padding,
 * icon treatment and CTA placement, and were called out in
 * section 16 of docs/enterprise-capability-analysis.html.
 *
 * The component exposes:
 *  - icon: TDesign icon name OR imageSrc for raster art
 *  - title / description slots + props
 *  - actions slot OR a single action button
 *
 * Slots win over props when both are provided, so views that
 * need richer markup (multi-button, link list, illustration)
 * can fall back to slots without losing the layout.
 */
const props = withDefaults(
  defineProps<{
    icon?: string
    imageSrc?: string
    iconSize?: string
    title?: string
    description?: string
    actionLabel?: string
    actionIcon?: string
    actionTheme?: 'primary' | 'default' | 'success' | 'warning' | 'danger'
    compact?: boolean
    inline?: boolean
  }>(),
  {
    icon: undefined,
    imageSrc: undefined,
    iconSize: '48px',
    title: undefined,
    description: undefined,
    actionLabel: undefined,
    actionIcon: undefined,
    actionTheme: 'primary',
    compact: false,
    inline: false,
  },
)

const emit = defineEmits<{
  (e: 'action'): void
}>()

const hasIcon = computed(() => Boolean(props.icon || props.imageSrc))
const hasTitle = computed(() => Boolean(props.title))
const hasDescription = computed(() => Boolean(props.description))
const hasActions = computed(() => Boolean(props.actionLabel))

const onAction = () => emit('action')
</script>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  text-align: center;
  color: var(--td-text-color-secondary);
  width: 100%;
}

.empty-state--compact {
  padding: 32px 16px;
}

.empty-state--inline {
  padding: 24px 16px;
}

.empty-state__icon {
  margin-bottom: 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-placeholder);
}

.empty-state__img {
  width: 162px;
  height: 162px;
  margin-bottom: 20px;
  background: transparent;
  border-radius: 0;
}

.empty-state--compact .empty-state__img {
  width: 96px;
  height: 96px;
  margin-bottom: 12px;
}

.empty-state__title {
  font-size: 16px;
  font-weight: 600;
  line-height: 26px;
  color: var(--td-text-color-placeholder);
  margin: 0 0 8px;
  max-width: 480px;
}

.empty-state--compact .empty-state__title {
  font-size: 14px;
  line-height: 22px;
  margin-bottom: 4px;
}

.empty-state__desc {
  font-size: 14px;
  font-weight: 400;
  line-height: 22px;
  color: var(--td-text-color-disabled);
  margin: 0;
  max-width: 520px;
}

.empty-state--compact .empty-state__desc {
  font-size: 13px;
  line-height: 20px;
}

.empty-state__actions {
  margin-top: 20px;
  display: inline-flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: center;
}

.empty-state--compact .empty-state__actions {
  margin-top: 12px;
}
</style>
