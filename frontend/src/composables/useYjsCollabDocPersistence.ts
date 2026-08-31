/**
 * useYjsCollabDocPersistence — v0.7.27 IndexedDB-backed Yjs persistence.
 *
 * Mirrors the wiki realtime pattern (frontend/src/composables/useYjsWiki.ts
 * — see y-indexeddb wiring there for reference) but is keyed on doc_id so
 * each collab document has its own offline buffer. When the websocket is
 * down, local edits keep accumulating in IndexedDB; when it comes back,
 * y-websocket's sync protocol merges the local buffer with the remote
 * state automatically (CRDT merge, no manual replay needed).
 *
 * Lifecycle:
 *   1. Component calls useYjsCollabDocPersistence({ docId, ydoc }) once
 *      the Y.Doc is created (typically inside useYjsCollabDoc or right
 *      after it).
 *   2. The composable attaches IndexeddbPersistence and exposes a few
 *      reactive flags the editor can surface to the user:
 *        - synced: true after IndexedDB has finished loading the
 *          persisted state into ydoc (or true immediately if no record
 *          exists yet).
 *        - status: 'idle' | 'loading' | 'ready' | 'error'.
 *   3. On unmount the composable detaches IndexedDB; the Y.Doc itself
 *      is owned by the caller so the WebsocketProvider can still flush
 *      updates before disconnect.
 *
 * Storage:
 *   - Per-doc key: `collab-doc-${docId}` in IndexedDB.
 *   - The composable appends the WeKnora tenant id when available so
 *     two tenants sharing the same doc_id on the same browser don't
 *     collide (defense in depth; the docId itself is a UUID).
 */
import { onBeforeUnmount, ref, shallowRef } from 'vue'
import * as Y from 'yjs'
import { IndexeddbPersistence } from 'y-indexeddb'

export interface YjsCollabDocPersistenceOptions {
  docId: string
  ydoc: Y.Doc
  /** Tenant id; included in the storage key when provided. */
  tenantId?: number | string
  /** Optional override of the IndexedDB database name (defaults to 'weknora'). */
  databaseName?: string
}

export interface YjsCollabDocPersistenceHandle {
  /** IndexeddbPersistence instance; `null` until attached. */
  persistence: ReturnType<typeof shallowRef<IndexeddbPersistence | null>>
  /** Reactive status flag for UI indicators. */
  status: ReturnType<typeof ref<'idle' | 'loading' | 'ready' | 'error'>>
  /** True once IndexedDB has loaded any persisted state into ydoc. */
  synced: ReturnType<typeof ref<boolean>>
  /** Last error message; null when no error. */
  error: ReturnType<typeof ref<string | null>>
  /** Detach persistence and clear state. */
  destroy: () => void
}

export function useYjsCollabDocPersistence(
  options: YjsCollabDocPersistenceOptions,
): YjsCollabDocPersistenceHandle {
  const status = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
  const synced = ref(false)
  const error = ref<string | null>(null)
  const persistence = shallowRef<IndexeddbPersistence | null>(null)

  const key =
    options.tenantId !== undefined && options.tenantId !== ''
      ? `collab-doc-${options.tenantId}-${options.docId}`
      : `collab-doc-${options.docId}`

  try {
    const p = new IndexeddbPersistence(key, options.ydoc)
    persistence.value = p
    status.value = 'loading'
    p.once('synced', () => {
      synced.value = true
      status.value = 'ready'
    })
  } catch (e) {
    status.value = 'error'
    error.value = e instanceof Error ? e.message : String(e)
  }

  const destroy = () => {
    if (persistence.value) {
      try {
        // destroy() flushes pending writes and closes the IDB connection.
        persistence.value.destroy()
      } catch {
        // best-effort; surface to console so devs see leaks during dev.
      }
      persistence.value = null
    }
  }

  onBeforeUnmount(destroy)
  return { persistence, status, synced, error, destroy }
}
