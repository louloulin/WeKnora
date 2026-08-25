<template>
  <TDialog
    v-model:visible="visible"
    :header="headerTitle"
    :footer="false"
    width="560px"
    destroy-on-close
    @open="onOpen"
    @close="onClose"
  >
    <div class="wiki-share-dialog">
      <div class="wiki-share-dialog__section">
        <div class="wiki-share-dialog__section-title">
          {{ t('wiki.share.createTitle') }}
        </div>
        <div class="wiki-share-dialog__form">
          <label class="wiki-share-dialog__field">
            <span>{{ t('wiki.share.expiry') }}</span>
            <TSelect v-model="form.expiresIn">
              <TOption value="24h" :label="t('wiki.share.expiry24h')" />
              <TOption value="7d" :label="t('wiki.share.expiry7d')" />
              <TOption value="30d" :label="t('wiki.share.expiry30d')" />
              <TOption value="never" :label="t('wiki.share.expiryNever')" />
            </TSelect>
          </label>
          <label class="wiki-share-dialog__field">
            <span>{{ t('wiki.share.passwordLabel') }}</span>
            <TInput
              v-model="form.password"
              type="password"
              :placeholder="t('wiki.share.passwordPlaceholder')"
            />
          </label>
          <div class="wiki-share-dialog__hint">
            {{ t('wiki.share.watermarkHint') }}
          </div>
          <div class="wiki-share-dialog__actions">
            <TButton
              theme="primary"
              :loading="creating"
              @click="submit"
            >
              {{ t('wiki.share.createBtn') }}
            </TButton>
          </div>
        </div>
      </div>

      <div class="wiki-share-dialog__section">
        <div class="wiki-share-dialog__section-title">
          {{ t('wiki.share.activeTitle') }}
          <span class="wiki-share-dialog__count">{{ activeShares.length }}</span>
        </div>
        <div v-if="loading" class="wiki-share-dialog__loading">
          <TSkeleton :row="3" />
        </div>
        <div v-else-if="!activeShares.length" class="wiki-share-dialog__empty">
          {{ t('wiki.share.empty') }}
        </div>
        <ul v-else class="wiki-share-dialog__list">
          <li v-for="s in activeShares" :key="s.id" class="wiki-share-dialog__item">
            <div class="wiki-share-dialog__item-head">
              <TButton
                size="small"
                variant="text"
                theme="primary"
                @click="copy(s)"
              >
                {{ t('wiki.share.copyBtn') }}
              </TButton>
              <span class="wiki-share-dialog__item-expiry">
                {{ formatExpiry(s) }}
              </span>
              <TButton
                size="small"
                variant="text"
                theme="danger"
                @click="confirmRevoke(s)"
              >
                {{ t('wiki.share.revokeBtn') }}
              </TButton>
            </div>
            <div class="wiki-share-dialog__item-url" :title="s.url">
              {{ truncate(s.url, 64) }}
            </div>
            <div class="wiki-share-dialog__item-meta">
              <span v-if="s.hasPassword" class="wiki-share-dialog__chip wiki-share-dialog__chip--lock">
                {{ t('wiki.share.passwordChip') }}
              </span>
              <span class="wiki-share-dialog__chip">
                {{ t('wiki.share.views', { count: s.viewCount }) }}
              </span>
              <span v-if="s.lastViewedAt" class="wiki-share-dialog__chip">
                {{ t('wiki.share.lastViewed', { at: formatTime(s.lastViewedAt) }) }}
              </span>
            </div>
          </li>
        </ul>
      </div>
    </div>
  </TDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  Button as TButton,
  Dialog as TDialog,
  DialogPlugin,
  Input as TInput,
  MessagePlugin,
  Option as TOption,
  Select as TSelect,
  Skeleton as TSkeleton,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useWikiShareLinksStore } from '../../stores/wikiShareLinks'
import type { WikiShareLink } from '../../api/wiki/share'

const props = defineProps<{
  visible: boolean
  kbId: string
  slug: string
  pageTitle?: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
}>()

const { t, locale } = useI18n()
const store = useWikiShareLinksStore()

const visible = computed({
  get: () => props.visible,
  set: (val: boolean) => emit('update:visible', val),
})

const headerTitle = computed(() =>
  props.pageTitle
    ? t('wiki.share.headerFor', { title: props.pageTitle })
    : t('wiki.share.header'),
)

const form = reactive({
  expiresIn: '7d' as '24h' | '7d' | '30d' | 'never',
  password: '',
})

