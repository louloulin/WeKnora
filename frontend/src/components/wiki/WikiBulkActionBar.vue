<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { listWikiFolders } from '@/api/wiki';
import { MessagePlugin } from 'tdesign-vue-next';

// Build #12 — 顶部批量操作工具栏。父组件持有 select-mode 状态 +
// 已选 slug 列表,通过 props 传入。本组件只负责渲染和发出意图,
// 不做实际网络请求(由父组件统一处理 move / status / delete)。
//
// Move / Status / Delete 三个动作的具体执行仍由父组件注入,因为:
// - move 之后需要刷新 directory tree(folder 计数变了)
// - delete 之后需要重新拉列表
// - status 切换后某些列表按 status 过滤
// 工具栏自身只显示已选数 + 弹窗 + 提交按钮 + toast。

const { t } = useI18n();

const props = defineProps<{
  kbId: string;
  selectedSlugs: string[];
  busy?: boolean;
}>();

const emit = defineEmits<{
  (e: 'clear'): void;
  (e: 'move', folderId: string): void;
  (e: 'status', status: 'draft' | 'published' | 'archived'): void;
  (e: 'delete'): void;
}>();

const count = computed(() => props.selectedSlugs.length);
const disabled = computed(() => count.value === 0 || props.busy);

// Move dialog state — inline t-dialog to keep the component self-contained.
const moveDialogVisible = ref(false);
const moveTargetFolderId = ref<string>('');
const folderChoices = ref<{ id: string; name: string; depth: number }[]>([]);

watch(moveDialogVisible, async (visible) => {
  if (!visible) return;
  moveTargetFolderId.value = '';
  try {
    const resp = await listWikiFolders(props.kbId);
    const folders = (resp as { folders?: { id: string; name: string; depth: number }[] }).folders ?? [];
    folderChoices.value = [{ id: '', name: '(root)', depth: 0 }, ...folders];
  } catch {
    MessagePlugin.warning(t('knowledgeEditor.wikiBrowser.bulkMoveLoadFoldersFailed'));
  }
});

function submitMove() {
  if (props.busy) return;
  emit('move', moveTargetFolderId.value);
  moveDialogVisible.value = false;
}

// Confirm dialog for delete — uses t-popconfirm semantic via t-dialog.
const deleteDialogVisible = ref(false);

function submitDelete() {
  if (props.busy) return;
  emit('delete');
  deleteDialogVisible.value = false;
}
</script>

<template>
  <div v-if="count > 0" class="wiki-bulk-action-bar" role="toolbar" aria-label="bulk actions">
    <div class="wiki-bulk-action-bar__count">
      <t-icon name="check-circle-filled" />
      <span>{{ t('knowledgeEditor.wikiBrowser.bulkBar', { count }) }}</span>
    </div>
    <div class="wiki-bulk-action-bar__actions">
      <t-button
        variant="outline"
        size="small"
        :disabled="disabled"
        data-testid="bulk-move"
        @click="moveDialogVisible = true"
      >
        <t-icon name="folder-open" />
        {{ t('knowledgeEditor.wikiBrowser.bulkMove') }}
      </t-button>
      <t-dropdown
        :disabled="disabled"
        trigger="click"
        placement="bottom-end"
      >
        <t-button variant="outline" size="small" :disabled="disabled" data-testid="bulk-status">
          <t-icon name="swap" />
          {{ t('knowledgeEditor.wikiBrowser.bulkStatus') }}
          <t-icon name="chevron-down" />
        </t-button>
        <template #dropdown>
          <t-dropdown-menu>
            <t-dropdown-item @click="emit('status', 'published')">
              {{ t('knowledgeEditor.wikiBrowser.status.published') }}
            </t-dropdown-item>
            <t-dropdown-item @click="emit('status', 'draft')">
              {{ t('knowledgeEditor.wikiBrowser.status.draft') }}
            </t-dropdown-item>
            <t-dropdown-item @click="emit('status', 'archived')">
              {{ t('knowledgeEditor.wikiBrowser.status.archived') }}
            </t-dropdown-item>
          </t-dropdown-menu>
        </template>
      </t-dropdown>
      <t-button
        variant="outline"
        theme="danger"
        size="small"
        :disabled="disabled"
        data-testid="bulk-delete"
        @click="deleteDialogVisible = true"
      >
        <t-icon name="delete" />
        {{ t('knowledgeEditor.wikiBrowser.bulkDelete') }}
      </t-button>
      <t-button
        variant="text"
        size="small"
        :disabled="props.busy"
        data-testid="bulk-clear"
        @click="emit('clear')"
      >
        {{ t('knowledgeEditor.wikiBrowser.bulkClear') }}
      </t-button>
    </div>

    <!-- Move dialog -->
    <t-dialog
      v-model:visible="moveDialogVisible"
      :header="t('knowledgeEditor.wikiBrowser.bulkMoveDialogTitle')"
      :confirm-btn="t('knowledgeEditor.wikiBrowser.bulkMoveConfirm')"
      :cancel-btn="t('common.cancel')"
      :on-confirm="submitMove"
      width="500px"
    >
      <p class="wiki-bulk-dialog-hint">
        {{ t('knowledgeEditor.wikiBrowser.bulkMoveDialogHint', { count }) }}
      </p>
      <t-select
        v-model="moveTargetFolderId"
        :placeholder="t('knowledgeEditor.wikiBrowser.bulkMoveSelectFolder')"
        filterable
        clearable
      >
        <t-option
          v-for="f in folderChoices"
          :key="f.id || '__root__'"
          :value="f.id"
          :label="f.name"
        />
      </t-select>
    </t-dialog>

    <!-- Delete dialog -->
    <t-dialog
      v-model:visible="deleteDialogVisible"
      :header="t('knowledgeEditor.wikiBrowser.bulkDeleteDialogTitle')"
      :confirm-btn="t('knowledgeEditor.wikiBrowser.bulkDeleteConfirm')"
      :cancel-btn="t('common.cancel')"
      :on-confirm="submitDelete"
      theme="danger"
      width="420px"
    >
      <p>{{ t('knowledgeEditor.wikiBrowser.bulkDeleteDialogHint', { count }) }}</p>
    </t-dialog>
  </div>
</template>

<style scoped>
.wiki-bulk-action-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--td-size-4, 16px);
  padding: var(--td-size-2, 8px) var(--td-size-3, 12px);
  background: var(--td-brand-color-light, var(--td-component-color-light));
  border: 1px solid var(--td-component-stroke, var(--td-gray-color-3));
  border-radius: var(--td-radius-medium, 6px);
  margin-bottom: var(--td-size-2, 8px);
  font-size: var(--td-font-size-base, 14px);
}

.wiki-bulk-action-bar__count {
  display: flex;
  align-items: center;
  gap: var(--td-size-1, 4px);
  color: var(--td-text-color-primary);
  font-weight: 500;
}

.wiki-bulk-action-bar__actions {
  display: flex;
  align-items: center;
  gap: var(--td-size-2, 8px);
  flex-wrap: wrap;
}

.wiki-bulk-dialog-hint {
  margin-bottom: var(--td-size-3, 12px);
  color: var(--td-text-color-secondary);
  font-size: var(--td-font-size-base, 14px);
}
</style>