import type { WikiPageAcl } from '../../../api/wiki/aclTypes'

/**
 * Pure helpers extracted from `WikiBrowser.vue` for the ACL toolbar
 * button + readonly indicator (Build #10). Centralizing this here
 * keeps the template logic readable and lets us unit-test the
 * visibility rules without mounting the whole component.
 */

export type AclToolbarVisibility =
  | 'writable' // full button — owner/admin can open the dialog
  | 'readonly' // muted lock + tooltip; restricted page, no write
  | 'hidden' // no need to render anything (page not restricted + user can't edit)

/**
 * Decide whether the toolbar should show the lock icon, and whether
 * the user can act on it.
 *
 *   - `pageAcl.restricted` is derived from the stored ACL mode.
 *   - `props.canEdit` is the existing visibility flag for edit-level
 *     operations on the page (parent passes it from KnowledgeBase.vue).
 */
export function aclToolbarVisibility(
  acl: WikiPageAcl,
  canEdit: boolean,
): AclToolbarVisibility {
  const restricted = acl.mode !== 'inherit'
  if (canEdit) return 'writable'
  return restricted ? 'readonly' : 'hidden'
}