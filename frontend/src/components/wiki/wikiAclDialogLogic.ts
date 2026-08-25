import type { WikiPageAcl } from '../../api/wiki/aclTypes'

/**
 * Pure helpers extracted from `WikiAclDialog.vue` so the dialog's
 * `isDirty` and timestamp-rendering logic can be unit-tested without
 * mounting the full TDesign dialog or i18n.
 */

export function isAclDraftDirty(
  draft: Pick<WikiPageAcl, 'mode' | 'allowUserIds' | 'denyInherited'>,
  original: Pick<WikiPageAcl, 'mode' | 'allowUserIds' | 'denyInherited'>,
): boolean {
  if (draft.mode !== original.mode) return true
  if (draft.denyInherited !== original.denyInherited) return true
  if (draft.allowUserIds.length !== original.allowUserIds.length) return true
  const originalIds = new Set(original.allowUserIds)
  for (const id of draft.allowUserIds) {
    if (!originalIds.has(id)) return true
  }
  return false
}

/**
 * Format `updatedAt` for display in the dialog footer. Returns '' when
 * the source ACL has no timestamp; falls back to the raw ISO string
 * when `Intl.DateTimeFormat` throws (e.g. exotic locales).
 */
export function formatAclUpdatedAt(
  updatedAt: string | undefined,
  locale: string,
): string {
  if (!updatedAt) return ''
  const d = new Date(updatedAt)
  if (Number.isNaN(d.getTime())) return updatedAt
  try {
    return new Intl.DateTimeFormat(locale, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }).format(d)
  } catch {
    return d.toISOString()
  }
}