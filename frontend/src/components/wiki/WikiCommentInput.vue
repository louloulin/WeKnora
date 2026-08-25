<template>
  <div class="wiki-comment-input" :class="{ 'is-editing': isEditing }">
    <textarea
      ref="textareaRef"
      v-model="text"
      :placeholder="placeholder"
      :maxlength="maxLength"
      rows="3"
      @keydown="onKeydown"
      @input="onInput"
    />
    <div v-if="mentionStore.mentionOpen" class="wiki-comment-input__mentions">
      <div v-if="mentionStore.mentionLoading" class="wiki-comment-input__mention-loading">
        {{ t('wiki.comments.mention.loading') }}
      </div>
      <ul v-else>
        <li
          v-for="(cand, idx) in mentionStore.mentionCandidates"
          :key="cand.userId"
          :class="{ 'is-active': idx === activeCandidateIdx }"
          @mousedown.prevent="selectMention(cand)"
        >
          <span class="wiki-comment-input__mention-name">@{{ cand.displayName }}</span>
          <span v-if="cand.handle" class="wiki-comment-input__mention-handle">
            {{ cand.handle }}
          </span>
        </li>
        <li v-if="!mentionStore.mentionCandidates.length" class="is-empty">
          {{ t('wiki.comments.mention.empty') }}
        </li>
      </ul>
    </div>
    <div class="wiki-comment-input__bar">
      <span class="wiki-comment-input__count">
        {{ text.length }} / {{ maxLength }}
      </span>
      <div class="wiki-comment-input__actions">
        <TButton
          v-if="isEditing"
          theme="default"
          variant="text"
          size="small"
          @click="cancel"
        >
          {{ t('common.cancel') }}
        </TButton>
        <TButton
          theme="primary"
          size="small"
          :disabled="!canSubmit"
          @click="submit"
        >
          {{ isEditing ? t('common.save') : t('wiki.comments.submit') }}
        </TButton>
      </div>
    </div>
    <div v-if="errorMsg" class="wiki-comment-input__error">{{ errorMsg }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Button as TButton } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useWikiCommentsStore } from '../../stores/wikiComments'
import type {
  WikiCommentMention,
  WikiMentionCandidate,
} from '../../api/wiki/comments'

const props = defineProps<{
  kbId: string
  slug: string
  initialBody?: string
  initialMentions?: WikiCommentMention[]
  placeholder?: string
  isEditing?: boolean
}>()

const emit = defineEmits<{
  (e: 'submit', payload: { body: string; mentions: WikiCommentMention[] }): void
  (e: 'cancel'): void
}>()

const { t } = useI18n()
const mentionStore = useWikiCommentsStore()

const MAX_LEN = 4096
const maxLength = MAX_LEN

const textareaRef = ref<HTMLTextAreaElement | null>(null)
const text = ref(props.initialBody ?? '')
const mentions = ref<WikiCommentMention[]>(props.initialMentions ? [...props.initialMentions] : [])
const activeCandidateIdx = ref(0)
const errorMsg = ref('')

const canSubmit = computed(() => text.value.trim().length > 0 && text.value.length <= MAX_LEN)

watch(
  () => props.initialBody,
  (val) => {
    if (val !== undefined && val !== text.value) text.value = val
  },
)

watch(
  () => props.isEditing,
  (val) => {
    if (val) {
      nextTick(() => textareaRef.value?.focus())
    }
  },
)

function caretIndex(): number {
  const ta = textareaRef.value
  return ta?.selectionStart ?? text.value.length
}

function detectMentionTrigger(): { query: string; start: number } | null {
  const caret = caretIndex()
  // Scan backwards from caret to find the most recent '@' that is at start
  // or preceded by whitespace — ensures "@foo" inside "email@foo" is ignored.
  const upto = text.value.slice(0, caret)
  const match = /(?:^|\s)@(\S*)$/.exec(upto)
  if (!match) return null
  const query = match[1] ?? ''
  const start = caret - query.length - 1
  return { query, start }
}

async function onInput(): Promise<void> {
  const trigger = detectMentionTrigger()
  if (!trigger) {
    mentionStore.resetMentions()
    activeCandidateIdx.value = 0
    return
  }
  await mentionStore.searchMentions(props.kbId, trigger.query)
  activeCandidateIdx.value = 0
}

