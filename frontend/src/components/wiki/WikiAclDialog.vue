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
    <div class="wiki-acl-dialog">
      <div v-if="loading" class="wiki-acl-dialog__loading">
        <TSkeleton :row="3" />
      </div>

      <template v-else>
        <div class="wiki-acl-dialog__section">
          <div class="wiki-acl-dialog__section-title">
            {{ t('wiki.acl.modeTitle') }}
          </div>
          <div class="wiki-acl-dialog__modes">
            <label
              v-for="opt in modeOptions"
              :key="opt.value"
              class="wiki-acl-dialog__mode"
              :class="{ 'is-active': draft.mode === opt.value }"
            >
              <input
                v-model="draft.mode"
                type="radio"
                :value="opt.value"
                name="wiki-acl-mode"
              />
              <span class="wiki-acl-dialog__mode-label">
                {{ t(opt.labelKey) }}
              </span>
              <span class="wiki-acl-dialog__mode-hint">
                {{ t(opt.hintKey) }}
              </span>
            </label>
          </div>
        </div>

        <div v-if="draft.mode === 'allow_list'" class="wiki-acl-dialog__section">
          <div class="wiki-acl-dialog__section-title">
            {{ t('wiki.acl.allowListTitle') }}
          </div>
          <div class="wiki-acl-dialog__search">
            <TInput
              v-model="searchInput"
              :placeholder="t('wiki.acl.searchPlaceholder')"
              @input="onSearchInput"
            />
            <div
              v-if="store.candidateOpen"
              class="wiki-acl-dialog__candidates"
            >
              <div
                v-if="store.candidateLoading"
                class="wiki-acl-dialog__candidates-loading"
              >
                {{ t('wiki.acl.searching') }}
              </div>
              <ul v-else>
                <li
                  v-for="cand in store.candidateList"
                  :key="cand.userId"
                  class="wiki-acl-dialog__candidate"
                  @mousedown.prevent="addCandidate(cand)"
                >
                  <span class="wiki-acl-dialog__candidate-name">
                    {{ cand.displayName }}
                  </span>
                  <span v-if="cand.email" class="wiki-acl-dialog__candidate-email">
                    {{ cand.email }}
                  </span>
                </li>
                <li
                  v-if="!store.candidateList.length"
                  class="wiki-acl-dialog__candidate is-empty"
                >
                  {{ t('wiki.acl.searchEmpty') }}
                </li>
              </ul>
            </div>
          </div>

          <ul v-if="selectedUsers.length" class="wiki-acl-dialog__chips">
            <li
              v-for="user in selectedUsers"
              :key="user.userId"
              class="wiki-acl-dialog__chip"
            >
              <span>{{ user.displayName }}</span>
              <button
                type="button"
                class="wiki-acl-dialog__chip-remove"
                :aria-label="t('wiki.acl.removeAria', { name: user.displayName })"
                @click="removeUser(user.userId)"
              >
                ×
              </button>
            </li>
          </ul>
          <div v-else class="wiki-acl-dialog__chips-empty">
            {{ t('wiki.acl.allowListEmpty') }}
          </div>

          <label class="wiki-acl-dialog__checkbox">
            <input v-model="draft.denyInherited" type="checkbox" />
            <span>{{ t('wiki.acl.denyInherited') }}</span>
          </label>
        </div>

        <div class="wiki-acl-dialog__section wiki-acl-dialog__actions">
          <span v-if="updatedAtText" class="wiki-acl-dialog__updated">
            {{ t('wiki.acl.updatedAt', { time: updatedAtText }) }}
          </span>
          <TButton variant="text" @click="visible = false">
            {{ t('common.cancel') }}
          </TButton>
          <TButton
            theme="primary"
            :loading="saving"
            :disabled="!isDirty"
            @click="submit"
          >
            {{ t('common.save') }}
          </TButton>
        </div>
      </template>
    </div>
  </TDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import {
  Button as TButton,
  Dialog as TDialog,
  Input as TInput,
  MessagePlugin,
  Skeleton as TSkeleton,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { onAclEvent, useWikiPageAclStore } from '../../stores/wikiPageAcl'
import {
  type WikiAclMode,
  type WikiPageAcl,
  type WikiUserCandidate,
  defaultWikiPageAcl,
} from '../../api/wiki/acl'
import {
  formatAclUpdatedAt,
  isAclDraftDirty,
} from './wikiAclDialogLogic'

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
const store = useWikiPageAclStore()

const visible = computed({
  get: () => props.visible,
  set: (val: boolean) => emit('update:visible', val),
})

const headerTitle = computed(() =>
  props.pageTitle
    ? t('wiki.acl.headerFor', { title: props.pageTitle })
    : t('wiki.acl.header'),
)

const modeOptions: { value: WikiAclMode; labelKey: string; hintKey: string }[] = [
  {
    value: 'inherit',
    labelKey: 'wiki.acl.mode.inherit',
    hintKey: 'wiki.acl.mode.inheritHint',
  },
  {
    value: 'private',
    labelKey: 'wiki.acl.mode.private',
    hintKey: 'wiki.acl.mode.privateHint',
  },
  {
    value: 'allow_list',
    labelKey: 'wiki.acl.mode.allowList',
    hintKey: 'wiki.acl.mode.allowListHint',
  },
]

const draft = reactive<WikiPageAcl>(defaultWikiPageAcl())
const selectedUsers = ref<WikiUserCandidate[]>([])
const searchInput = ref('')
const saving = ref(false)

const loading = computed(() => store.isLoading(props.kbId, props.slug))

const isDirty = computed(() => {
  const original = store.aclFor(props.kbId, props.slug)
  return isAclDraftDirty(draft, original)
})

const updatedAtText = computed(() => {
  const src = store.aclFor(props.kbId, props.slug)
  return formatAclUpdatedAt(src.updatedAt, locale.value)
})

async function onOpen(): Promise<void> {
  await store.fetchAcl(props.kbId, props.slug)
  resetDraft()
}

function onClose(): void {
  store.resetCandidates()
  searchInput.value = ''
}

watch(
  () => props.slug,
  async (next, prev) => {
    if (next && next !== prev) {
      await store.fetchAcl(props.kbId, next)
      resetDraft()
    }
  },
)

// Out-of-band ACL updates (another admin's save via 409, or a future
// websocket push) should drop the user's local draft — server wins.
const unsubscribeAclEvents = onAclEvent((event) => {
  if (event.kind !== 'conflict' && event.kind !== 'updated') return
  if (event.kbId !== props.kbId || event.slug !== props.slug) return
  resetDraft()
  if (event.kind === 'conflict') {
    MessagePlugin.warning(t('wiki.acl.error.conflict'))
  }
})
onBeforeUnmount(() => {
  unsubscribeAclEvents()
})

function resetDraft(): void {
  const src = store.aclFor(props.kbId, props.slug)
  draft.mode = src.mode
  draft.allowUserIds = [...src.allowUserIds]
  draft.allowGroupIds = [...src.allowGroupIds]
  draft.denyInherited = src.denyInherited
  draft.revision = src.revision
  draft.updatedAt = src.updatedAt
  // Clear any chip display; backend fetch will supply display names when
  // we wire `/api/v1/users/lookup?ids=...`. Until then the chip list
  // starts empty when opening — saving will persist the IDs alone.
  selectedUsers.value = []
}

let searchTimer: number | undefined
function onSearchInput(): void {
  if (searchTimer) window.clearTimeout(searchTimer)
  const q = searchInput.value.trim()
  if (!q) {
    store.resetCandidates()
    return
  }
  searchTimer = window.setTimeout(() => {
    store.searchCandidates(q)
  }, 200)
}

function addCandidate(cand: WikiUserCandidate): void {
  if (draft.allowUserIds.includes(cand.userId)) {
    store.resetCandidates()
    searchInput.value = ''
    return
  }
  draft.allowUserIds.push(cand.userId)
  selectedUsers.value = [...selectedUsers.value, cand]
  store.resetCandidates()
  searchInput.value = ''
}

function removeUser(userId: string): void {
  draft.allowUserIds = draft.allowUserIds.filter((id) => id !== userId)
  selectedUsers.value = selectedUsers.value.filter((u) => u.userId !== userId)
}

async function submit(): Promise<void> {
  if (!isDirty.value || saving.value) return
  saving.value = true
  try {
    const result = await store.saveAcl(props.kbId, props.slug, {
      mode: draft.mode,
      allowUserIds: draft.allowUserIds,
      allowGroupIds: draft.allowGroupIds,
      denyInherited: draft.denyInherited,
      baseRevision: draft.revision,
    })
    if (result.ok) {
      MessagePlugin.success(t('wiki.acl.saveSuccess'))
      visible.value = false
      return
    }
    if (result.conflict) {
      // Store has already adopted the canonical ACL; resetDraft() runs
      // via the conflict event listener. Toast + close so the user can
      // re-open with the fresh state.
      MessagePlugin.warning(t('wiki.acl.error.conflict'))
      visible.value = false
      return
    }
    const msg = result.error || ''
    if (msg === 'acl.denied') {
      MessagePlugin.error(t('wiki.acl.error.denied'))
    } else if (msg === 'acl.network') {
      MessagePlugin.error(t('wiki.acl.error.network'))
    } else {
      MessagePlugin.error(t('wiki.acl.error.generic'))
    }
  } finally {
    saving.value = false
  }
}
</script>

<style scoped lang="less">
.wiki-acl-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;

  &__loading,
  &__chips-empty {
    padding: 16px 8px;
    text-align: center;
    color: var(--text-color-placeholder, #999);
    font-size: 13px;
  }

  &__section {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  &__section-title {
    font-weight: 600;
    font-size: 13px;
    color: var(--text-color-secondary, #555);
  }

  &__modes {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  &__mode {
    display: grid;
    grid-template-columns: 18px 1fr;
    grid-template-rows: auto auto;
    column-gap: 8px;
    padding: 8px 10px;
    border: 1px solid var(--component-border, #dcdfe6);
    border-radius: 6px;
    cursor: pointer;
    transition: border-color 0.15s ease;

    &.is-active {
      border-color: var(--brand-color, #0052d9);
      background: var(--brand-color-light, #e6f0ff);
    }

    input {
      grid-row: 1 / span 2;
      align-self: start;
      margin-top: 3px;
    }
  }

  &__mode-label {
    font-weight: 500;
  }

  &__mode-hint {
    grid-column: 2;
    font-size: 12px;
    color: var(--text-color-placeholder, #999);
  }

  &__search {
    position: relative;
  }

  &__candidates {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    max-height: 220px;
    overflow-y: auto;
    background: var(--bg-color-container, #fff);
    border: 1px solid var(--component-border, #dcdfe6);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
    z-index: 10;

    ul {
      list-style: none;
      margin: 0;
      padding: 4px 0;
    }
  }

  &__candidates-loading {
    padding: 8px 12px;
    color: var(--text-color-placeholder, #999);
    font-size: 13px;
  }

  &__candidate {
    padding: 6px 12px;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 8px;

    &:hover {
      background: var(--brand-color-light, #e6f0ff);
    }

    &.is-empty {
      color: var(--text-color-placeholder, #999);
      cursor: default;
    }
  }

  &__candidate-email {
    font-size: 12px;
    color: var(--text-color-placeholder, #999);
  }

  &__chips {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  &__chip {
    background: var(--brand-color-light, #e6f0ff);
    color: var(--brand-color, #0052d9);
    border-radius: 12px;
    padding: 2px 10px;
    font-size: 12px;
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  &__chip-remove {
    border: 0;
    background: transparent;
    color: inherit;
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
    padding: 0 2px;
    border-radius: 50%;

    &:hover {
      background: rgba(0, 82, 217, 0.18);
    }
  }

  &__checkbox {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--text-color-secondary, #555);
    margin-top: 4px;
  }

  &__actions {
    flex-direction: row;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 8px;
    border-top: 1px solid var(--component-border, #dcdfe6);
    padding-top: 12px;
  }

  &__updated {
    flex: 1;
    align-self: center;
    color: var(--text-color-placeholder, #999);
    font-size: 12px;
  }
}
</style>