const creating = ref(false)
const activeShares = computed(() => store.linksFor(props.kbId, props.slug))
const loading = computed(() => store.isLoading(props.kbId, props.slug))

async function onOpen(): Promise<void> {
  await store.fetchLinks(props.kbId, props.slug)
}

function onClose(): void {
  form.expiresIn = '7d'
  form.password = ''
}

watch(
  () => props.slug,
  async (next, prev) => {
    if (next && next !== prev) {
      await store.fetchLinks(props.kbId, next)
    }
  },
)

async function submit(): Promise<void> {
  if (creating.value) return
  creating.value = true
  try {
    const created = await store.createLink(props.kbId, props.slug, {
      expiresIn: form.expiresIn,
      password: form.password.trim() || undefined,
    })
    if (created) {
      MessagePlugin.success(t('wiki.share.createSuccess'))
      form.password = ''
      // Auto-copy the freshly minted link for fast pasting.
      await copy(created, false)
    } else if (store.error) {
      MessagePlugin.error(t('wiki.share.error.' + store.error))
    }
  } finally {
    creating.value = false
  }
}

async function copy(link: WikiShareLink, showToast = true): Promise<void> {
  try {
    await navigator.clipboard.writeText(link.url)
    if (showToast) MessagePlugin.success(t('wiki.share.copySuccess'))
  } catch {
    // Fallback for browsers without clipboard API: open the prompt so the
    // user can copy manually.
    window.prompt(t('wiki.share.copyFallback'), link.url)
  }
}

function confirmRevoke(link: WikiShareLink): void {
  const dialog = DialogPlugin.confirm({
    header: t('wiki.share.revokeTitle'),
    body: t('wiki.share.revokeConfirm'),
    onConfirm: async () => {
      const ok = await store.revokeLink(props.kbId, props.slug, link.id)
      if (ok) MessagePlugin.success(t('wiki.share.revokeSuccess'))
      dialog.hide()
    },
    onClose: () => dialog.hide(),
  })
}

function truncate(s: string, max: number): string {
  return s.length <= max ? s : `${s.slice(0, max - 1)}…`
}

function formatTime(iso: string): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString(locale.value, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return ''
  }
}

function formatExpiry(link: WikiShareLink): string {
  if (link.revokedAt) return t('wiki.share.expiryRevoked')
  if (!link.expiresAt) return t('wiki.share.expiryNeverLabel')
  const ts = new Date(link.expiresAt).getTime()
  const now = Date.now()
  if (ts <= now) return t('wiki.share.expiryExpired')
  return t('wiki.share.expiresAt', { at: formatTime(link.expiresAt) })
}
</script>

<style scoped lang="less">
.wiki-share-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;

  &__section {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  &__section-title {
    font-weight: 600;
    font-size: 14px;
    display: flex;
    align-items: center;
    gap: 6px;
  }

  &__count {
    background: var(--brand-color-light, #e6f0ff);
    color: var(--brand-color, #0052d9);
    border-radius: 10px;
    padding: 1px 8px;
    font-size: 12px;
    font-weight: 500;
  }

  &__form {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  &__field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13px;
    color: var(--text-color-secondary, #555);
  }

  &__hint {
    font-size: 12px;
    color: var(--text-color-placeholder, #999);
    background: var(--bg-color-secondary, #f6f6f6);
    border-radius: 4px;
    padding: 6px 10px;
  }

  &__actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 4px;
  }

  &__loading,
  &__empty {
    padding: 16px 8px;
    text-align: center;
    color: var(--text-color-placeholder, #999);
    font-size: 13px;
  }

  &__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-height: 320px;
    overflow-y: auto;
  }

  &__item {
    border: 1px solid var(--component-border, #dcdfe6);
    border-radius: 6px;
    padding: 8px 10px;
    background: var(--bg-color-container, #fff);
  }

  &__item-head {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  &__item-expiry {
    flex: 1;
    font-size: 12px;
    color: var(--text-color-placeholder, #999);
  }

  &__item-url {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12px;
    color: var(--text-color-secondary, #555);
    word-break: break-all;
    margin: 4px 0;
  }

  &__item-meta {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }

  &__chip {
    background: var(--bg-color-secondary, #f6f6f6);
    color: var(--text-color-secondary, #555);
    border-radius: 10px;
    padding: 1px 8px;
    font-size: 11px;

    &--lock {
      background: var(--warning-color-light, #fff7e6);
      color: var(--warning-color, #d27a00);
    }
  }
}
</style>