<!--
  WikiPresenceBar — v0.7.19 collaborator avatars + connection indicator.

  Shows:
    • Connection status dot (green = online, yellow = connecting, red = offline)
    • Up to 5 peer avatars (with overflow +N chip)
    • Tooltip with peer display name on hover

  Emits no events; the parent component owns the useYjsWiki composable.
-->
<template>
  <div class="wiki-presence-bar" :class="{ 'is-empty': !peers.length }">
    <span class="wiki-presence-dot" :class="connectionClass" :title="connectionTitle" />
    <span v-if="peers.length" class="wiki-presence-peers">
      <span
        v-for="peer in visiblePeers"
        :key="peer.clientId"
        class="wiki-presence-avatar"
        :style="{ backgroundColor: peer.color }"
        :title="peer.displayName"
      >{{ initials(peer.displayName) }}</span>
      <span v-if="overflowCount > 0" class="wiki-presence-overflow">
        +{{ overflowCount }}
      </span>
    </span>
    <span v-else class="wiki-presence-empty">Solo</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Peer {
  clientId: number
  displayName: string
  color: string
}

const props = withDefaults(defineProps<{
  peers: Peer[]
  connected: boolean
  error?: string | null
  max?: number
}>(), {
  max: 5,
  error: null,
})

const visiblePeers = computed(() => props.peers.slice(0, props.max))
const overflowCount = computed(() => Math.max(0, props.peers.length - props.max))

const connectionClass = computed(() => {
  if (props.error) return 'is-error'
  if (props.connected) return 'is-connected'
  return 'is-connecting'
})

const connectionTitle = computed(() => {
  if (props.error) return `Realtime error: ${props.error}`
  if (props.connected) return 'Connected — realtime sync on'
  return 'Connecting…'
})

function initials(name: string): string {
  if (!name) return '?'
  const parts = name.trim().split(/\s+/)
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}
</script>

<style scoped>
.wiki-presence-bar {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--color-bg-elevated, #161b22);
  border: 1px solid var(--color-border, #30363d);
  font-size: 12px;
  color: var(--color-text-muted, #9da7b3);
}
.wiki-presence-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-text-muted, #9da7b3);
}
.wiki-presence-dot.is-connected { background: #3fb950; }
.wiki-presence-dot.is-connecting { background: #d29922; animation: pulse 1.5s infinite; }
.wiki-presence-dot.is-error { background: #f85149; }
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
.wiki-presence-peers {
  display: inline-flex;
  align-items: center;
}
.wiki-presence-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  margin-left: -4px;
  border: 2px solid var(--color-bg-elevated, #161b22);
}
.wiki-presence-avatar:first-child { margin-left: 0; }
.wiki-presence-overflow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--color-text-muted, #9da7b3);
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  margin-left: -4px;
  border: 2px solid var(--color-bg-elevated, #161b22);
}
.wiki-presence-empty { font-style: italic; }
</style>
