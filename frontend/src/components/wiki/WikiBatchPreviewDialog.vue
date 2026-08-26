<template>
  <TDialog
    v-model:visible="visible"
    :header="t('wiki.batchPreview.title', { type: typeLabel })"
    :confirm-btn="confirmBtn"
    :cancel-btn="t('wiki.batchPreview.cancel')"
    :on-confirm="onConfirm"
    :on-close="onClose"
    width="640px"
    :close-on-esc-key="true"
    destroy-on-close
  >
    <div class="wiki-batch-preview-dialog" data-testid="wiki-batch-preview-dialog">
      <!-- Summary header -->
      <div
        v-if="preview"
        class="wiki-batch-preview-dialog__summary"
        data-testid="wiki-batch-preview-summary"
      >
        <span class="wiki-batch-preview-dialog__stat is-success">
          <TIcon name="check-circle-filled" />
          {{ t('wiki.batchPreview.willSucceed', { count: preview.summary.will_succeed }) }}
        </span>
        <span
          v-if="preview.summary.will_fail > 0"
          class="wiki-batch-preview-dialog__stat is-fail"
        >
          <TIcon name="error-circle-filled" />
          {{ t('wiki.batchPreview.willFail', { count: preview.summary.will_fail }) }}
        </span>
        <span class="wiki-batch-preview-dialog__stat is-total">
          {{ t('wiki.batchPreview.summary', { total: preview.summary.total }) }}
        </span>
      </div>

      <!-- Empty body (zero slugs after dedup) -->
      <div
        v-if="preview && preview.summary.total === 0"
        class="wiki-batch-preview-dialog__empty"
      >
        {{ t('wiki.batchPreview.empty') }}
      </div>

      <!-- Per-slug table -->
      <table
        v-else-if="preview"
        class="wiki-batch-preview-dialog__table"
        data-testid="wiki-batch-preview-table"
      >
        <thead>
          <tr>
            <th class="col-status">&nbsp;</th>
            <th class="col-slug">slug</th>
            <th class="col-code">{{ t('wiki.batchPreview.codeHeader') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in tableRows"
            :key="row.key"
            :class="{ 'is-fail': row.kind === 'fail' }"
          >
            <td class="col-status">
              <TIcon
                v-if="row.kind === 'success'"
                name="check-circle-filled"
                class="wiki-batch-preview-dialog__icon is-success"
              />
              <TIcon
                v-else
                name="error-circle-filled"
                class="wiki-batch-preview-dialog__icon is-fail"
              />
            </td>
            <td class="col-slug">
              <code>{{ row.slug }}</code>
            </td>
            <td class="col-code">
              <span
                v-if="row.kind === 'fail'"
                class="wiki-batch-preview-dialog__code"
                :class="`is-${row.code}`"
              >
                {{ codeLabel(row.code) }}
              </span>
              <span v-else class="wiki-batch-preview-dialog__code is-success">
                {{ t('wiki.batchPreview.willSucceedTag') }}
              </span>
              <div v-if="row.kind === 'fail' && row.error" class="wiki-batch-preview-dialog__err">
                {{ row.error }}
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- Loading placeholder -->
      <div v-else class="wiki-batch-preview-dialog__loading">
        <TSkeleton :row="4" />
      </div>
    </div>
  </TDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  Dialog as TDialog,
  Icon as TIcon,
  Skeleton as TSkeleton,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  WikiBatchErrorCodeToI18nKey,
} from '../../api/wiki'
import type {
  WikiBatchPreviewResponse,
  WikiBatchPreviewType,
} from '../../api/wiki'

const props = defineProps<{
  visible: boolean
  previewType: WikiBatchPreviewType
  preview: WikiBatchPreviewResponse | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'confirm'): void
}>()

const { t } = useI18n()

const visible = computed({
  get: () => props.visible,
  set: (v) => emit('update:visible', v),
})

const typeLabel = computed(() => t(`wiki.batchPreview.typeLabel.${props.previewType}`))

const confirmBtn = computed(() => t('wiki.batchPreview.confirm'))

