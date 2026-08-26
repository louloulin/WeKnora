<template>
  <TDrawer
    v-model:visible="visible"
    :header="headerTitle"
    :size="`520px`"
    :close-on-esc-key="true"
    :footer="false"
    destroy-on-close
    @open="onOpen"
    @close="onClose"
  >
    <div class="wiki-batch-job-history">
      <div v-if="loading" class="wiki-batch-job-history__loading">
        <TSkeleton :row="4" />
      </div>

      <div v-else-if="!events.length" class="wiki-batch-job-history__empty">
        {{ t('wiki.batchAudit.empty') }}
      </div>

      <ol v-else class="wiki-batch-job-history__list">
        <li
          v-for="(e, i) in events"
          :key="e.id"
          class="wiki-batch-job-history__item"
          :class="{ 'is-terminal': isTerminal(e.action) }"
        >
          <div class="wiki-batch-job-history__item-head">
            <span class="wiki-batch-job-history__action">{{ actionLabel(e.action) }}</span>
            <span class="wiki-batch-job-history__actor">
              {{ actorLabel(e.actor_id) }}
            </span>
            <span class="wiki-batch-job-history__time">
              {{ formatTime(e.occurred_at) }}
            </span>
          </div>
          <div
            v-if="e.metadata && Object.keys(e.metadata).length"
            class="wiki-batch-job-history__metadata"
          >
            <code>{{ formatMetadata(e.metadata) }}</code>
          </div>
          <div
            v-if="i < events.length - 1"
            class="wiki-batch-job-history__connector"
            aria-hidden="true"
          />
        </li>
      </ol>
    </div>
  </TDrawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  Drawer as TDrawer,
  Skeleton as TSkeleton,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  getWikiBatchJobAudit,
  WikiBatchAuditActorSystem,
} from '../../api/wiki'
import type { WikiBatchJobAuditEvent, WikiBatchAuditAction } from '../../api/wiki'
import {
  isTerminalAuditAction,
  jobDrawerTitleTokens,
} from './wikiBatchAuditLogic'

const props = defineProps<{
  kbId: string
  jobId: string
}>()

const { t, locale } = useI18n()

const visible = defineModel<boolean>('visible', { default: false })

const events = ref<WikiBatchJobAuditEvent[]>([])
const loading = ref(false)

const headerTitle = computed(() =>
  t('wiki.batchAudit.jobDrawerTitle', jobDrawerTitleTokens(props.jobId)),
)

async function onOpen() {
  loading.value = true
  try {
    events.value = await getWikiBatchJobAudit(props.kbId, props.jobId)
  } catch (err) {
    events.value = []
  } finally {
    loading.value = false
  }
}

function onClose() {
  events.value = []
}

function actionLabel(a: WikiBatchAuditAction): string {
  return t(`wiki.batchAudit.action.${a}`)
}

function actorLabel(actor: string): string {
  if (actor === WikiBatchAuditActorSystem) {
    return t('wiki.batchAudit.actor.system')
  }
  return t('wiki.batchAudit.actor.user', { name: actor })
}

function isTerminal(a: WikiBatchAuditAction): boolean {
  return isTerminalAuditAction(a)
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso)
    return d.toLocaleString(locale.value)
  } catch {
    return iso
  }
}

function formatMetadata(m: Record<string, unknown>): string {
  try {
    return JSON.stringify(m)
  } catch {
    return String(m)
  }
}
</script>

<style scoped lang="less">
.wiki-batch-job-history {
  padding: 0 4px;

  &__empty,
  &__loading {
    padding: 24px 0;
    text-align: center;
    color: var(--td-text-color-secondary, #666);
  }

  &__list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  &__item {
    position: relative;
    padding: 12px 12px 12px 28px;
    border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);

    &:last-child {
      border-bottom: none;
    }

    &.is-terminal .wiki-batch-job-history__action {
      color: var(--td-brand-color, #0052d9);
      font-weight: 600;
    }
  }

  &__item-head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
  }

  &__action {
    font-weight: 500;
  }

  &__actor {
    color: var(--td-text-color-secondary, #666);
    font-size: 12px;
  }

  &__time {
    margin-left: auto;
    color: var(--td-text-color-placeholder, #999);
    font-size: 12px;
  }

  &__metadata {
    margin-top: 6px;
    font-size: 12px;
    color: var(--td-text-color-secondary, #666);
    overflow-wrap: anywhere;
  }

  &__connector {
    position: absolute;
    left: 14px;
    top: 28px;
    bottom: -1px;
    width: 2px;
    background: var(--td-component-stroke, #e7e7e7);
  }
}
</style>