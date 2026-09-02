/**
 * mindmapFormat — v0.7.111 MindMap / WIKI 打磨 (Phase 1)
 *
 * Lightweight presentation helpers extracted from MindMapListView.vue so
 * the timezone-sensitive formatting has unit-test coverage.
 */

/** Format an ISO timestamp as a Chinese short month + day ("9月2日"). */
export function formatShortDate(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  try {
    return d.toLocaleString('zh-CN', { month: 'short', day: 'numeric' })
  } catch {
    return ''
  }
}

/** Format an ISO timestamp as a full Chinese datetime. */
export function formatLongDate(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  try {
    return d.toLocaleString('zh-CN', {
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return ''
  }
}

/** Validate a MindMap layout name. */
export function isMindMapLayout(s: string): s is 'tree' | 'fishbone' | 'timeline' | 'radial' | 'free' {
  return s === 'tree' || s === 'fishbone' || s === 'timeline' || s === 'radial' || s === 'free'
}
