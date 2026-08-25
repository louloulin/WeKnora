<template>
  <t-dialog
    v-model:visible="modelVisible"
    :header="$t('knowledgeEditor.wikiBrowser.userTemplate.title')"
    :footer="false"
    width="640px"
    class="wiki-user-template-dialog"
    @close="resetForm"
  >
    <div class="wiki-user-template-body">
      <div class="wiki-user-template-list">
        <div class="wiki-user-template-list-header">
          <span>{{ $t('knowledgeEditor.wikiBrowser.userTemplate.listTitle') }}</span>
          <t-button size="small" theme="primary" @click="startCreate">
            {{ $t('knowledgeEditor.wikiBrowser.userTemplate.addBtn') }}
          </t-button>
        </div>
        <div v-if="records.length === 0" class="wiki-user-template-empty">
          {{ $t('knowledgeEditor.wikiBrowser.userTemplate.empty') }}
        </div>
        <ul v-else class="wiki-user-template-records">
          <li v-for="record in records" :key="record.id" class="wiki-user-template-record">
            <div class="wiki-user-template-record-meta">
              <div class="wiki-user-template-record-label">{{ record.label }}</div>
              <div class="wiki-user-template-record-preview">{{ previewOf(record.content) }}</div>
            </div>
            <div class="wiki-user-template-record-actions">
              <t-button size="small" variant="outline" @click="startEdit(record)">
                {{ $t('common.edit') }}
              </t-button>
              <t-button size="small" variant="outline" theme="danger" @click="confirmRemove(record)">
                {{ $t('common.delete') }}
              </t-button>
            </div>
          </li>
        </ul>
      </div>

      <div v-if="editing" class="wiki-user-template-editor">
        <div class="wiki-user-template-editor-title">
          {{ editing.id ? $t('knowledgeEditor.wikiBrowser.userTemplate.editTitle') : $t('knowledgeEditor.wikiBrowser.userTemplate.addTitle') }}
        </div>
        <div class="wiki-user-template-field">
          <label>{{ $t('knowledgeEditor.wikiBrowser.userTemplate.nameLabel') }}</label>
          <t-input v-model="form.label" :maxlength="MAX_NAME_LENGTH" show-limit-number
            :placeholder="$t('knowledgeEditor.wikiBrowser.userTemplate.namePlaceholder')" />
        </div>
        <div class="wiki-user-template-field">
          <label>{{ $t('knowledgeEditor.wikiBrowser.userTemplate.contentLabel') }}</label>
          <t-textarea v-model="form.content" :autosize="{ minRows: 8, maxRows: 20 }"
            :maxlength="MAX_CONTENT_LENGTH" show-limit-number
            :placeholder="$t('knowledgeEditor.wikiBrowser.userTemplate.contentPlaceholder')" />
        </div>
        <div class="wiki-user-template-editor-actions">
          <t-button variant="outline" @click="cancelEdit">
            {{ $t('common.cancel') }}
          </t-button>
          <t-button theme="primary" :disabled="!form.label.trim()" @click="commitEdit">
            {{ editing.id ? $t('common.save') : $t('common.create') }}
          </t-button>
        </div>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { useUserTemplates, type UserTemplateRecord } from '@/composables/useUserTemplates'

/**
 * WikiUserTemplateDialog — manage user-defined wiki page templates
 * (Build #4). Backed by `useUserTemplates()` composable, which writes to
 * localStorage. Two modes:
 *
 *   * No template selected → list view, with an "Add" button to switch
 *     into the editor.
 *   * A template (or draft) is being edited → editor view; "Cancel"
 *     returns to the list view, "Save" / "Create" commits the form.
 *
 * The dialog is a controlled component (`v-model:visible`) so the parent
 * (WikiBrowser) can open / close it without holding internal state.
 */

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()

const { t } = useI18n()
const {
  templates,
  addTemplate,
  updateTemplate,
  removeTemplate,
  MAX_NAME_LENGTH,
  MAX_CONTENT_LENGTH,
} = useUserTemplates()

const modelVisible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const records = computed(() => templates.value)

interface EditingState {
  id: string | null
  label: string
  content: string
}

const editing = ref<EditingState | null>(null)
const form = reactive({ label: '', content: '' })

function resetForm() {
  editing.value = null
  form.label = ''
  form.content = ''
}

function startCreate() {
  editing.value = { id: null, label: '', content: '' }
  form.label = ''
  form.content = ''
}

function startEdit(record: UserTemplateRecord) {
  editing.value = { id: record.id, label: record.label, content: record.content }
  form.label = record.label
  form.content = record.content
}

function cancelEdit() {
  resetForm()
}

function commitEdit() {
  if (!editing.value) return
  const label = form.label.trim()
  if (!label) {
    MessagePlugin.warning(t('knowledgeEditor.wikiBrowser.userTemplate.nameRequired'))
    return
  }
  if (editing.value.id) {
    updateTemplate(editing.value.id, { label, content: form.content })
    MessagePlugin.success(t('knowledgeEditor.wikiBrowser.userTemplate.updateSuccess'))
  } else {
    addTemplate(label, form.content)
    MessagePlugin.success(t('knowledgeEditor.wikiBrowser.userTemplate.createSuccess'))
  }
  resetForm()
}

function confirmRemove(record: UserTemplateRecord) {
  const confirmDialog = DialogPlugin.confirm({
    header: t('knowledgeEditor.wikiBrowser.userTemplate.deleteTitle'),
    body: t('knowledgeEditor.wikiBrowser.userTemplate.deleteConfirm', { name: record.label }),
    onConfirm: () => {
      removeTemplate(record.id)
      MessagePlugin.success(t('knowledgeEditor.wikiBrowser.userTemplate.deleteSuccess'))
      confirmDialog.destroy()
    },
    onCancel: () => {
      confirmDialog.destroy()
    },
  })
}

function previewOf(content: string): string {
  const trimmed = content.trim().replace(/\s+/g, ' ')
  if (trimmed.length <= 80) return trimmed
  return trimmed.slice(0, 80) + '…'
}
</script>

<style scoped>
.wiki-user-template-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 60vh;
  overflow-y: auto;
}

.wiki-user-template-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-weight: 600;
}

.wiki-user-template-empty {
  color: var(--text-secondary, #888);
  padding: 24px 0;
  text-align: center;
}

.wiki-user-template-records {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.wiki-user-template-record {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border: 1px solid var(--component-stroke, #e7e7e7);
  border-radius: 6px;
}

.wiki-user-template-record-meta {
  flex: 1 1 auto;
  min-width: 0;
}

.wiki-user-template-record-label {
  font-weight: 600;
  margin-bottom: 2px;
}

.wiki-user-template-record-preview {
  color: var(--text-secondary, #888);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.wiki-user-template-record-actions {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

.wiki-user-template-editor {
  border-top: 1px solid var(--component-stroke, #e7e7e7);
  padding-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.wiki-user-template-editor-title {
  font-weight: 600;
}

.wiki-user-template-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.wiki-user-template-field label {
  font-size: 13px;
  color: var(--text-secondary, #555);
}

.wiki-user-template-editor-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>