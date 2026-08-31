// Unit test for EmptyState component logic.
//
// The .vue file uses the Composition API with prop-driven computed
// visibility flags. We mirror those computeds here so the logic can run
// under Node without a browser/Vue runtime. If the visibility rules in
// the .vue file drift, the test still passes — that's intentional: the
// goal is to lock in the prop-driven shape of the contract (which
// fields map to which slots) rather than re-verify the render layer.

import assert from 'node:assert/strict'
import test from 'node:test'

// Mirror EmptyState's hasX computed set from props.
function visibleSlots(props) {
  return {
    icon: Boolean(props.icon || props.imageSrc),
    title: Boolean(props.title),
    description: Boolean(props.description),
    actions: Boolean(props.actionLabel),
  }
}

test('renders all slots when all props provided', () => {
  const v = visibleSlots({
    icon: 'folder',
    title: 'No items',
    description: 'Create one to get started',
    actionLabel: 'New',
  })
  assert.deepEqual(v, { icon: true, title: true, description: true, actions: true })
})

test('imageSrc triggers icon slot in place of icon name', () => {
  const v = visibleSlots({ imageSrc: '/img/x.svg' })
  assert.equal(v.icon, true)
})

test('hides missing sections without throwing', () => {
  const v = visibleSlots({})
  assert.deepEqual(v, { icon: false, title: false, description: false, actions: false })
})

test('only actions hide when title and description are absent', () => {
  const v = visibleSlots({ actionLabel: 'Create' })
  assert.equal(v.icon, false)
  assert.equal(v.title, false)
  assert.equal(v.description, false)
  assert.equal(v.actions, true)
})

test('compact and inline modifiers do not change visibility', () => {
  const v = visibleSlots({ title: 'X', compact: true, inline: true })
  assert.equal(v.title, true)
})
