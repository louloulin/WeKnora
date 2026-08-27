<template>
  <TDrawer
    v-model:visible="visible"
    :header="headerTitle"
    :size="'480px'"
    :close-on-esc-key="true"
    :footer="false"
    destroy-on-close
    @open="onOpen"
    @close="onClose"
  >
    <div class="wiki-comment-drawer">
      <div class="wiki-comment-drawer__hint">
        {{ t('wiki.comments.hint') }}
      </div>

      <div v-if="loading" class="wiki-comment-drawer__loading">
        <TSkeleton :row="3" />
      </div>

      <div
        v-else-if="!comments.length"
        class="wiki-comment-drawer__empty"
      >
        {{ t('wiki.comments.empty') }}
      </div>

      <ul v-else class="wiki-comment-drawer__list">
        <li v-for="c in comments" :key="c.id" class="wiki-comment-drawer__item">
          <div class="wiki-comment-drawer__item-head">
            <div class="wiki-comment-drawer__author">
              <TAvatar :image="c.authorAvatarUrl" :alt="c.authorName" size="28px">
                {{ initials(c.authorName) }}
              </TAvatar>
              <span class="wiki-comment-drawer__author-name">{{ c.authorName }}</span>
              <span class="wiki-comment-drawer__time">{{ formatTime(c.createdAt) }}</span>
            </div>
            <div v-if="canModify(c)" class="wiki-comment-drawer__actions">
              <TButton
                v-if="editingId !== c.id"
                theme="default"
                variant="text"
                size="small"
                @click="startEdit(c)"
              >
                {{ t('common.edit') }}
              </TButton>
              <TButton
                theme="danger"
                variant="text"
                size="small"
                @click="confirmDelete(c)"
              >
                {{ t('common.delete') }}
              </TButton>
            </div>
          </div>

          <WikiCommentInput
            v-if="editingId === c.id"
            :kb-id="kbId"
            :slug="slug"
            :initial-body="c.body"
            :initial-mentions="c.mentions"
            :is-editing="true"
            @submit="onUpdateSubmit(c.id, $event)"
            @cancel="cancelEdit"
          />
          <div v-else class="wiki-comment-drawer__body" v-text="formatBody(c.body)" />
          <div v-if="c.mentions?.length" class="wiki-comment-drawer__mentions">
            <span
              v-for="m in c.mentions"
              :key="m.userId"
              class="wiki-comment-drawer__mention"
            >
              @{{ m.displayName }}
            </span>
          </div>
        </li>
      </ul>

      <div v-if="!editingId" class="wiki-comment-drawer__compose">
        <WikiCommentInput
          :kb-id="kbId"
          :slug="slug"
          @submit="onCreateSubmit"
        />
      </div>
    </div>
  </TDrawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Avatar as TAvatar, Button as TButton, Drawer as TDrawer, Skeleton as TSkeleton, DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useWikiCommentsStore } from '../../stores/wikiComments'
import type {
  WikiComment,
  WikiCommentMention,
} from '../../api/wiki/comments'
import WikiCommentInput from './WikiCommentInput.vue'

const props = defineProps<{
  visible: boolean
  kbId: string
  slug: string
  pageTitle?: string
  /** Current user id; comments authored by this user can be edited/deleted. */
  currentUserId?: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
}>()

const { t, locale } = useI18n()
const store = useWikiCommentsStore()

const editingId = ref<string | null>(null)

const visible = computed({
  get: () => props.visible,
  set: (val: boolean) => emit('update:visible', val),
})

const headerTitle = computed(() => {
  const title = props.pageTitle?.trim()
  return title ? t('wiki.comments.titleFor', { title }) : t('wiki.comments.title')
})

const comments = computed(() => store.commentsFor(props.slug))
const loading = computed(() => store.isLoading(props.slug))

