/**
 * xlsxCellLock — SHEET cell-lock helpers (v0.7.77).
 *
 * The "lock" is a soft optimistic lock: when a peer has selected a cell via the
 * awareness layer (remoteSelections.cell), other peers treat that cell as
 * read-only until the peer deselects / moves away. We don't gate Yjs writes
 * at the CRDT level (Yjs merges concurrent writes by construction), but we DO
 * gate the UI (input read-only + tooltip) and we surface a warning toast when
 * a local user tries to write to a cell that a peer is currently editing.
 *
 * Vendored from genoffice as a soft pattern; no equivalent file exists in the
 * editor/ tree because it's a SHEET-only concept. Implementation is local.
 */

export interface RemoteCellPeer {
  clientId: number
  displayName: string
  color: string
  cell?: { ri: number; ci: number } | null
}

export interface CellLockResult {
  locked: boolean
  locker: RemoteCellPeer | null
}

/** Is the cell currently locked by any peer other than me? */
export function isCellLockedByOther(
  remoteSelections: ReadonlyArray<RemoteCellPeer>,
  myClientId: number,
  ri: number,
  ci: number,
): CellLockResult {
  const locker = remoteSelections.find(
    (p) =>
      p.clientId !== myClientId &&
      p.cell &&
      p.cell.ri === ri &&
      p.cell.ci === ci,
  )
  return { locked: !!locker, locker: locker ?? null }
}

/** All currently-locked cells, with their locker, as a flat map. */
export function buildLockMap(
  remoteSelections: ReadonlyArray<RemoteCellPeer>,
  myClientId: number,
): Map<string, RemoteCellPeer> {
  const out = new Map<string, RemoteCellPeer>()
  for (const p of remoteSelections) {
    if (p.clientId === myClientId) continue
    if (!p.cell) continue
    out.set(`${p.cell.ri}:${p.cell.ci}`, p)
  }
  return out
}

/** Helper to convert (ri, ci) to the lock-map key. */
export function cellKey(ri: number, ci: number): string {
  return `${ri}:${ci}`
}

/**
 * Decide whether a local edit should proceed. Returns the reason to surface
 * to the user, or null when the edit is allowed.
 */
export function checkEditAllowed(
  remoteSelections: ReadonlyArray<RemoteCellPeer>,
  myClientId: number,
  ri: number,
  ci: number,
): { allowed: true } | { allowed: false; locker: string } {
  const { locked, locker } = isCellLockedByOther(remoteSelections, myClientId, ri, ci)
  if (locked && locker) {
    return { allowed: false, locker: locker.displayName }
  }
  return { allowed: true }
}
