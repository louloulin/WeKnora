<template>
  <div class="wiki-public-share" :class="{ 'is-locked': needsPassword }">
    <header class="wiki-public-share__topbar">
      <div class="wiki-public-share__brand">
        <span class="wiki-public-share__brand-mark">WeKnora</span>
        <span class="wiki-public-share__brand-sub">{{ subtitle }}</span>
      </div>
      <span v-if="watermarkText" class="wiki-public-share__brand-watermark">
        {{ watermarkText }}
      </span>
    </header>

    <main class="wiki-public-share__body">
      <div v-if="loading" class="wiki-public-share__loading">
        <TSkeleton :row="5" />
      </div>

      <div v-else-if="errorMessage" class="wiki-public-share__error">
        <TIcon name="error-circle-filled" size="40px" />
        <h3>{{ errorTitle }}</h3>
        <p>{{ errorMessage }}</p>
      </div>

      <template v-else-if="needsPassword">
        <div class="wiki-public-share__password">
          <h2>{{ $t('wiki.share.passwordTitle') }}</h2>
          <p class="wiki-public-share__password-hint">
            {{ $t('wiki.share.passwordHint') }}
          </p>
          <TInput
            v-model="passwordInput"
            type="password"
            :placeholder="$t('wiki.share.passwordPlaceholder')"
            @enter="submitPassword"
          />
          <TButton theme="primary" :loading="unlocking" @click="submitPassword">
            {{ $t('wiki.share.passwordUnlock') }}
          </TButton>
        </div>
      </template>

      <article v-else-if="page" class="wiki-public-share__article">
        <h1 class="wiki-public-share__title">{{ page.title }}</h1>
        <p v-if="page.summary" class="wiki-public-share__summary">
          {{ page.summary }}
        </p>
        <div
          class="wiki-public-share__content wiki-reader-content"
          v-html="page.contentHtml"
        />
        <footer class="wiki-public-share__footer">
          {{ footerText }}
        </footer>
      </article>
    </main>

    <!-- Watermark overlay (always rendered while content is visible).
         Repeats a diagonal stripe of token-derived text — opacity 6% keeps
         the page readable while making screenshots traceable. -->
    <div
      v-if="page && !needsPassword"
      class="wiki-public-share__watermark"
      :style="{ backgroundImage: watermarkBackground }"
      aria-hidden="true"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  Button as TButton,
  Icon as TIcon,
  Input as TInput,
  MessagePlugin,
  Skeleton as TSkeleton,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  type WikiSharePublicResponse,
  fetchPublicShare,
  unlockPublicShare,
} from '../../api/wiki/share'

const props = defineProps<{
  token: string
}>()

const { t, locale } = useI18n()

const loading = ref(true)
const unlocking = ref(false)
const data = ref<WikiSharePublicResponse | null>(null)
const needsPassword = ref(false)
const passwordInput = ref('')
const errorTitle = ref('')
const errorMessage = ref('')

const page = computed(() => data.value?.page ?? null)
const kb = computed(() => data.value?.kb ?? null)
const watermarkText = computed(() => data.value?.watermark ?? '')

const subtitle = computed(() => {
  if (!kb.value) return t('wiki.share.publicSubtitle')
  return t('wiki.share.publicSubtitleFor', { kb: kb.value.name })
})

const footerText = computed(() => {
  if (!page.value) return ''
  const updated = page.value.updatedAt
  try {
    const when = new Date(updated).toLocaleString(locale.value, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
    return t('wiki.share.publicFooter', { when, type: page.value.pageType })
  } catch {
    return t('wiki.share.publicFooterRaw', { when: updated, type: page.value.pageType })
  }
})

// Diagonal watermark: repeat the truncated token (≤ 12 chars) plus viewer
// info at low opacity. CSS escapes ensure even if the token contains '@'
// the gradient parser stays valid.
const watermarkBackground = computed(() => {
  const text = (watermarkText.value || props.token || '').slice(0, 16)
  const safe = text.replace(/[^a-zA-Z0-9_-]/g, '')
  if (!safe) return 'none'
  const encoded = encodeURIComponent(safe)
  return `repeating-linear-gradient(
    -30deg,
    transparent 0,
    transparent 220px,
    rgba(0, 0, 0, 0.06) 220px,
    rgba(0, 0, 0, 0.06) 320px
  ), url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='320' height='220'><text x='10' y='110' transform='rotate(-30 10 110)' font-size='14' fill='rgba(0,0,0,0.06)' font-family='ui-monospace,monospace'>${encoded}</text></svg>")`
})

async function load(): Promise<void> {
  loading.value = true
  errorTitle.value = ''
  errorMessage.value = ''
  needsPassword.value = false
  try {
    const res = await fetchPublicShare(props.token)
    data.value = res.data ?? null
  } catch (err) {
    const status = extractStatus(err)
    if (status === 401) {
      needsPassword.value = true
      errorMessage.value = ''
    } else if (status === 410) {
      errorTitle.value = t('wiki.share.errorRevokedTitle')
      errorMessage.value = t('wiki.share.errorRevokedBody')
    } else if (status === 404) {
      errorTitle.value = t('wiki.share.errorNotFoundTitle')
      errorMessage.value = t('wiki.share.errorNotFoundBody')
    } else {
      errorTitle.value = t('wiki.share.errorLoadTitle')
      errorMessage.value = messageOf(err) || t('wiki.share.errorLoadBody')
    }
  } finally {
    loading.value = false
  }
}

async function submitPassword(): Promise<void> {
  if (!passwordInput.value.trim()) return
  unlocking.value = true
  try {
    const res = await unlockPublicShare(props.token, passwordInput.value)
    data.value = res.data ?? null
    needsPassword.value = false
    passwordInput.value = ''
  } catch (err) {
    const status = extractStatus(err)
    if (status === 401 || status === 403) {
      MessagePlugin.error(t('wiki.share.passwordWrong'))
    } else {
      MessagePlugin.error(messageOf(err) || t('wiki.share.passwordUnlockFailed'))
    }
  } finally {
    unlocking.value = false
  }
}

function messageOf(err: unknown): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string') return m
  }
  return ''
}

