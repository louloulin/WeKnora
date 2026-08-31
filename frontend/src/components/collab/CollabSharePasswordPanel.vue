<!--
  CollabSharePasswordPanel — v0.7.41 Build #47.

  Lets a doc owner turn the share link on / off and (optionally) gate
  it with a password + expiry. The backend lives at
  POST/DELETE /collaborative-docs/:id/share; the response shape is
  { share_token, expires_at, protected, url }.

  Design notes
  ------------
  * "Open link" mode (no password) preserves the legacy behaviour.
  * Password must be at least 6 characters — the handler rejects
    shorter values with 400, and we mirror that here so users see
    a clear inline error before the round-trip.
  * Expiry is optional and shown in the tenant's locale. Empty means
    "no expiry" (the link lives forever).
  * The "Copy link" button copies the absolute URL plus the password
    to clipboard when protected, so a user can paste it straight into
    Slack. When not protected, only the URL is copied.
-->
<template>
  <div class="share-panel" data-testid="share-panel">
    <div class="share-panel__header">
      <h3>{{ t('knowledgeBase.collabDoc.share.title') }}</h3>
      <span
        v-if="isProtected"
        class="share-panel__badge share-panel__badge--locked"
        data-testid="share-protected-badge"
      >
        🔒 {{ t('knowledgeBase.collabDoc.share.protected') }}
      </span>
      <span
        v-else-if="shareToken"
        class="share-panel__badge share-panel__badge--open"
        data-testid="share-open-badge"
      >
        🔗 {{ t('knowledgeBase.collabDoc.share.open') }}
      </span>
    </div>

    <p class="share-panel__hint">
      {{ t('knowledgeBase.collabDoc.share.hint') }}
    </p>

    <template v-if="shareToken">
      <div class="share-panel__row">
        <input
          class="share-panel__url"
          :value="shareUrl"
          readonly
          data-testid="share-url-input"
          @focus="onFocusUrl"
        />
        <button
          class="share-panel__btn"
          data-testid="share-copy-btn"
          @click="copyLink"
        >
          {{ copied ? t('knowledgeBase.collabDoc.share.copied') : t('knowledgeBase.collabDoc.share.copy') }}
        </button>
      </div>

      <div class="share-panel__meta">
        <span v-if="expiresAt" class="share-panel__meta-item">
          {{ t('knowledgeBase.collabDoc.share.expiresAt') }}:
          <strong>{{ formattedExpiry }}</strong>
        </span>
        <span v-else class="share-panel__meta-item">
          {{ t('knowledgeBase.collabDoc.share.neverExpires') }}
        </span>
      </div>

      <details class="share-panel__advanced">
        <summary>{{ t('knowledgeBase.collabDoc.share.advanced') }}</summary>
        <div class="share-panel__form">
          <label class="share-panel__label">
            {{ t('knowledgeBase.collabDoc.share.passwordLabel') }}
            <input
              v-model="password"
              type="password"
              minlength="6"
              autocomplete="new-password"
              data-testid="share-password-input"
            />
            <small v-if="hasPasswordTooShort" class="share-panel__error">
              {{ t('knowledgeBase.collabDoc.share.passwordTooShort') }}
            </small>
          </label>
          <label class="share-panel__label">
            {{ t('knowledgeBase.collabDoc.share.expiresLabel') }}
            <input
              v-model="expiresAtInput"
              type="datetime-local"
              data-testid="share-expiry-input"
            />
            <small class="share-panel__hint-small">
              {{ t('knowledgeBase.collabDoc.share.expiresHint') }}
            </small>
          </label>
          <button
            class="share-panel__btn share-panel__btn--primary"
            :disabled="submitting || hasPasswordTooShort"
            data-testid="share-refresh-btn"
            @click="onRefresh"
          >
            {{ t('knowledgeBase.collabDoc.share.refresh') }}
          </button>
        </div>
      </details>

      <button
        class="share-panel__btn share-panel__btn--danger"
        :disabled="submitting"
        data-testid="share-disable-btn"
        @click="onDisable"
      >
        {{ t('knowledgeBase.collabDoc.share.disable') }}
      </button>
    </template>

    <template v-else>
      <div class="share-panel__form">
        <label class="share-panel__label">
          {{ t('knowledgeBase.collabDoc.share.passwordLabel') }}
          <input
            v-model="password"
            type="password"
            minlength="6"
            autocomplete="new-password"
            :placeholder="t('knowledgeBase.collabDoc.share.passwordPlaceholder')"
            data-testid="share-password-input"
          />
          <small v-if="hasPasswordTooShort" class="share-panel__error">
            {{ t('knowledgeBase.collabDoc.share.passwordTooShort') }}
          </small>
        </label>
        <label class="share-panel__label">
          {{ t('knowledgeBase.collabDoc.share.expiresLabel') }}
          <input
            v-model="expiresAtInput"
            type="datetime-local"
            data-testid="share-expiry-input"
          />
          <small class="share-panel__hint-small">
            {{ t('knowledgeBase.collabDoc.share.expiresHint') }}
          </small>
        </label>
        <button
          class="share-panel__btn share-panel__btn--primary"
          :disabled="submitting || hasPasswordTooShort"
          data-testid="share-enable-btn"
          @click="onEnable"
        >
          {{ submitting
              ? t('knowledgeBase.collabDoc.share.enabling')
              : t('knowledgeBase.collabDoc.share.enable') }}
        </button>
      </div>
    </template>

    <p v-if="errorMessage" class="share-panel__error" data-testid="share-error">
      {{ errorMessage }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  enableCollabDocShare,
  disableCollabDocShare,
  type EnableShareResponse,
} from '@/api/collabDoc'

