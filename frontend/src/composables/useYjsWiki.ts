/**
 * useYjsWiki — v0.7.19 Yjs realtime collaboration composable for the Wiki
 * editor. Wires a Tiptap editor to a y-websocket provider pointed at the
 * WeKnora backend endpoint:
 *
 *   ws://host/api/v1/knowledgebase/:kbId/wiki/realtime/:pageId?token=...
 *
 * The token query parameter is required because the browser cannot set
 * custom headers on a WebSocket request; the backend AuthZ middleware
 * accepts it as an equivalent to the Authorization header.
 *
 * The composable returns reactive refs for:
 *   - editor: the Tiptap Editor instance (or null while connecting)
 *   - connected: boolean — true once the WS handshake completes
 *   - peers: list of awareness states (cursor color + display name)
 *   - error: last error message or null
 *
 * Tearing down the composable closes the WS and clears awareness.
 */
import { ref, shallowRef, onBeforeUnmount } from 'vue'
import { Editor } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import { Collaboration } from '@tiptap/extension-collaboration'
import { CollaborationCursor } from '@tiptap/extension-collaboration-cursor'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'

export interface YjsWikiOptions {
  /** Knowledge base id — first URL path segment. */
  kbId: string
  /** Wiki page id — second URL path segment. */
  pageId: string
  /** JWT access token (browser can't set custom WS headers). */
  token: string
  /** Display name for the local user — shown in collaboration cursors. */
  displayName: string
  /** Cursor color (hex). Auto-picked from a 12-color palette when absent. */
  color?: string
  /** Initial content to hydrate the doc with when no snapshot exists. */
  initialContent?: string
}

export interface YjsWikiHandle {
  editor: ReturnType<typeof shallowRef<Editor | null>>
  connected: ReturnType<typeof ref<boolean>>
  peers: ReturnType<typeof ref<Array<{ clientId: number; displayName: string; color: string }>>>
  error: ReturnType<typeof ref<string | null>>
  destroy: () => void
}

const PALETTE = [
  '#58a6ff', '#3fb950', '#d29922', '#f85149',
  '#a371f7', '#1f6feb', '#bf3989', '#39c5cf',
  '#f0883e', '#7d8590', '#2ea043', '#db61a2',
]

function pickColor(seed: number): string {
  return PALETTE[Math.abs(seed) % PALETTE.length]
}

export function useYjsWiki(options: YjsWikiOptions): YjsWikiHandle {
  const editor = shallowRef<Editor | null>(null)
  const connected = ref(false)
  const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
  const error = ref<string | null>(null)

  // Build the WS URL — match the backend route shape.
  const baseUrl = (window.location.protocol === 'https:' ? 'wss://' : 'ws://') + window.location.host
  const wsUrl = `${baseUrl}/api/v1/knowledgebase/${encodeURIComponent(options.kbId)}/wiki/realtime/${encodeURIComponent(options.pageId)}?token=${encodeURIComponent(options.token)}`

  const ydoc = new Y.Doc()
  const provider = new WebsocketProvider(wsUrl, `wiki-${options.kbId}-${options.pageId}`, ydoc, {
    connect: true,
    // y-websocket doesn't accept a headers param natively; the token
    // travels in the URL query string by design.
  })

  provider.on('status', (event: { status: string }) => {
    if (event.status === 'connected') {
      connected.value = true
      error.value = null
    } else if (event.status === 'disconnected') {
      connected.value = false
    }
  })

  provider.on('connection-error', (event: Event) => {
    error.value = `realtime: connection error (${(event as ErrorEvent).message || 'unknown'})`
    connected.value = false
  })

  // Local user state — broadcast to peers.
  provider.awareness.setLocalStateField('user', {
    name: options.displayName,
    color: options.color || pickColor(Math.floor(Math.random() * PALETTE.length)),
  })

  // Track peer presence.
  const refreshPeers = () => {
    const states = Array.from(provider.awareness.getStates().entries())
    const others: Array<{ clientId: number; displayName: string; color: string }> = []
    for (const [clientId, state] of states) {
      if (clientId === provider.awareness.clientID) continue
      const u = (state as { user?: { name?: string; color?: string } }).user
      if (!u) continue
      others.push({
        clientId,
        displayName: u.name || `User ${clientId}`,
        color: u.color || '#9da7b3',
      })
    }
    peers.value = others
  }
  provider.awareness.on('change', refreshPeers)
  refreshPeers()

  // Build Tiptap editor with collaboration extensions.
  editor.value = new Editor({
    extensions: [
      StarterKit.configure({
        // History is owned by Yjs in collab mode.
        history: false,
      }),
      Link.configure({ openOnClick: false }),
      Collaboration.configure({ document: ydoc }),
      CollaborationCursor.configure({
        provider,
        user: {
          name: options.displayName,
          color: options.color || pickColor(Math.floor(Math.random() * PALETTE.length)),
        },
      }),
    ],
    content: options.initialContent || '',
  })

  const destroy = () => {
    try {
      editor.value?.destroy()
    } catch {
      // Editor already destroyed — safe to ignore.
    }
    try {
      provider.disconnect()
      provider.destroy()
    } catch {
      // Provider already destroyed — safe to ignore.
    }
    ydoc.destroy()
    editor.value = null
    connected.value = false
    peers.value = []
  }

  onBeforeUnmount(destroy)

  return { editor, connected, peers, error, destroy }
}
