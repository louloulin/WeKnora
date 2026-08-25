import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

/**
 * Wiki real-time collaboration session store (Build #8).
 *
 * Tracks WebSocket connection status, peer roster, and remote cursor
 * positions for the currently edited page. The actual y-doc / provider
 * lifecycle lives in `composables/useWikiCollab.ts`; this store only
 * surfaces observable state to UI components.
 *
 * Status semantics:
 *   - idle:        no session open (default mount state)
 *   - connecting:  WebSocket opening / Yjs sync handshake in progress
 *   - connected:   synced; peer list + cursor positions active
 *   - reconnecting:WS dropped; backoff loop running
 *   - error:       terminal failure (auth, 404, etc.)
 */

export type WikiCollabStatus = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error'

export interface WikiCollabPeer {
  /** Yjs clientID (uint32 as string for serialization). */
  clientId: number
  userId: string
  displayName: string
  /** Hex color used for cursor + selection highlight. */
  color: string
  /** Last awareness update (ISO); used to evict stale peers. */
  lastSeen: string
}

/**
 * Recent collaborator entry (Build #8.1).
 *
 * Unlike WikiCollabPeer (which tracks a live y-websocket awareness
 * state), `WikiCollabRecentPeer` represents a user who was active in
 * this room recently but is no longer connected. The presence stack
 * renders these as ghost avatars with a "5 minutes ago" tooltip.
 */
export interface WikiCollabRecentPeer {
  /** Server-assigned client identifier (hash of userID for now). */
  clientId: string
  userId: string
  displayName: string
  color: string
  /** ISO timestamp of the last awareness frame from this user. */
  lastSeenAt: string
}

export interface WikiCollabSessionMeta {
  kbId: string
  slug: string
  /** Resolved WebSocket URL `${origin}/api/v1/wiki/collab/:kbId/:slug`. */
  wsUrl: string
}

export const useWikiCollabStore = defineStore('wikiCollab', () => {
  const status = ref<WikiCollabStatus>('idle')
  const session = ref<WikiCollabSessionMeta | null>(null)
  const peers = ref<Record<number, WikiCollabPeer>>({})
  const recentPeers = ref<WikiCollabRecentPeer[]>([])
  const lastError = ref<string | null>(null)
  /** Increments on every connection attempt; lets UI reset transient state. */
  const attempt = ref(0)

  const peerCount = computed(() => Object.keys(peers.value).length)
  const peerList = computed(() => Object.values(peers.value))
  const isLive = computed(
    () => status.value === 'connected' || status.value === 'reconnecting',
  )

  function setStatus(next: WikiCollabStatus): void {
    status.value = next
  }

  function beginSession(meta: WikiCollabSessionMeta): void {
    session.value = meta
    peers.value = {}
    recentPeers.value = []
    lastError.value = null
    status.value = 'connecting'
    attempt.value += 1
  }

  function endSession(): void {
    session.value = null
    peers.value = {}
    recentPeers.value = []
    status.value = 'idle'
    lastError.value = null
  }

  function markConnected(): void {
    status.value = 'connected'
    lastError.value = null
  }

  function markReconnecting(): void {
    if (status.value === 'connected' || status.value === 'connecting') {
      status.value = 'reconnecting'
    }
  }

  function markError(message: string): void {
    status.value = 'error'
    lastError.value = message
  }

  function upsertPeer(peer: WikiCollabPeer): void {
    peers.value = { ...peers.value, [peer.clientId]: peer }
  }

  function removePeer(clientId: number): void {
    if (!(clientId in peers.value)) return
    const next = { ...peers.value }
    delete next[clientId]
    peers.value = next
  }

  function clearPeers(): void {
    peers.value = {}
  }

  function setRecentPeers(next: WikiCollabRecentPeer[]): void {
    recentPeers.value = next
  }

  return {
    status,
    session,
    peers,
    recentPeers,
    lastError,
    attempt,
    peerCount,
    peerList,
    isLive,
    setStatus,
    beginSession,
    endSession,
    markConnected,
    markReconnecting,
    markError,
    upsertPeer,
    removePeer,
    clearPeers,
    setRecentPeers,
  }
})