// tableRows merges success + failed into a single display list. Success
// rows come from `preview.success` (slug only); failed rows come from
// `preview.failed` with code + error. Order: success first (input order),
// then failed (also input order). Stable across renders.
const tableRows = computed(() => {
  const rows: Array<{
    key: string
    slug: string
    kind: 'success' | 'fail'
    code: string
    error: string
  }> = []
  if (!props.preview) return rows
  for (const slug of props.preview.success) {
    rows.push({ key: `s:${slug}`, slug, kind: 'success', code: '', error: '' })
  }
  for (const f of props.preview.failed) {
    rows.push({ key: `f:${f.slug}`, slug: f.slug, kind: 'fail', code: f.code, error: f.error })
  }
  return rows
})

function codeLabel(code: string): string {
  const key = WikiBatchErrorCodeToI18nKey[code]
  if (key) return t(key)
  return t('wiki.batchPreview.unknownCode', { code })
}

function onConfirm() {
  emit('confirm')
  visible.value = false
}

function onClose() {
  // User closed the dialog without confirming — that's a cancel, not a
  // confirm. The parent keeps the slug selection so the user can reopen
  // the dialog or hit the original "确认" button on the bar.
  emit('update:visible', false)
}
</script>

<style scoped>
.wiki-batch-preview-dialog {
  display: flex;
  flex-direction: column;
  gap: var(--td-size-3, 12px);
}

.wiki-batch-preview-dialog__summary {
  display: flex;
  gap: var(--td-size-3, 12px);
  flex-wrap: wrap;
  padding: var(--td-size-2, 8px) var(--td-size-3, 12px);
  background: var(--td-component-color-light, #f3f3f3);
  border-radius: var(--td-radius-medium, 6px);
}

.wiki-batch-preview-dialog__stat {
  display: inline-flex;
  align-items: center;
  gap: var(--td-size-1, 4px);
  font-size: var(--td-font-size-base, 14px);
}

.wiki-batch-preview-dialog__stat.is-success {
  color: var(--td-success-color, #2ba471);
}

.wiki-batch-preview-dialog__stat.is-fail {
  color: var(--td-error-color, #d54941);
}

.wiki-batch-preview-dialog__stat.is-total {
  color: var(--td-text-color-secondary);
  margin-left: auto;
}

.wiki-batch-preview-dialog__empty {
  padding: var(--td-size-4, 16px);
  text-align: center;
  color: var(--td-text-color-secondary);
}

.wiki-batch-preview-dialog__loading {
  padding: var(--td-size-3, 12px);
}

.wiki-batch-preview-dialog__table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--td-font-size-base, 14px);
}

.wiki-batch-preview-dialog__table th,
.wiki-batch-preview-dialog__table td {
  padding: var(--td-size-2, 8px);
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
  text-align: left;
  vertical-align: top;
}

.wiki-batch-preview-dialog__table th {
  font-weight: 500;
  color: var(--td-text-color-secondary);
  background: var(--td-component-color-light, #f3f3f3);
}

.wiki-batch-preview-dialog__table tr.is-fail {
  background: rgba(213, 73, 65, 0.04);
}

.wiki-batch-preview-dialog__icon.is-success {
  color: var(--td-success-color, #2ba471);
}

.wiki-batch-preview-dialog__icon.is-fail {
  color: var(--td-error-color, #d54941);
}

.wiki-batch-preview-dialog__code {
  display: inline-block;
  padding: 2px 8px;
  border-radius: var(--td-radius-small, 3px);
  font-size: var(--td-font-size-small, 12px);
  background: var(--td-error-color-light, #fbe5e3);
  color: var(--td-error-color, #d54941);
}

.wiki-batch-preview-dialog__code.is-success {
  background: var(--td-success-color-light, #e3f7ed);
  color: var(--td-success-color, #2ba471);
}

.wiki-batch-preview-dialog__err {
  margin-top: var(--td-size-1, 4px);
  font-size: var(--td-font-size-small, 12px);
  color: var(--td-text-color-secondary);
  word-break: break-word;
}

.col-status {
  width: 32px;
}
.col-slug {
  width: 40%;
}
.col-code {
  width: auto;
}
</style>