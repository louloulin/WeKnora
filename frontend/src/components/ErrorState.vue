<template>
  <div class="error-state" :class="{ 'error-state--compact': compact }">
    <div class="error-state__icon">
      <slot name="icon">
        <t-icon :name="iconName" :size="iconSize" />
      </slot>
    </div>
    <div v-if="hasTitle" class="error-state__title">
      <slot name="title">{{ title }}</slot>
    </div>
    <div v-if="hasDescription" class="error-state__desc">
      <slot name="description">{{ description }}</slot>
    </div>
    <div v-if="errorCode || lastSyncedAt" class="error-state__meta">
      <span v-if="errorCode" class="error-state__code">{{ errorCodeLabel }}: {{ errorCode }}</span>
      <span v-if="lastSyncedAt" class="error-state__sync">{{ lastSyncedLabel }}: {{ lastSyncedAt }}</span>
    </div>
    <div v-if="hasActions" class="error-state__actions">
      <slot name="actions">
        <t-button v-if="retryLabel" :theme="retryTheme" :loading="retrying" @click="onRetry">
          <template #icon><t-icon name="refresh" /></template>
          {{ retryLabel }}
        </t-button>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Icon as TIcon, Button as TButton } from 'tdesign-vue-next'

/**
 * Unified error-state primitive with retry affordance.
 *
 * Addresses audit finding 16.7: "缺少'重试按钮 + 错误归因（鉴权/网络/服务端）
 * + 最近一次成功时间'". Existing fetch failures across views surface as
 * toasts and silently empty the list, leaving users with no recovery path.
 *
 * Usage:
 *   <ErrorState
 *     :retrying="fetching"
 *     title="加载失败"
 *     :description="errorMessage"
 *     error-code="NETWORK"
 *     retry-label="重试"
 *     @retry="fetchList"
 *   />
 */
const props = withDefaults(
  defineProps<{
    title?: string
    description?: string
    errorCode?: string | number
    iconName?: string
    iconSize?: string
    retryLabel?: string
    retryTheme?: 'primary' | 'default' | 'danger'
    retrying?: boolean
    lastSyncedAt?: string
    errorCodeLabel?: string
    lastSyncedLabel?: string
    compact?: boolean
  }>(),
  {
    title: undefined,
    description: undefined,
    errorCode: undefined,
    iconName: 'error-circle',
    iconSize: '48px',
    retryLabel: undefined,
    retryTheme: 'primary',
    retrying: false,
    lastSyncedAt: undefined,
    errorCodeLabel: '错误码',
    lastSyncedLabel: '最近一次成功',
    compact: false,
  },
)

const emit = defineEmits<{
  (e: 'retry'): void
}>()

const hasTitle = computed(() => Boolean(props.title))
const hasDescription = computed(() => Boolean(props.description))
const hasActions = computed(() => Boolean(props.retryLabel))

const onRetry = () => emit('retry')
</script>

<style scoped>
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 20px;
  text-align: center;
  color: var(--td-text-color-secondary);
  width: 100%;
}

.error-state--compact {
  padding: 24px 16px;
}

.error-state__icon {
  margin-bottom: 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background: var(--td-error-color-1, rgba(255, 123, 139, 0.12));
  color: var(--td-error-color);
}

.error-state__title {
  font-size: 16px;
  font-weight: 600;
  line-height: 26px;
  color: var(--td-text-color-primary);
  margin: 0 0 8px;
  max-width: 480px;
}

.error-state--compact .error-state__title {
  font-size: 14px;
  line-height: 22px;
}

.error-state__desc {
  font-size: 14px;
  font-weight: 400;
  line-height: 22px;
  color: var(--td-text-color-secondary);
  margin: 0;
  max-width: 520px;
}

.error-state__meta {
  margin-top: 12px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  display: inline-flex;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: center;
}

.error-state__code,
.error-state__sync {
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
}

.error-state__actions {
  margin-top: 20px;
  display: inline-flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: center;
}

.error-state--compact .error-state__actions {
  margin-top: 12px;
}
</style>