function initials(name: string): string {
  if (!name) return '?'
  const trimmed = name.trim()
  if (!trimmed) return '?'
  // CJK names: take first character; Latin names: take first letter of each word.
  if (/[一-龥]/.test(trimmed)) return trimmed.slice(-2)
  const parts = trimmed.split(/\s+/)
  return (parts[0]?.[0] ?? '?') + (parts[1]?.[0] ?? '')
}

function formatTime(iso: string): string {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return ''
    return d.toLocaleString(locale.value, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return ''
  }
}

function formatBody(body: string): string {
  // Trim trailing whitespace; keep newlines as-is (drawer body uses white-space: pre-wrap).
  return body.replace(/[ \t]+\n/g, '\n').trimEnd()
}

function canModify(comment: WikiComment): boolean {
  if (!props.currentUserId) return false
  return comment.authorId === props.currentUserId
}

async function onOpen(): Promise<void> {
  await store.fetchComments(props.kbId, props.slug)
}

function onClose(): void {
  editingId.value = null
  store.resetMentions()
}

watch(
  () => props.slug,
  async (next, prev) => {
    if (next && next !== prev) {
      editingId.value = null
      await store.fetchComments(props.kbId, next)
    }
  },
)

function startEdit(c: WikiComment): void {
  editingId.value = c.id
}

function cancelEdit(): void {
  editingId.value = null
}

async function onCreateSubmit(payload: {
  body: string
  mentions: WikiCommentMention[]
}): Promise<void> {
  const created = await store.addComment(props.kbId, props.slug, payload)
  if (created) {
    MessagePlugin.success(t('wiki.comments.createSuccess'))
  } else if (store.error) {
    MessagePlugin.error(t('wiki.comments.error.' + store.error))
  }
}

async function onUpdateSubmit(
  id: string,
  payload: { body: string; mentions: WikiCommentMention[] },
): Promise<void> {
  const updated = await store.editComment(props.kbId, props.slug, id, payload)
  if (updated) {
    editingId.value = null
    MessagePlugin.success(t('wiki.comments.updateSuccess'))
  } else if (store.error) {
    MessagePlugin.error(t('wiki.comments.error.' + store.error))
  }
}

function confirmDelete(c: WikiComment): void {
  const dialog = DialogPlugin.confirm({
    header: t('wiki.comments.deleteTitle'),
    body: t('wiki.comments.deleteConfirm', { author: c.authorName }),
    onConfirm: async () => {
      const ok = await store.removeComment(props.kbId, props.slug, c.id)
      if (ok) MessagePlugin.success(t('wiki.comments.deleteSuccess'))
      dialog.hide()
    },
    onClose: () => dialog.hide(),
  })
}
</script>

<style scoped lang="less">
.wiki-comment-drawer {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 200px;

  &__hint {
    font-size: 12px;
    color: var(--text-color-secondary, #666);
    background: var(--bg-color-secondary, #f6f6f6);
    padding: 6px 10px;
    border-radius: 4px;
  }

  &__loading,
  &__empty {
    padding: 24px 8px;
    text-align: center;
    color: var(--text-color-placeholder, #a6a6a6);
    font-size: 13px;
  }

  &__list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
    max-height: 60vh;
    overflow-y: auto;
  }

  &__item {
    background: var(--bg-color-secondary, #f6f6f6);
    border-radius: 6px;
    padding: 10px 12px;
  }

  &__item-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }

  &__author {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  &__author-name {
    font-weight: 500;
    font-size: 13px;
  }

  &__time {
    color: var(--text-color-placeholder, #a6a6a6);
    font-size: 12px;
  }

  &__actions {
    display: flex;
    gap: 4px;
  }

  &__body {
    font-size: 14px;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--text-color-primary, #181818);
  }

  &__mentions {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 6px;
  }

  &__mention {
    background: var(--brand-color-light, #e6f0ff);
    color: var(--brand-color, #0052d9);
    border-radius: 4px;
    padding: 1px 6px;
    font-size: 12px;
  }

  &__compose {
    border-top: 1px solid var(--component-border, #dcdfe6);
    padding-top: 12px;
  }
}
</style>