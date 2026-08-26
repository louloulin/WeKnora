<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  parseTemplateBody,
  type ParsedTemplatePlaceholders,
  type WikiApplyTemplateRequest,
  type WikiApplyTemplateResult,
  type WikiTemplatePlaceholderChild,
  type WikiTemplatePlaceholderSection,
  type WikiTemplateSkeleton,
} from '../../api/wiki/templates'
import { useWikiTemplatesStore } from '../../stores/wikiTemplates'

interface Props {
  /** KB id of the parent page. */
  kbId: string
  /** Slug of the parent page that will own the auto children. */
  parentSlug: string
  /** Parent page title — surfaced in the dialog header. */
  parentTitle: string
  /** Saved template body; placeholders are parsed on mount. */
  templateBody: string
  /** Saved template id (stamped into SourceRefs). */
  templateId?: string
  /** Optional bodyAppend hint carried in from the caller's caller. */
  initialBodyAppend?: string
  /** Dialog visibility. */
  visible: boolean
}

const props = withDefaults(defineProps<Props>(), {
  templateId: '',
  initialBodyAppend: '',
})

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'applied', result: WikiApplyTemplateResult): void
}>()

const { t } = useI18n()
const store = useWikiTemplatesStore()

// --- Local form state -------------------------------------------------

const children = ref<WikiTemplatePlaceholderChild[]>([])
const sections = ref<WikiTemplatePlaceholderSection[]>([])
const taggedTokensInput = ref<string>('')
const bodyAppend = ref<string>(props.initialBodyAppend || '')
const previewBody = ref<string>('')
const lastResult = ref<WikiApplyTemplateResult | null>(null)

const parsed = computed<ParsedTemplatePlaceholders>(() =>
  parseTemplateBody(props.templateBody || ''),
)

const skeleton = computed<WikiTemplateSkeleton>(() => ({
  children: children.value,
  sections: sections.value,
  tagged_tokens: taggedTokensInput.value
    .split(/[,\s]+/)
    .map((s) => s.trim())
    .filter(Boolean),
}))

const buildRequest = (): WikiApplyTemplateRequest => ({
  template_id: props.templateId || undefined,
  body_append: bodyAppend.value || undefined,
  skeleton: skeleton.value,
})

// Reset local form whenever the dialog opens with a new template.
watch(
  () => [props.visible, props.templateBody, props.templateId] as const,
  ([visible]) => {
    if (!visible) return
    previewBody.value = ''
    lastResult.value = null
    // Seed the children list from any {{child_pages}} placeholder
    // present in the body — the user is expected to fill in concrete
    // titles before applying. We leave it empty otherwise; the user
    // can add rows via the "+ 子页面" button.
    children.value = []
    sections.value = []
    taggedTokensInput.value = parsed.value.taggedTokens.join(', ')
  },
  { immediate: true },
)

function addChild() {
  children.value.push({ title: '', content: '' })
}
function removeChild(idx: number) {
  children.value.splice(idx, 1)
}
function addSection() {
  sections.value.push({ title: '', body: '' })
}
function removeSection(idx: number) {
  sections.value.splice(idx, 1)
}

async function onPreview() {
  try {
    const result = await store.preview(props.kbId, props.parentSlug, buildRequest())
    previewBody.value = result.new_body || ''
    lastResult.value = result
    MessagePlugin.success(t('wiki.template.previewOk'))
  } catch (err) {
    MessagePlugin.error(
      t('wiki.template.previewFailed', { error: err instanceof Error ? err.message : String(err) }),
    )
  }
}

async function onApply() {
  if (children.value.length === 0 && sections.value.length === 0) {
    MessagePlugin.warning(t('wiki.template.emptySkeleton'))
    return
  }
  try {
    const result = await store.apply(props.kbId, props.parentSlug, buildRequest())
    lastResult.value = result
    MessagePlugin.success(
      t('wiki.template.applyOk', { count: result.pages.length }),
    )
    emit('applied', result)
    emit('update:visible', false)
  } catch (err) {
    MessagePlugin.error(
      t('wiki.template.applyFailed', { error: err instanceof Error ? err.message : String(err) }),
    )
  }
}

function close() {
  emit('update:visible', false)
}

const inFlight = computed(() => store.isInFlight(props.kbId, props.parentSlug))
const lastError = computed(() => store.lastError(props.kbId, props.parentSlug))
</script>

