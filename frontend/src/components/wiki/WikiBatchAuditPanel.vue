<template>
  <TDrawer
    v-model:visible="visible"
    :header="headerTitle"
    :size="`640px`"
    :close-on-esc-key="true"
    :footer="false"
    destroy-on-close
    @open="onOpen"
    @close="onClose"
  >
    <div class="wiki-batch-audit-panel">
      <div class="wiki-batch-audit-panel__filters">
        <TSelect
          v-model="filter.action"
          :placeholder="t('wiki.batchAudit.filterAction')"
          clearable
          size="small"
          style="width: 160px"
        >
          <TOption
            v-for="action in actionOptions"
            :key="action"
            :label="t(`wiki.batchAudit.action.${action}`)"
            :value="action"
          />
        </TSelect>
        <TInput
          v-model="filter.actor"
          :placeholder="t('wiki.batchAudit.filterActor')"
          size="small"
          style="width: 160px"
          clearable
        />
        <TButton size="small" theme="default" @click="reload">
          {{ t('common.refresh') }}
        </TButton>
        <TButton size="small" theme="default" @click="onExport">
          {{ t('wiki.batchAudit.export') }}
        </TButton>
      </div>

      <div v-if="loading" class="wiki-batch-audit-panel__loading">
        <TSkeleton :row="6" />
      </div>

      <div v-else-if="!events.length" class="wiki-batch-audit-panel__empty">
        {{ t('wiki.batchAudit.empty') }}
      </div>

      <table v-else class="wiki-batch-audit-panel__table">
        <thead>
          <tr>
            <th>{{ t('wiki.batchAudit.table.action') }}</th>
            <th>{{ t('wiki.batchAudit.table.actor') }}</th>
            <th>{{ t('wiki.batchAudit.table.job') }}</th>
            <th>{{ t('wiki.batchAudit.table.occurred') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in events" :key="e.id">
            <td>
              <span :class="{ 'is-terminal': isTerminal(e.action) }">
                {{ t(`wiki.batchAudit.action.${e.action}`) }}
              </span>
            </td>
            <td>{{ e.actor_id }}</td>
            <td>
              <TLink
                theme="primary"
                size="small"
                @click="onOpenJob(e.batch_job_id)"
              >
                {{ shortJobId(e.batch_job_id) }}
              </TLink>
            </td>
            <td>{{ formatTime(e.occurred_at) }}</td>
          </tr>
        </tbody>
      </table>

      <div class="wiki-batch-audit-panel__pagination">
        <TPagination
          v-model:current="filter.page"
          v-model:page-size="filter.page_size"
          :total="total"
          :page-size-options="[20, 50, 100]"
          size="small"
          show-page-size
          @change="reload"
        />
      </div>
    </div>

    <WikiBatchJobHistory
      v-if="drawerJobId"
      v-model:visible="jobHistoryVisible"
      :kb-id="kbId"
      :job-id="drawerJobId"
    />
  </TDrawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import {
  Button as TButton,
  Drawer as TDrawer,
  Input as TInput,
  Link as TLink,
  MessagePlugin,
  Option as TOption,
  Pagination as TPagination,
  Select as TSelect,
  Skeleton as TSkeleton,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  exportWikiBatchAuditCsv,
  listWikiBatchAudit,
} from '../../api/wiki'
import type {
  WikiBatchAuditAction,
  WikiBatchJobAuditEvent,
} from '../../api/wiki'
import {
  AllAuditActions,
  isTerminalAuditAction,
  shortJobId,
} from './wikiBatchAuditLogic'
import WikiBatchJobHistory from './WikiBatchJobHistory.vue'

const props = defineProps<{
  kbId: string
}>()

const { t, locale } = useI18n()

const visible = defineModel<boolean>('visible', { default: false })

const events = ref<WikiBatchJobAuditEvent[]>([])
const total = ref(0)
const loading = ref(false)

const filter = reactive({
  actor: '' as string,
  action: '' as WikiBatchAuditAction | '',
  page: 1,
  page_size: 50,
})

const actionOptions: WikiBatchAuditAction[] = AllAuditActions

const jobHistoryVisible = ref(false)
const drawerJobId = ref('')

const headerTitle = computed(() => t('wiki.batchAudit.kbDrawerTitle'))

function isTerminal(a: WikiBatchAuditAction): boolean {
  return isTerminalAuditAction(a)
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString(locale.value)
  } catch {
    return iso
  }
}

async function reload() {
  loading.value = true
  try {
    const r = await listWikiBatchAudit(props.kbId, {
      actor: filter.actor || undefined,
      action: filter.action || undefined,
      page: filter.page,
      page_size: filter.page_size,
    })
    events.value = r.events
    total.value = r.total
  } catch {
    events.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

async function onExport() {
  try {
    const blob = await exportWikiBatchAuditCsv(props.kbId, {
      actor: filter.actor || undefined,
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `wiki-batch-audit-${props.kbId}.csv`
    a.click()
    URL.revokeObjectURL(url)
  } catch (err: unknown) {
    MessagePlugin.error(t('wiki.batchAudit.exportFailed'))
  }
}

function openJobHistory(jobId: string) {
  drawerJobId.value = jobId
  jobHistoryVisible.value = true
}

function onOpenJob(jobId: string) {
  openJobHistory(jobId)
}

function onOpen() {
  reload()
}

function onClose() {
  events.value = []
  total.value = 0
}
</script>

<style scoped lang="less">
.wiki-batch-audit-panel {
  padding: 0 4px;

  &__filters {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
    flex-wrap: wrap;
  }

  &__loading,
  &__empty {
    padding: 24px 0;
    text-align: center;
    color: var(--td-text-color-secondary, #666);
  }

  &__table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;

    th,
    td {
      padding: 8px 6px;
      border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
      text-align: left;
    }

    th {
      color: var(--td-text-color-secondary, #666);
      font-weight: 500;
    }

    .is-terminal {
      color: var(--td-brand-color, #0052d9);
      font-weight: 600;
    }
  }

  &__pagination {
    margin-top: 16px;
    display: flex;
    justify-content: flex-end;
  }
}
</style>