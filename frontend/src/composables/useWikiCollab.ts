import { computed, onBeforeUnmount, ref, watch } from 'vue'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'
import { useWikiCollabStore } from '../stores/wikiCollab'

/**
 * Wiki Y.js CRDT session composable (Build #8).
 *
 * Owns the lifetime of one Yjs document + WebSocket provider pair. Call
 * once per editing session — it watches `kbId` / `slug` props and
 * disconnects + reconnects when they change.
 *
 * Server contract (Build #8 — backend lands separately):
 *   WS /api/v1/wiki/collab/:kb_id/:slug?token=:jwt
 *
 * The WebsocketProvider emits binary Yjs sync messages; the server is
 * expected to fan out updates to all peers of the same room AND persist
 * the doc snapshot to `wiki_collab_snapshots` (interval configurable —
 * default 30s, debounced on last-write).
 *
 * Awareness payload schema:
 *   { user: { id, displayName }, color: '#xxxxxx' }
 *
 * Color generation: stable hash from userId so the same user keeps the
 * same color across reconnects and pages.
 */

export interface UseWikiCollabOptions {
  kbId: string
  slug: string
  userId: string
  displayName: string
  /** Disable auto-connect (used in SSR or unit tests). */
  enabled?: boolean
  /** Override the WS endpoint (defaults to current origin). */
  endpoint?: string
}

function wsEndpoint(origin: string, kbId: string, slug: string): string {
  // Server contract: /api/v1/wiki/collab/:kb_id/:slug
  // The WebsocketProvider joins a room named after the path's last segment;
  // the server uses the same room name as the channel key.
  return `${origin.replace(/\/$/, '')}/api/v1/wiki/collab/${encodeURIComponent(kbId)}/${encodeURIComponent(slug)}`
}

function hashColor(seed: string): string {
  // FNV-1a 32-bit → 24-bit hue bucket → HSL with fixed S/L for consistent
  // brightness on light + dark themes.
  let hash = 0x811c9dc5
  for (let i = 0; i < seed.length; i += 1) {
    hash ^= seed.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  const hue = ((hash >>> 0) % 360 + 360) % 360
  return `hsl(${hue}, 65%, 48%)`
}

export function useWikiCollab(opts: UseWikiCollabOptions) {
  const store = useWikiCollabStore()
  const ydoc = new Y.Doc()
  const provider = ref<WebsocketProvider | null>(null)
  const enabled = ref(opts.enabled !== false)
  const destroyed = ref(false)

  const color = computed(() => hashColor(opts.userId || opts.displayName || 'anon'))

  function teardown(): void {
    if (provider.value) {
      try {
        provider.value.awareness.setLocalState(null)
        provider.value.disconnect()
        provider.value.destroy()
      } catch {
        // Provider may already be torn down by y-websocket on close.
      }
      provider.value = null
    }
    store.endSession()
  }

  function connect(): void {
    if (!enabled.value || destroyed.value) return
    teardown()

    const origin =
      typeof window !== 'undefined' ? window.location.origin : 'http://localhost'
    const wsUrl = wsEndpoint(
      opts.endpoint ?? origin,
      opts.kbId,
      opts.slug,
    )

    store.beginSession({
      kbId: opts.kbId,
      slug: opts.slug,
      wsUrl,
    })

    let p: WebsocketProvider
    try {
      p = new WebsocketProvider(wsUrl, opts.slug, ydoc, {
        // Disable built-in IndexedDB persistence — server is source of truth.
        connect: true,
      })
    } catch (err) {
      store.markError(errMsg(err) || 'collab.connectFailed')
      return
    }

    provider.value = p

    p.on('status', (event: { status: 'connecting' | 'connected' | 'disconnected' }) => {
      if (event.status === 'connected') store.markConnected()
      else if (event.status === 'connecting') store.setStatus('connecting')
      else if (event.status === 'disconnected') store.markReconnecting()
    })

    p.on('connection-error', (err: unknown) => {
      store.markError(errMsg(err) || 'collab.connectionError')
    })

    p.on('sync', (isSynced: boolean) => {
      if (isSynced) store.markConnected()
    })

    // Awareness — sync the local user into the peer roster.
    p.awareness.setLocalState({
      user: { id: opts.userId, displayName: opts.displayName },
      color: color.value,
    })

    const refreshPeers = (): void => {
      const states = p.awareness.getStates()
      const next: typeof store.peers.value = {}
      states.forEach((value, clientId) => {
        if (clientId === p.awareness.clientID) return
        const user = (value as { user?: { id?: string; displayName?: string } })
          ?.user
        if (!user?.id) return
        next[clientId] = {
          clientId,
          userId: user.id,
          displayName: user.displayName || user.id,
          color: (value as { color?: string }).color || '#888',
          lastSeen: new Date().toISOString(),
        }
      })
      store.clearPeers()
      for (const peer of Object.values(next)) store.upsertPeer(peer)
    }
    p.awareness.on('change', refreshPeers)
    refreshPeers()
  }

  watch(
    () => [opts.kbId, opts.slug, opts.userId, opts.displayName] as const,
    (next, prev) => {
      const [nextKb, nextSlug] = next
      const [prevKb, prevSlug] = prev ?? ['', '']
      if (nextKb !== prevKb || nextSlug !== prevSlug) {
        connect()
      } else {
        // Identity change only — refresh awareness.
        provider.value?.awareness.setLocalState({
          user: { id: opts.userId, displayName: opts.displayName },
          color: color.value,
        })
      }
    },
  )

  onBeforeUnmount(() => {
    destroyed.value = true
    teardown()
  })

  if (enabled.value && !destroyed.value) {
    // Connect synchronously so callers (e.g. WikiTiptapEditor) can hand
    // the provider reference straight to the Collaboration extensions.
    // The WebSocket handshake itself is async and driven by y-websocket;
    // we don't need to await it before mounting the editor.
    connect()
  }

  return {
    ydoc,
    provider,
    color,
    status: computed(() => store.status),
    peerList: computed(() => store.peerList),
    peerCount: computed(() => store.peerCount),
    isLive: computed(() => store.isLive),
    lastError: computed(() => store.lastError),
    reconnect: connect,
    disconnect: teardown,
  }
}

function errMsg(err: unknown): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string') return m
  }
  return ''
}