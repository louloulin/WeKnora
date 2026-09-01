/**
 * useYjsCollabDoc — v0.7.25 Yjs realtime composable for the collaborative
 * doc surface. Mirrors useYjsWiki (v0.7.19) but is keyed on doc_id instead
 * of (kb_id, page_id), so the same wire protocol (y-websocket binary) can
 * carry TipTap (doc), Univer (sheet) and pptxgenjs (slide) updates under
 * one hub.
 *
 * v0.7.38 — DOC selection range awareness. Editors call
 * `publishSelection(from, to, anchor?)` whenever their local selection
 * changes; the value rides on the awareness layer alongside `user` and
 * surfaces through `remoteSelections` for the editor to render highlight
 * rectangles over the other collaborators' selections.
 */
import { onBeforeUnmount, ref, shallowRef } from 'vue'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'
import { openCollabDocRealtimeURL } from '@/api/collabDoc'
import { useYjsCollabDocPersistence } from './useYjsCollabDocPersistence'

export interface YjsCollabDocOptions {
  docId: string
  token: string
  displayName: string
  color?: string
  /** When true (default), wire IndexedDB-backed offline persistence. */
  enableOffline?: boolean
  /** Tenant id; required when enableOffline is true so the IDB key
   *  includes a tenant discriminator (defense in depth). */
  tenantId?: number | string
}

export interface YjsCollabDocPeer {
  clientId: number
  displayName: string
  color: string
  /** DOC selection range; null/undefined when the peer has no active
   *  selection. Always {from,to} in TipTap doc positions when set. */
  selection?: { from: number; to: number } | null
  /** SHEET selection; for SHEET editors the from/to are {ri, ci}. */
  cell?: { ri: number; ci: number } | null
}

export interface YjsCollabDocHandle {
  ydoc: Y.Doc
  provider: WebsocketProvider
  connected: ReturnType<typeof ref<boolean>>
  peers: ReturnType<typeof ref<YjsCollabDocPeer[]>>
  remoteSelections: ReturnType<typeof ref<YjsCollabDocPeer[]>>
  error: ReturnType<typeof ref<string | null>>
  /** Reactive offline status; null when persistence is disabled. */
  offline: ReturnType<
    typeof import('./useYjsCollabDocPersistence').useYjsCollabDocPersistence
  > | null
  /** Broadcast a DOC selection range. Idempotent — re-publishing the
   *  same {from,to} is a no-op on the wire. */
  publishSelection: (from: number, to: number) => void
  /** Broadcast a SHEET selection (cell-level). */
  publishCellSelection: (ri: number, ci: number) => void
  /** Clear any active selection broadcast (e.g. on blur / teardown). */
  clearSelection: () => void
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

function selectionKey(from: number, to: number): string {
  return `${Math.min(from, to)}-${Math.max(from, to)}`
}

export function useYjsCollabDoc(options: YjsCollabDocOptions): YjsCollabDocHandle {
  const ydoc = new Y.Doc()
  const wsUrl = openCollabDocRealtimeURL(options.docId, options.token)
  const provider = new WebsocketProvider(wsUrl, `collab-doc-${options.docId}`, ydoc, {
    connect: true,
    params: { token: options.token },
  })

  const connected = ref(false)
  const peers = ref<YjsCollabDocPeer[]>([])
  const remoteSelections = ref<YjsCollabDocPeer[]>([])
  const error = ref<string | null>(null)
  const awareness = provider.awareness
  const localColor = options.color || pickColor(Math.floor(Math.random() * PALETTE.length))

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
    color: localColor,
  })

  let lastSelKey = ''
  let lastCellKey = ''

  const refreshPeers = () => {
    const list: YjsCollabDocPeer[] = []
    const selections: YjsCollabDocPeer[] = []
    awareness.getStates().forEach((state: any, clientId: number) => {
      if (clientId === ydoc.clientID) return
      const u = state.user || {}
      const sel = state.selection
      const cell = state.cell
      const peer: YjsCollabDocPeer = {
        clientId,
        displayName: u.name || 'Anonymous',
        color: u.color || '#58a6ff',
        selection: sel || null,
        cell: cell || null,
      }
      list.push(peer)
      if (sel || cell) selections.push(peer)
    })
    peers.value = list
    remoteSelections.value = selections
  }
  awareness.on('change', refreshPeers)
  refreshPeers()

  const publishSelection = (from: number, to: number) => {
    const key = selectionKey(from, to)
    if (key === lastSelKey) return
    lastSelKey = key
    awareness.setLocalStateField('selection', { from, to })
  }

  const publishCellSelection = (ri: number, ci: number) => {
    const key = `${ri}-${ci}`
    if (key === lastCellKey) return
    lastCellKey = key
    awareness.setLocalStateField('cell', { ri, ci })
  }

  const clearSelection = () => {
    lastSelKey = ''
    lastCellKey = ''
    awareness.setLocalStateField('selection', null)
    awareness.setLocalStateField('cell', null)
  }

  // v0.7.27 — IndexedDB-backed offline buffer. Edits made while the
  // websocket is down keep accumulating locally; y-websocket merges the
  // local state with remote on reconnect (CRDT, no manual replay).
  // Default-on so every editor that uses this composable gets the
  // behavior; pass `enableOffline: false` to opt out (e.g. for
  // share-token read-only viewers).
  const offline =
    options.enableOffline === false
      ? null
      : useYjsCollabDocPersistence({ docId: options.docId, ydoc, tenantId: options.tenantId })

  const destroy = () => {
    clearSelection()
    provider.awareness.setLocalState(null)
    provider.disconnect()
    provider.destroy()
    offline?.destroy()
  }

  onBeforeUnmount(destroy)
  return {
    ydoc,
    provider,
    connected,
    peers,
    remoteSelections,
    error,
    offline,
    publishSelection,
    publishCellSelection,
    clearSelection,
    destroy,
  }
}
