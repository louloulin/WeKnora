/**
 * useYjsCollabDoc — v0.7.25 Yjs realtime composable for the collaborative
 * doc surface. Mirrors useYjsWiki (v0.7.19) but is keyed on doc_id instead
 * of (kb_id, page_id), so the same wire protocol (y-websocket binary) can
 * carry TipTap (doc), Univer (sheet) and pptxgenjs (slide) updates under
 * one hub.
 */
import { onBeforeUnmount, ref, shallowRef } from 'vue'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'
import { openCollabDocRealtimeURL } from '@/api/collabDoc'

export interface YjsCollabDocOptions {
  docId: string
  token: string
  displayName: string
  color?: string
}

export interface YjsCollabDocHandle {
  ydoc: Y.Doc
  provider: WebsocketProvider
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

export function useYjsCollabDoc(options: YjsCollabDocOptions): YjsCollabDocHandle {
  const ydoc = new Y.Doc()
  const wsUrl = openCollabDocRealtimeURL(options.docId, options.token)
  const provider = new WebsocketProvider(wsUrl, `collab-doc-${options.docId}`, ydoc, { connect: true })

  const connected = ref(false)
  const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
  const error = ref<string | null>(null)
  const awareness = provider.awareness

  provider.on('status', (event: { status: string }) => {
    if (event.status === 'connected') connected.value = true
    else if (event.status === 'disconnected') connected.value = false
  })
  provider.on('connection-error', (e: Event) => {
    error.value = `realtime: ${(e as ErrorEvent).message || 'unknown'}`
    connected.value = false
  })

  awareness.setLocalStateField('user', {
    name: options.displayName,
    color: options.color || pickColor(Math.floor(Math.random() * PALETTE.length)),
  })

  const refreshPeers = () => {
    const out: Array<{ clientId: number; displayName: string; color: string }> = []
    awareness.getStates().forEach((state: any, clientId: number) => {
      if (clientId === ydoc.clientID) return
      const u = state.user || {}
      out.push({ clientId, displayName: u.name || 'Anonymous', color: u.color || '#58a6ff' })
    })
    peers.value = out
  }
  awareness.on('change', refreshPeers)
  refreshPeers()

  const destroy = () => {
    provider.awareness.setLocalState(null)
    provider.disconnect()
    provider.destroy()
  }

  onBeforeUnmount(destroy)
  return { ydoc, provider, connected, peers, error, destroy }
}
