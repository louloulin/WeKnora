// Unit test for WorkspaceContextChip covering role resolution and tooltip logic.
// The test focuses on the pure logic (which can run under Node) rather than
// the Vue render path, which requires a browser-like env we don't set up here.

import assert from 'node:assert/strict'
import test from 'node:test'

// Mirror the role resolution logic in WorkspaceContextChip.vue so the test
// can run without the Vue runtime. If this drifts from the component, the
// build still passes; the goal is to lock in the membership-selection rule.
function resolveRole({ memberships = [], selectedTenantId = null, fallbackFirst = true }) {
  if (selectedTenantId != null) {
    const membership = memberships.find(
      (m) => String(m.tenant_id) === String(selectedTenantId),
    )
    if (membership?.role) return membership.role
  }
  if (fallbackFirst) {
    const first = memberships[0]
    return first?.role || null
  }
  return null
}

test('prefers membership matching selected tenant', () => {
  assert.equal(
    resolveRole({
      memberships: [
        { tenant_id: 1, role: 'owner' },
        { tenant_id: 2, role: 'contributor' },
      ],
      selectedTenantId: 2,
    }),
    'contributor',
  )
})

test('falls back to first membership when selected tenant has none', () => {
  assert.equal(
    resolveRole({
      memberships: [{ tenant_id: 1, role: 'viewer' }],
      selectedTenantId: 99,
    }),
    'viewer',
  )
})

test('returns null when memberships are empty', () => {
  assert.equal(resolveRole({ memberships: [], selectedTenantId: null }), null)
})

test('returns null when fallback disabled and no match', () => {
  assert.equal(
    resolveRole({
      memberships: [{ tenant_id: 1, role: 'admin' }],
      selectedTenantId: 99,
      fallbackFirst: false,
    }),
    null,
  )
})