<template>
  <t-dialog
    :visible="visible"
    :header="t('wiki.template.title', { parent: parentTitle })"
    :width="720"
    :on-close="close"
    :on-confirm="onApply"
    :confirm-btn="t('wiki.template.apply')"
    :cancel-btn="t('common.cancel')"
    :confirm-loading="inFlight"
    :on-cancel="close"
    destroy-on-close
  >
    <div class="wiki-template-dialog">
      <p class="wiki-template-dialog__hint">
        {{ t('wiki.template.hint') }}
      </p>

      <section class="wiki-template-dialog__section">
        <header>
          <h4>{{ t('wiki.template.children') }}</h4>
          <t-button theme="default" size="small" @click="addChild">
            + {{ t('wiki.template.addChild') }}
          </t-button>
        </header>
        <div v-if="children.length === 0" class="wiki-template-dialog__empty">
          {{ t('wiki.template.noChildren') }}
        </div>
        <div v-else class="wiki-template-dialog__rows">
          <div
            v-for="(c, idx) in children"
            :key="idx"
            class="wiki-template-dialog__row"
          >
            <t-input
              v-model="c.title"
              :placeholder="t('wiki.template.childTitle')"
              class="wiki-template-dialog__row-title"
            />
            <t-input
              v-model="c.content"
              :placeholder="t('wiki.template.childContent')"
              class="wiki-template-dialog__row-content"
            />
            <t-button
              theme="danger"
              variant="text"
              size="small"
              @click="removeChild(idx)"
            >
              {{ t('common.delete') }}
            </t-button>
          </div>
        </div>
      </section>

      <section class="wiki-template-dialog__section">
        <header>
          <h4>{{ t('wiki.template.sections') }}</h4>
          <t-button theme="default" size="small" @click="addSection">
            + {{ t('wiki.template.addSection') }}
          </t-button>
        </header>
        <div v-if="sections.length === 0" class="wiki-template-dialog__empty">
          {{ t('wiki.template.noSections') }}
        </div>
        <div v-else class="wiki-template-dialog__rows">
          <div
            v-for="(s, idx) in sections"
            :key="idx"
            class="wiki-template-dialog__row"
          >
            <t-input
              v-model="s.title"
              :placeholder="t('wiki.template.sectionTitle')"
              class="wiki-template-dialog__row-title"
            />
            <t-input
              v-model="s.body"
              :placeholder="t('wiki.template.sectionBody')"
              class="wiki-template-dialog__row-content"
            />
            <t-button
              theme="danger"
              variant="text"
              size="small"
              @click="removeSection(idx)"
            >
              {{ t('common.delete') }}
            </t-button>
          </div>
        </div>
      </section>

      <section class="wiki-template-dialog__section">
        <h4>{{ t('wiki.template.taggedTokens') }}</h4>
        <t-input
          v-model="taggedTokensInput"
          :placeholder="t('wiki.template.taggedTokensPlaceholder')"
        />
        <p class="wiki-template-dialog__hint">
          {{ t('wiki.template.taggedTokensHint') }}
        </p>
      </section>

      <section class="wiki-template-dialog__section">
        <header>
          <h4>{{ t('wiki.template.preview') }}</h4>
          <t-button
            theme="default"
            size="small"
            :loading="inFlight"
            @click="onPreview"
          >
            {{ t('wiki.template.previewButton') }}
          </t-button>
        </header>
        <pre v-if="previewBody" class="wiki-template-dialog__preview">{{
          previewBody
        }}</pre>
        <div v-else class="wiki-template-dialog__empty">
          {{ t('wiki.template.previewEmpty') }}
        </div>
      </section>

      <div v-if="lastError" class="wiki-template-dialog__error">
        {{ lastError }}
      </div>
    </div>
  </t-dialog>
</template>

<style scoped>
.wiki-template-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 70vh;
  overflow-y: auto;
}
.wiki-template-dialog__hint {
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
  margin: 0;
}
.wiki-template-dialog__section {
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 4px;
  padding: 12px;
}
.wiki-template-dialog__section > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.wiki-template-dialog__section h4 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.wiki-template-dialog__empty {
  font-size: 12px;
  color: var(--td-text-color-secondary, #888);
  padding: 8px;
  text-align: center;
}
.wiki-template-dialog__rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.wiki-template-dialog__row {
  display: grid;
  grid-template-columns: 1fr 2fr auto;
  gap: 8px;
  align-items: center;
}
.wiki-template-dialog__preview {
  background: var(--td-bg-color-container, #f5f5f5);
  padding: 8px;
  border-radius: 4px;
  font-size: 12px;
  max-height: 200px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-word;
}
.wiki-template-dialog__error {
  color: var(--td-error-color, #d54941);
  font-size: 12px;
  padding: 8px;
  background: var(--td-error-color-1, #fff1f0);
  border-radius: 4px;
}
</style>