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
    <div class="wiki-batch-failure-drawer">
      <div class="wiki-batch-failure-drawer__tabs">
        <button
          type="button"
          class="wiki-batch-failure-drawer__tab"
          :class="{ 'is-active': !filter.code }"
          @click="onPickCode('')"
        >
          <span>{{ t('wiki.batchFailures.tabAll') }}</span>
          <span class="wiki-batch-failure-drawer__tab-count">
            {{ totalCount }}
          </span>
        </button>
        <button
          v-for="g in groups"
          :key="g.code"
          type="button"
          class="wiki-batch-failure-drawer__tab"
          :class="{ 'is-active': filter.code === g.code }"
          @click="onPickCode(g.code)"
        >
          <span>{{ codeLabel(g.code) }}</span>
          <span class="wiki-batch-failure-drawer__tab-count">
            {{ g.count }}
          </span>
        </button>
      </div>

      <div v-if="loading" class="wiki-batch-failure-drawer__loading">
        <TSkeleton :row="6" />
      </div>

      <div v-else-if="!failures.length" class="wiki-batch-failure-drawer__empty">
        {{ t('wiki.batchFailures.empty') }}
      </div>

      <ul v-else class="wiki-batch-failure-drawer__list">
        <li
          v-for="row in failures"
          :key="row.id"
          class="wiki-batch-failure-drawer__item"
        >
          <div class="wiki-batch-failure-drawer__item-head">
            <span class="wiki-batch-failure-drawer__slug">
              <code>{{ row.slug }}</code>
            </span>
            <span
              class="wiki-batch-failure-drawer__code"
              :class="`is-${row.code}`"
            >
              {{ codeLabel(row.code) }}
            </span>
          </div>
          <div class="wiki-batch-failure-drawer__error">
            {{ row.error }}
          </div>
          <div class="wiki-batch-failure-drawer__time">
            {{ formatTime(row.occurred_at) }}
          </div>
        </li>
      </ul>

      <div class="wiki-batch-failure-drawer__pagination">
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
  </TDrawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  Drawer as TDrawer,
  Pagination as TPagination,
  Skeleton as TSkeleton,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  getWikiBatchJobFailures,
  WikiBatchErrorCodeToI18nKey,
} from '../../api/wiki'
import type {
  WikiBatchJobFailureRecord,
  WikiBatchFailureGroupCount,
} from '../../api/wiki'

const props = defineProps<{
  kbId: string
  jobId: string
}>()

const { t, locale } = useI18n()

const visible = defineModel<boolean>('visible', { default: false })

const failures = ref<WikiBatchJobFailureRecord[]>([])
const groups = ref<WikiBatchFailureGroupCount[]>([])
const total = ref(0)
const loading = ref(false)

const filter = reactive({
  code: '' as string,
  page: 1,
  page_size: 50,
})

const totalCount = computed(() =>
  groups.value.reduce((acc, g) => acc + g.count, 0),
)

const headerTitle = computed(() =>
  t('wiki.batchFailures.drawerTitle', {
    jobId: shortJobId(props.jobId),
  }),
)

watch(
  () => filter.code,
  () => {
    filter.page = 1
    if (visible.value) reload()
  },
)

async function onOpen() {
  filter.page = 1
  filter.code = ''
  await reload()
}

function onClose() {
  failures.value = []
  groups.value = []
  total.value = 0
}

function onPickCode(code: string) {
  filter.code = code || ''
}

async function reload() {
  loading.value = true
  try {
    const resp = await getWikiBatchJobFailures(props.kbId, props.jobId, {
      code: filter.code || undefined,
      page: filter.page,
      page_size: filter.page_size,
    })
    failures.value = resp.failures ?? []
    groups.value = resp.groups ?? []
    total.value = resp.total
  } catch {
    failures.value = []
    groups.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function codeLabel(code: string): string {
  const key = WikiBatchErrorCodeToI18nKey[code]
  if (key) return t(key)
  return code
}

function shortJobId(id: string): string {
  if (!id) return ''
  return id.length <= 8 ? id : id.slice(0, 8)
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString(locale.value)
  } catch {
    return iso
  }
}

defineExpose({ reload })
</script>

<style scoped lang="less">
.wiki-batch-failure-drawer {
  padding: 0 4px;

  &__tabs {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 12px;
  }

  &__tab {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    font-size: 12px;
    background: var(--td-bg-color-container, #f8f8f8);
    border: 1px solid var(--td-component-stroke, #e7e7e7);
    border-radius: 999px;
    color: var(--td-text-color-secondary, #666);
    cursor: pointer;
    transition: all 0.15s ease;

    &:hover {
      border-color: var(--td-brand-color, #0052d9);
      color: var(--td-brand-color, #0052d9);
    }

    &.is-active {
      background: var(--td-brand-color, #0052d9);
      border-color: var(--td-brand-color, #0052d9);
      color: #fff;
    }
  }

  &__tab-count {
    font-weight: 600;
  }

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
    max-height: calc(100vh - 240px);
    overflow-y: auto;
  }

  &__item {
    padding: 12px 8px;
    border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);

    &:last-child {
      border-bottom: none;
    }
  }

  &__item-head {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 4px;
  }

  &__slug {
    font-size: 13px;

    code {
      padding: 2px 6px;
      background: var(--td-bg-color-container, #f1f1f1);
      border-radius: 4px;
      font-family: var(--td-font-family-mono, monospace);
    }
  }

  &__code {
    margin-left: auto;
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 4px;
    background: var(--td-error-color-1, #ffe7e7);
    color: var(--td-error-color, #d54941);

    &.is-not_found {
      background: var(--td-warning-color-1, #fff3e0);
      color: var(--td-warning-color, #e37318);
    }

    &.is-internal {
      background: var(--td-error-color-1, #ffe7e7);
      color: var(--td-error-color, #d54941);
    }
  }

  &__error {
    font-size: 12px;
    color: var(--td-text-color-secondary, #555);
    overflow-wrap: anywhere;
    margin-bottom: 4px;
  }

  &__time {
    font-size: 11px;
    color: var(--td-text-color-placeholder, #999);
  }

  &__pagination {
    margin-top: 12px;
  }
}
</style>
