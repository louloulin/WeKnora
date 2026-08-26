<template>
  <TDialog
    v-model:visible="visible"
    :header="headerTitle"
    :footer="false"
    width="480px"
    destroy-on-close
  >
    <div class="wiki-tag-dialog">
      <div class="wiki-tag-dialog__field">
        <label class="wiki-tag-dialog__label">
          {{ t('wiki.tags.dialog.nameLabel') }}
        </label>
        <TInput
          v-model="name"
          :placeholder="t('wiki.tags.dialog.namePlaceholder')"
          :status="errors.name ? 'error' : undefined"
          @blur="validate"
        />
        <div v-if="errors.name" class="wiki-tag-dialog__error">
          {{ errors.name }}
        </div>
      </div>

      <div class="wiki-tag-dialog__field">
        <label class="wiki-tag-dialog__label">
          {{ t('wiki.tags.dialog.colorLabel') }}
        </label>
        <div class="wiki-tag-dialog__palette">
          <button
            v-for="c in palette"
            :key="c"
            type="button"
            class="wiki-tag-dialog__swatch"
            :class="[
              `wiki-tag-dialog__swatch--${c}`,
              { 'is-active': draft.color === c },
            ]"
            :aria-label="c"
            @click="draft.color = c"
          />
        </div>
      </div>

      <div class="wiki-tag-dialog__actions">
        <TButton variant="outline" @click="close">
          {{ t('wiki.tags.dialog.cancel') }}
        </TButton>
        <TButton theme="primary" :loading="saving" @click="onSave">
          {{ t('wiki.tags.dialog.save') }}
        </TButton>
      </div>
    </div>
  </TDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  Button as TButton,
  Dialog as TDialog,
  Input as TInput,
  MessagePlugin,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useWikiTagsStore } from '../../stores/wikiTags'
import { WikiTagPalette, type WikiTagColor } from '../../api/wiki/tags'

interface Props {
  kbId: string
  // Edit mode: pass an existing tag to pre-fill the dialog. Undefined
  // creates a new tag.
  tag?: {
    id: string
    name: string
    color: WikiTagColor
  }
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'saved', id: string): void
  (e: 'cancel'): void
}>()

const { t } = useI18n()
const store = useWikiTagsStore()

const visible = ref(true)
const saving = ref(false)
const name = ref(props.tag?.name ?? '')

const draft = reactive<{ color: WikiTagColor }>({
  color: props.tag?.color ?? 'blue',
})

const errors = reactive<{ name?: string }>({})

const palette = WikiTagPalette

const headerTitle = computed(() =>
    props.tag
      ? t('wiki.tags.dialog.editTitle', { name: props.tag.name })
      : t('wiki.tags.dialog.createTitle'),
)

// Reset the draft when the dialog opens with a different tag.
watch(
  () => props.tag?.id,
  () => {
    name.value = props.tag?.name ?? ''
    draft.color = props.tag?.color ?? 'blue'
    errors.name = undefined
  },
)

function validate(): void {
  const trimmed = name.value.trim()
  if (!trimmed) {
    errors.name = t('wiki.tags.error.nameEmpty')
    return
  }
  if (trimmed.length > 64) {
    errors.name = t('wiki.tags.error.nameTooLong')
    return
  }
  errors.name = undefined
}

async function onSave(): Promise<void> {
  validate()
  if (errors.name) return
  saving.value = true
  try {
    if (props.tag) {
      const updated = await store.updateTag(props.kbId, props.tag.id, {
        name: name.value.trim(),
        color: draft.color,
      })
      if (updated) {
        MessagePlugin.success(t('wiki.tags.dialog.updateSuccess'))
        emit('saved', updated.id)
        close()
      } else {
        MessagePlugin.error(t('wiki.tags.error.updateFailed'))
      }
    } else {
      const created = await store.createTag(props.kbId, {
        name: name.value.trim(),
        color: draft.color,
      })
      if (created) {
        MessagePlugin.success(t('wiki.tags.dialog.createSuccess'))
        emit('saved', created.id)
        close()
      } else {
        MessagePlugin.error(t('wiki.tags.error.createFailed'))
      }
    }
  } catch (e) {
    const msg = (e as { message?: string }).message ?? String(e)
    MessagePlugin.error(t('wiki.tags.error.saveFailed', { detail: msg }))
  } finally {
    saving.value = false
  }
}

function close(): void {
  visible.value = false
  emit('cancel')
}
</script>

<style scoped>
.wiki-tag-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.wiki-tag-dialog__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.wiki-tag-dialog__label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-color-secondary, #555);
}
.wiki-tag-dialog__error {
  font-size: 12px;
  color: var(--error-color, #d54941);
}
.wiki-tag-dialog__palette {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.wiki-tag-dialog__swatch {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
  transition: transform 0.1s;
}
.wiki-tag-dialog__swatch:hover {
  transform: scale(1.08);
}
.wiki-tag-dialog__swatch.is-active {
  border-color: var(--text-color-primary, #181818);
}
.wiki-tag-dialog__swatch--blue   { background: #2b7fff; }
.wiki-tag-dialog__swatch--green  { background: #2ba471; }
.wiki-tag-dialog__swatch--orange { background: #e37318; }
.wiki-tag-dialog__swatch--red    { background: #d54941; }
.wiki-tag-dialog__swatch--purple { background: #8a3ffc; }
.wiki-tag-dialog__swatch--teal   { background: #16a3a3; }
.wiki-tag-dialog__swatch--gray   { background: #888888; }
.wiki-tag-dialog__swatch--gold   { background: #c08a3e; }
.wiki-tag-dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}
</style>