function extractStatus(err: unknown): number | null {
  if (!err || typeof err !== 'object') return null
  const obj = err as Record<string, unknown>
  const response = obj.response as { status?: unknown } | undefined
  if (response && typeof response.status === 'number') return response.status
  const status = obj.status
  if (typeof status === 'number') return status
  // Fallback: scan the message text for HTTP codes.
  const m = messageOf(err)
  const match = /\b(401|403|404|410)\b/.exec(m)
  return match ? Number(match[1]) : null
}

onMounted(load)
watch(() => props.token, load)
</script>

<style lang="less">
.wiki-public-share {
  position: relative;
  min-height: 100vh;
  background: var(--bg-color-page, #f5f5f5);
  color: var(--text-color-primary, #181818);
  font-family: var(--font-family, system-ui, -apple-system, sans-serif);
  overflow: hidden;

  &__topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 24px;
    background: var(--bg-color-container, #fff);
    border-bottom: 1px solid var(--component-border, #dcdfe6);
    position: relative;
    z-index: 2;
  }

  &__brand {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }

  &__brand-mark {
    font-weight: 700;
    font-size: 18px;
  }

  &__brand-sub {
    font-size: 12px;
    color: var(--text-color-secondary, #555);
  }

  &__brand-watermark {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 11px;
    color: var(--text-color-placeholder, #999);
    max-width: 40%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__body {
    max-width: 760px;
    margin: 32px auto;
    padding: 32px 36px;
    background: var(--bg-color-container, #fff);
    border-radius: 12px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.06);
    position: relative;
    z-index: 1;
  }

  &__loading,
  &__error {
    padding: 40px 16px;
    text-align: center;
    color: var(--text-color-secondary, #555);
  }

  &__error {
    h3 {
      margin: 12px 0 6px;
      font-size: 18px;
    }
    p {
      margin: 0;
      font-size: 13px;
      color: var(--text-color-placeholder, #999);
    }
  }

  &__password {
    text-align: center;
    display: flex;
    flex-direction: column;
    gap: 10px;
    align-items: stretch;
    max-width: 320px;
    margin: 0 auto;

    h2 {
      margin: 0;
      font-size: 18px;
    }
  }

  &__password-hint {
    font-size: 13px;
    color: var(--text-color-secondary, #555);
    margin: 0;
  }

  &__title {
    font-size: 28px;
    margin: 0 0 12px;
    line-height: 1.25;
  }

  &__summary {
    color: var(--text-color-secondary, #555);
    font-size: 14px;
    margin: 0 0 24px;
    border-left: 3px solid var(--brand-color, #0052d9);
    padding-left: 12px;
  }

  &__content {
    font-size: 15px;
    line-height: 1.7;

    img {
      max-width: 100%;
    }

    pre,
    code {
      background: var(--bg-color-secondary, #f6f6f6);
      border-radius: 4px;
    }
  }

  &__footer {
    margin-top: 32px;
    padding-top: 16px;
    border-top: 1px dashed var(--component-border, #dcdfe6);
    color: var(--text-color-placeholder, #999);
    font-size: 12px;
  }

  // Watermark sits behind content but above background, tiled diagonally.
  // Pointer events disabled so it never blocks selection / scroll.
  &__watermark {
    position: fixed;
    inset: 0;
    pointer-events: none;
    z-index: 0;
    opacity: 1;
    mix-blend-mode: multiply;
  }
}

.wiki-public-share.is-locked .wiki-public-share__watermark {
  display: none;
}
</style>