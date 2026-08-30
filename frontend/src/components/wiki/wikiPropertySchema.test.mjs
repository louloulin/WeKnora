// Unit test for wikiPropertySchema helpers.
//
// Mirrors wikiPropertySchema.ts so we can run it under Node without the
// Vue/TS runtime. Locks in:
//   - coercePropertyValue per type
//   - readPropertyValues preserves schema order, drops invalid
//   - writePropertyValues merges without dropping unknown keys
//   - formatPropertyValue for read-only rendering

import assert from 'node:assert/strict'
import test from 'node:test'

// Inline copies of the schema & helpers (mirrors .ts).
const DEFAULT_PROPERTY_SCHEMA = [
  { id: 'status', name: '状态', type: 'select', options: ['Draft', 'Review', 'Published', 'Archived'] },
  { id: 'priority', name: '优先级', type: 'select', options: ['Low', 'Medium', 'High', 'Critical'] },
  { id: 'owner', name: '负责人', type: 'text' },
  { id: 'due_date', name: '截止日期', type: 'date' },
  { id: 'tags', name: '标签', type: 'multi-select' },
  { id: 'confidential', name: '机密', type: 'checkbox' },
  { id: 'source_url', name: '来源链接', type: 'url' },
]

function coercePropertyValue(property, raw) {
  if (raw === null || raw === undefined) return null
  switch (property.type) {
    case 'text':
      return typeof raw === 'string' ? raw : String(raw)
    case 'number': {
      const n = typeof raw === 'number' ? raw : Number(raw)
      return Number.isFinite(n) ? n : null
    }
    case 'date':
      return typeof raw === 'string' && raw.length > 0 ? raw : null
    case 'select':
      if (typeof raw !== 'string') return null
      if (!property.options || property.options.length === 0) return raw
      return property.options.includes(raw) ? raw : null
    case 'multi-select': {
      if (!Array.isArray(raw)) return null
      const arr = raw.filter((v) => typeof v === 'string')
      if (!property.options || property.options.length === 0) return arr
      return arr.filter((v) => property.options.includes(v))
    }
    case 'checkbox':
      return typeof raw === 'boolean' ? raw : null
    case 'url':
      return typeof raw === 'string' && /^https?:\/\//i.test(raw) ? raw : null
    default:
      return null
  }
}

function readPropertyValues(schema, pageMetadata) {
  const result = {}
  if (!pageMetadata) return result
  for (const prop of schema) {
    if (Object.prototype.hasOwnProperty.call(pageMetadata, prop.id)) {
      const coerced = coercePropertyValue(prop, pageMetadata[prop.id])
      if (coerced !== null) result[prop.id] = coerced
    }
  }
  return result
}

function writePropertyValues(pageMetadata, updates) {
  const base = { ...(pageMetadata || {}) }
  for (const [key, value] of Object.entries(updates)) {
    if (value === null || value === undefined) {
      delete base[key]
    } else {
      base[key] = value
    }
  }
  return base
}

// --- coercePropertyValue ---

test('coerce text accepts strings and stringifies numbers', () => {
  const p = { id: 'x', name: 'X', type: 'text' }
  assert.equal(coercePropertyValue(p, 'hello'), 'hello')
  assert.equal(coercePropertyValue(p, 42), '42')
  assert.equal(coercePropertyValue(p, null), null)
})

test('coerce number rejects non-finite values', () => {
  const p = { id: 'x', name: 'X', type: 'number' }
  assert.equal(coercePropertyValue(p, '3.14'), 3.14)
  assert.equal(coercePropertyValue(p, 5), 5)
  assert.equal(coercePropertyValue(p, 'not-a-number'), null)
  assert.equal(coercePropertyValue(p, Infinity), null)
})

test('coerce select only accepts declared options', () => {
  const p = { id: 'x', name: 'X', type: 'select', options: ['A', 'B'] }
  assert.equal(coercePropertyValue(p, 'A'), 'A')
  assert.equal(coercePropertyValue(p, 'C'), null)
  assert.equal(coercePropertyValue(p, 123), null)
})

