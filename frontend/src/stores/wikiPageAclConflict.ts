import {
  type WikiPageAcl,
  defaultWikiPageAcl,
} from '../api/wiki/aclTypes'

/**
 * Pure helpers extracted from `useWikiPageAclStore` so unit tests can
 * exercise the 409 → SaveAclResult decoding path without spinning up
 * Pinia + axios + i18n.
 */

export function errStatus(err: unknown): number | null {
  if (err && typeof err === 'object' && 'status' in err) {
    const s = (err as { status?: unknown }).status
    if (typeof s === 'number') return s
  }
  return null
}

export function errMsg(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string' && m) return m
  }
  return fallback
}

// Lightweight pub/sub used by the dialog to react to out-of-band ACL
// updates (e.g. another admin's save surfaced via 409, or a future
// websocket push). Built-ins avoid pulling mitt/eventemitter3 just for
// two events.
export type AclEvent =
  | { kind: 'updated'; kbId: string; slug: string; acl: WikiPageAcl }
  | { kind: 'conflict'; kbId: string; slug: string; current: WikiPageAcl }

export type AclEventListener = (e: AclEvent) => void

const aclListeners = new Set<AclEventListener>()

export function emitAclEvent(event: AclEvent): void {
  aclListeners.forEach((listener) => {
    try {
      listener(event)
    } catch {
      // listeners are best-effort; an exception in one must not stop others
    }
  })
}

export function onAclEvent(listener: AclEventListener): () => void {
  aclListeners.add(listener)
  return () => aclListeners.delete(listener)
}

/** Normalize arbitrary backend ACL payload into the typed WikiPageAcl. */
export function normalizeAcl(input: unknown): WikiPageAcl {
  const fallback = defaultWikiPageAcl()
  if (!input || typeof input !== 'object') return fallback
  const src = input as Record<string, unknown>
  const modeRaw = src.mode
  const mode: WikiPageAcl['mode'] =
    modeRaw === 'private' || modeRaw === 'allow_list' ? modeRaw : 'inherit'
  const allowUserIds = Array.isArray(src.allowUserIds)
    ? src.allowUserIds.filter((x): x is string => typeof x === 'string')
    : []
  const allowGroupIds = Array.isArray(src.allowGroupIds)
    ? src.allowGroupIds.filter((x): x is string => typeof x === 'string')
    : []
  const denyInherited = Boolean(src.denyInherited)
  const revision = typeof src.revision === 'number' ? src.revision : undefined
  const updatedAt = typeof src.updatedAt === 'string' ? src.updatedAt : undefined
  return {
    mode,
    allowUserIds,
    allowGroupIds,
    denyInherited,
    revision,
    updatedAt,
  }
}

/**
 * Decode a server error from `putWikiPageAcl` into the discriminated
 * `SaveAclResult` shape. Only the error arms are returned here —
 * callers should treat this as `Result<WikiPageAcl, ...>` style:
 *   - `{ kind: 'conflict', current }` for 409 with a usable payload.
 *   - `{ kind: 'error', message }` for everything else.
 */
export type DecodedAclError =
  | { kind: 'conflict'; current: WikiPageAcl }
  | { kind: 'error'; message: string }

export function decodeAclError(
  err: unknown,
  fallbackMsg = 'acl.saveFailed',
): DecodedAclError {
  const status = errStatus(err)
  if (status === 409) {
    const data = err && typeof err === 'object'
      ? ((err as Record<string, unknown>).data as Record<string, unknown> | null | undefined) ?? null
      : null
    const raw =
      (data && (data.currentAcl ?? data.current_acl ?? data.current ?? data.data)) || null
    return { kind: 'conflict', current: normalizeAcl(raw) }
  }
  return { kind: 'error', message: errMsg(err, fallbackMsg) }
}

// Re-exports so the store can `import { decodeAclError, errStatus,
// normalizeAcl, onAclEvent, emitAclEvent } from './wikiPageAclConflict'`
// and so the unit test can exercise them without loading axios / vue-i18n.