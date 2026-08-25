import assert from 'node:assert/strict'
import test from 'node:test'

import {
  aclToolbarVisibility,
} from './wikiBrowserAclVisibility.ts'

const inherit = {
  mode: 'inherit' as const,
  allowUserIds: [],
  allowGroupIds: [],
  denyInherited: false,
}
const allowListMode = {
  mode: 'allow_list' as const,
  allowUserIds: ['u1'],
  allowGroupIds: [],
  denyInherited: false,
}
const privateMode = {
  mode: 'private' as const,
  allowUserIds: [],
  allowGroupIds: [],
  denyInherited: false,
}

test('canEdit users see the full button on every ACL mode', () => {
  assert.equal(aclToolbarVisibility(inherit, true), 'writable')
  assert.equal(aclToolbarVisibility(allowListMode, true), 'writable')
  assert.equal(aclToolbarVisibility(privateMode, true), 'writable')
})

test('non-canEdit users see the readonly indicator only on restricted pages', () => {
  assert.equal(aclToolbarVisibility(inherit, false), 'hidden')
  assert.equal(aclToolbarVisibility(allowListMode, false), 'readonly')
  assert.equal(aclToolbarVisibility(privateMode, false), 'readonly')
})

test('default ACL (no page metadata) is treated as inherit and hidden for non-canEdit users', () => {
  // Sanity check: an unset ACL means the page is inheriting KB
  // permissions, so the indicator should not render for read-only
  // viewers.
  const acl = { ...inherit, allowUserIds: [], allowGroupIds: [] }
  assert.equal(aclToolbarVisibility(acl, false), 'hidden')
})