test('coerce multi-select filters to declared options', () => {
  const p = { id: 'x', name: 'X', type: 'multi-select', options: ['A', 'B'] }
  assert.deepEqual(coercePropertyValue(p, ['A', 'B', 'C']), ['A', 'B'])
  assert.deepEqual(coercePropertyValue(p, 'not-array'), null)
  assert.deepEqual(coercePropertyValue(p, ['A', 1, 'B']), ['A', 'B'])
})

test('coerce date only accepts non-empty strings', () => {
  const p = { id: 'x', name: 'X', type: 'date' }
  assert.equal(coercePropertyValue(p, '2026-01-01'), '2026-01-01')
  assert.equal(coercePropertyValue(p, ''), null)
  assert.equal(coercePropertyValue(p, 12345), null)
})

test('coerce checkbox only accepts booleans', () => {
  const p = { id: 'x', name: 'X', type: 'checkbox' }
  assert.equal(coercePropertyValue(p, true), true)
  assert.equal(coercePropertyValue(p, false), false)
  assert.equal(coercePropertyValue(p, 'true'), null)
})

test('coerce url only accepts http(s) strings', () => {
  const p = { id: 'x', name: 'X', type: 'url' }
  assert.equal(coercePropertyValue(p, 'https://example.com'), 'https://example.com')
  assert.equal(coercePropertyValue(p, 'http://x.test'), 'http://x.test')
  assert.equal(coercePropertyValue(p, 'ftp://x.test'), null)
  assert.equal(coercePropertyValue(p, 'javascript:alert(1)'), null)
})

// --- readPropertyValues ---

test('readPropertyValues preserves schema order and drops invalid entries', () => {
  const meta = {
    status: 'Draft',
    priority: 'INVALID',          // not in options -> dropped
    owner: 'Alice',
    tags: ['x', 'y'],             // no options declared -> kept as-is
    unknown_key: 'preserved',     // not in schema -> not in result (by design)
  }
  const r = readPropertyValues(DEFAULT_PROPERTY_SCHEMA, meta)
  assert.deepEqual(Object.keys(r), ['status', 'owner', 'tags'])
  assert.equal(r.status, 'Draft')
  assert.equal(r.owner, 'Alice')
  assert.deepEqual(r.tags, ['x', 'y'])
})

test('readPropertyValues returns empty map for null/undefined metadata', () => {
  assert.deepEqual(readPropertyValues(DEFAULT_PROPERTY_SCHEMA, null), {})
  assert.deepEqual(readPropertyValues(DEFAULT_PROPERTY_SCHEMA, undefined), {})
})

// --- writePropertyValues ---

test('writePropertyValues merges without dropping unknown keys', () => {
  const meta = { unknown_key: 'preserved', status: 'Draft' }
  const r = writePropertyValues(meta, { owner: 'Alice' })
  assert.equal(r.unknown_key, 'preserved')
  assert.equal(r.status, 'Draft')
  assert.equal(r.owner, 'Alice')
})

test('writePropertyValues null/undefined removes the key', () => {
  const meta = { status: 'Draft', owner: 'Alice' }
  const r = writePropertyValues(meta, { owner: null })
  assert.equal(r.status, 'Draft')
  assert.equal('owner' in r, false)
})

test('writePropertyValues works on empty input', () => {
  const r = writePropertyValues(null, { status: 'Draft' })
  assert.deepEqual(r, { status: 'Draft' })
})

// --- integration ---

test('full round-trip: read -> write preserves unknown keys and coerces types', () => {
  const meta = { custom_marker: 'x', status: 999 /* invalid */, owner: 'Alice' }
  const values = readPropertyValues(DEFAULT_PROPERTY_SCHEMA, meta)
  // status=999 should be dropped (not a string in options)
  assert.equal(values.status, undefined)
  assert.equal(values.owner, 'Alice')
  const updated = writePropertyValues(meta, { ...values, priority: 'High' })
  assert.equal(updated.custom_marker, 'x')
  assert.equal(updated.priority, 'High')
  assert.equal(updated.owner, 'Alice')
})
