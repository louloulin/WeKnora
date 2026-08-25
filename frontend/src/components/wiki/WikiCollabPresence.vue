<template>
  <div class="wiki-collab-presence" :class="`is-${status}`">
    <div class="wiki-collab-presence__status" :title="statusTitle">
      <span class="wiki-collab-presence__dot" />
      <span class="wiki-collab-presence__status-text">{{ statusText }}</span>
    </div>
    <div v-if="peerList.length" class="wiki-collab-presence__peers">
      <span
        v-for="peer in peerList"
        :key="peer.clientId"
        class="wiki-collab-presence__peer"
        :style="{ backgroundColor: peer.color }"
        :title="peer.displayName"
      >
        {{ initials(peer.displayName) }}
      </span>
      <span v-if="peerList.length > 6" class="wiki-collab-presence__more">
        +{{ peerList.length - 6 }}
      </span>
    </div>
    <button
      v-if="status !== 'connected'"
      type="button"
      class="wiki-collab-presence__reconnect"
      @click="emit('reconnect')"
    >
      {{ t('wiki.collab.reconnect') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WikiCollabStatus } from '../../stores/wikiCollab'

const props = defineProps<{
  status: WikiCollabStatus
  peerList: { clientId: number; displayName: string; color: string }[]
  selfName?: string
}>()

const emit = defineEmits<{
  (e: 'reconnect'): void
}>()

const { t } = useI18n()

const statusText = computed(() => {
  switch (props.status) {
    case 'connecting':
      return t('wiki.collab.status.connecting')
    case 'connected':
      return t('wiki.collab.status.connected')
    case 'reconnecting':
      return t('wiki.collab.status.reconnecting')
    case 'error':
      return t('wiki.collab.status.error')
    default:
      return t('wiki.collab.status.off')
  }
})

const statusTitle = computed(() => {
  if (props.selfName) return `${statusText.value} · ${props.selfName}`
  return statusText.value
})

function initials(name: string): string {
  if (!name) return '?'
  const trimmed = name.trim()
  if (/[一-龥]/.test(trimmed)) return trimmed.slice(-2)
  const parts = trimmed.split(/\s+/)
  return (parts[0]?.[0] ?? '?') + (parts[1]?.[0] ?? '')
}
</script>

<style scoped lang="less">
.wiki-collab-presence {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border-radius: 12px;
  background: var(--bg-color-secondary, #f6f6f6);
  border: 1px solid var(--component-border, #dcdfe6);
  font-size: 12px;
  color: var(--text-color-secondary, #555);

  &.is-connected &__dot {
    background: var(--td-success-color, #2ba471);
  }

  &.is-connecting &__dot,
  &.is-reconnecting &__dot {
    background: var(--td-warning-color, #e37318);
    animation: wiki-collab-presence-pulse 1.2s ease-in-out infinite;
  }

  &.is-error &__dot {
    background: var(--td-error-color, #d54941);
  }

  &.is-idle &__dot {
    background: var(--td-text-color-placeholder, #c5c5c5);
  }

  &__status {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  &__dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--td-text-color-placeholder, #c5c5c5);
    flex-shrink: 0;
  }

  &__peers {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  &__peer {
    width: 22px;
    height: 22px;
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: #fff;
    font-weight: 600;
    font-size: 10px;
    border: 2px solid var(--bg-color-container, #fff);
    margin-left: -4px;

    &:first-child {
      margin-left: 0;
    }
  }

  &__more {
    font-size: 11px;
    color: var(--text-color-secondary, #555);
    margin-left: 2px;
  }

  &__reconnect {
    border: 0;
    background: transparent;
    color: var(--brand-color, #0052d9);
    cursor: pointer;
    font-size: 12px;
    padding: 0 4px;

    &:hover {
      text-decoration: underline;
    }
  }
}

@keyframes wiki-collab-presence-pulse {
  0%, 100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.3);
    opacity: 0.6;
  }
}
</style>