const props = defineProps<{
  docId: string
  initialShareToken: string
  initialProtected: boolean
  initialExpiresAt: string | null
}>()

const emit = defineEmits<{
  (e: 'updated', payload: EnableShareResponse): void
  (e: 'disabled'): void
}>()

const { t } = useI18n()

const shareToken = ref(props.initialShareToken || '')
const isProtected = ref(props.initialProtected || false)
const expiresAt = ref<string | null>(props.initialExpiresAt || null)
const password = ref('')
const expiresAtInput = ref<string>('')
const submitting = ref(false)
const errorMessage = ref('')
const copied = ref(false)

watch(
  () => [props.initialShareToken, props.initialProtected, props.initialExpiresAt],
  ([tok, p, e]) => {
    shareToken.value = (tok as string) || ''
    isProtected.value = (p as boolean) || false
    expiresAt.value = (e as string) || null
  },
)

const shareUrl = computed(() => {
  if (!shareToken.value) return ''
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  return `${origin}/collab-share/${encodeURIComponent(shareToken.value)}`
})

const formattedExpiry = computed(() => {
  if (!expiresAt.value) return ''
  try {
    return new Date(expiresAt.value).toLocaleString()
  } catch {
    return expiresAt.value
  }
})

const hasPasswordTooShort = computed(
  () => password.value.length > 0 && password.value.length < 6,
)

function onFocusUrl(event: FocusEvent) {
  const target = event.target as HTMLInputElement
  target.select()
}

async function callEnable() {
  if (hasPasswordTooShort.value) {
    errorMessage.value = t('knowledgeBase.collabDoc.share.passwordTooShort')
    return
  }
  submitting.value = true
  errorMessage.value = ''
  try {
    const resp = await enableCollabDocShare(props.docId, {
      password: password.value,
      expires_at: expiresAtInput.value
        ? new Date(expiresAtInput.value).toISOString()
        : null,
    })
    shareToken.value = resp.share_token
    isProtected.value = resp.protected
    expiresAt.value = resp.expires_at
    password.value = ''
    emit('updated', resp)
  } catch (e) {
    errorMessage.value = (e as Error).message || t('knowledgeBase.collabDoc.share.error')
  } finally {
    submitting.value = false
  }
}

async function onEnable() {
  if (submitting.value) return
  await callEnable()
}

async function onRefresh() {
  if (submitting.value) return
  await callEnable()
}

async function onDisable() {
  if (submitting.value) return
  submitting.value = true
  errorMessage.value = ''
  try {
    await disableCollabDocShare(props.docId)
    shareToken.value = ''
    isProtected.value = false
    expiresAt.value = null
    password.value = ''
    expiresAtInput.value = ''
    emit('disabled')
  } catch (e) {
    errorMessage.value = (e as Error).message || t('knowledgeBase.collabDoc.share.error')
  } finally {
    submitting.value = false
  }
}

async function copyLink() {
  if (!shareUrl.value) return
  try {
    let payload = shareUrl.value
    if (isProtected.value && password.value) {
      payload += `\n${t('knowledgeBase.collabDoc.share.passwordLabel')}: ${password.value}`
    } else if (isProtected.value) {
      payload += `\n${t('knowledgeBase.collabDoc.share.passwordLabel')}: <unchanged>`
    }
    await navigator.clipboard.writeText(payload)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    errorMessage.value = t('knowledgeBase.collabDoc.share.copyFailed')
  }
}
</script>

<style scoped>
.share-panel {
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 8px;
  padding: 16px;
  background: var(--surface, #fff);
  color: var(--text, #1f2937);
}
.share-panel__header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.share-panel__header h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.share-panel__badge {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 12px;
}
.share-panel__badge--locked {
  background: rgba(245, 158, 11, 0.12);
  color: #b45309;
}
.share-panel__badge--open {
  background: rgba(16, 185, 129, 0.12);
  color: #047857;
}
.share-panel__hint {
  margin: 0 0 12px;
  color: var(--text-muted, #6b7280);
  font-size: 13px;
}
.share-panel__row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
.share-panel__url {
  flex: 1;
  font-family: ui-monospace, monospace;
  font-size: 12px;
  padding: 6px 8px;
  border: 1px solid var(--border-color, #d1d5db);
  border-radius: 6px;
  background: var(--surface-soft, #f9fafb);
}
.share-panel__meta {
  font-size: 12px;
  color: var(--text-muted, #6b7280);
  margin-bottom: 8px;
}
.share-panel__advanced {
  margin: 8px 0;
  font-size: 13px;
}
.share-panel__form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 10px;
}
.share-panel__label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--text-muted, #4b5563);
}
.share-panel__label input {
  padding: 6px 8px;
  border: 1px solid var(--border-color, #d1d5db);
  border-radius: 6px;
  font-size: 13px;
}
.share-panel__hint-small {
  color: var(--text-muted, #9ca3af);
  font-size: 11px;
}
.share-panel__btn {
  padding: 6px 12px;
  border: 1px solid var(--border-color, #d1d5db);
  border-radius: 6px;
  background: var(--surface, #fff);
  font-size: 13px;
  cursor: pointer;
}
.share-panel__btn:hover {
  background: var(--hover, #f3f4f6);
}
.share-panel__btn--primary {
  background: var(--accent, #2563eb);
  border-color: var(--accent, #2563eb);
  color: #fff;
}
.share-panel__btn--primary:hover {
  background: var(--accent-dark, #1d4ed8);
}
.share-panel__btn--danger {
  border-color: #ef4444;
  color: #b91c1c;
  margin-top: 8px;
}
.share-panel__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.share-panel__error {
  color: #b91c1c;
  font-size: 12px;
  margin: 4px 0 0;
}
</style>