function selectMention(cand: WikiMentionCandidate): void {
  const trigger = detectMentionTrigger()
  if (!trigger) {
    mentionStore.resetMentions()
    return
  }
  const before = text.value.slice(0, trigger.start)
  const after = text.value.slice(caretIndex())
  const insertion = `@${cand.displayName} `
  text.value = before + insertion + after
  // Dedupe by userId, also normalize handle.
  const existing = mentions.value.findIndex((m) => m.userId === cand.userId)
  const next: WikiCommentMention = {
    userId: cand.userId,
    displayName: cand.displayName,
    handle: cand.handle,
    avatarUrl: cand.avatarUrl,
  }
  if (existing >= 0) mentions.value[existing] = next
  else mentions.value.push(next)
  mentionStore.resetMentions()
  activeCandidateIdx.value = 0
  nextTick(() => {
    const ta = textareaRef.value
    if (!ta) return
    const caret = (before + insertion).length
    ta.focus()
    ta.setSelectionRange(caret, caret)
  })
}

function onKeydown(ev: KeyboardEvent): void {
  if (!mentionStore.mentionOpen || !mentionStore.mentionCandidates.length) return
  if (ev.key === 'ArrowDown') {
    ev.preventDefault()
    activeCandidateIdx.value =
      (activeCandidateIdx.value + 1) % mentionStore.mentionCandidates.length
  } else if (ev.key === 'ArrowUp') {
    ev.preventDefault()
    activeCandidateIdx.value =
      (activeCandidateIdx.value - 1 + mentionStore.mentionCandidates.length) %
      mentionStore.mentionCandidates.length
  } else if (ev.key === 'Enter') {
    ev.preventDefault()
    const cand = mentionStore.mentionCandidates[activeCandidateIdx.value]
    if (cand) selectMention(cand)
  } else if (ev.key === 'Escape') {
    mentionStore.resetMentions()
  }
}

function submit(): void {
  if (!canSubmit.value) return
  errorMsg.value = ''
  emit('submit', {
    body: text.value.trim(),
    mentions: mentions.value.filter(
      (m) => m.userId && text.value.includes(`@${m.displayName}`),
    ),
  })
  if (!props.isEditing) {
    text.value = ''
    mentions.value = []
  }
}

function cancel(): void {
  text.value = props.initialBody ?? ''
  mentions.value = props.initialMentions ? [...props.initialMentions] : []
  errorMsg.value = ''
  mentionStore.resetMentions()
  emit('cancel')
}

defineExpose({ focus: () => textareaRef.value?.focus() })
</script>

<style scoped lang="less">
.wiki-comment-input {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--component-border, #dcdfe6);
  border-radius: 6px;
  padding: 8px;
  background: var(--bg-color-container, #fff);

  &.is-editing {
    border-color: var(--brand-color, #0052d9);
  }

  textarea {
    width: 100%;
    resize: vertical;
    border: none;
    outline: none;
    font-family: inherit;
    font-size: 14px;
    line-height: 1.5;
    background: transparent;
    color: var(--text-color-primary, #181818);
  }

  &__mentions {
    position: absolute;
    bottom: calc(100% + 4px);
    left: 0;
    right: 0;
    background: var(--bg-color-container, #fff);
    border: 1px solid var(--component-border, #dcdfe6);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
    max-height: 220px;
    overflow-y: auto;
    z-index: 10;

    ul {
      list-style: none;
      margin: 0;
      padding: 4px 0;
    }

    li {
      padding: 6px 12px;
      cursor: pointer;
      display: flex;
      align-items: center;
      gap: 8px;

      &.is-active,
      &:hover {
        background: var(--brand-color-light, #e6f0ff);
      }

      &.is-empty {
        color: var(--text-color-placeholder, #a6a6a6);
        cursor: default;
      }
    }
  }

  &__mention-name {
    font-weight: 500;
  }

  &__mention-handle {
    color: var(--text-color-placeholder, #a6a6a6);
    font-size: 12px;
  }

  &__mention-loading {
    padding: 8px 12px;
    color: var(--text-color-placeholder, #a6a6a6);
    font-size: 13px;
  }

  &__bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  &__count {
    font-size: 12px;
    color: var(--text-color-placeholder, #a6a6a6);
  }

  &__actions {
    display: flex;
    gap: 6px;
  }

  &__error {
    color: var(--error-color, #d54941);
    font-size: 12px;
  }
}
</style>