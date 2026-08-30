// Unit test for WikiDatabaseView component logic.
//
// Mirrors the .vue file's filter / sort / cell-format logic so we can
// run it under Node without the Vue runtime. Locks in:
//   - filterText matches title / slug / summary / property values
//   - filterType scopes to a specific page_type
//   - sort toggles direction on repeated header click
//   - nulls always sort last regardless of direction
//   - different value types (number / string / boolean) compare sanely

import assert from 'node:assert/strict'
import test from 'node:test'

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
    case 'text': return typeof raw === 'string' ? raw : String(raw)
    case 'number': {
      const n = typeof raw === 'number' ? raw : Number(raw)
      return Number.isFinite(n) ? n : null
    }
    case 'date': return typeof raw === 'string' && raw.length > 0 ? raw : null
    case 'select':
      if (typeof raw !== 'string') return null
      if (!property.options || property.options.length === 0) return raw
      return property.options.includes(raw) ? raw : null
    case 'multi-select': {
      if (!Array.isArray(raw)) return null
      const arr = raw.filter((v) => typeof v === 'string')
      return arr
    }
    case 'checkbox': return typeof raw === 'boolean' ? raw : null
    case 'url': return typeof raw === 'string' && /^https?:\/\//i.test(raw) ? raw : null
    default: return null
  }
}

function readPropertyValues(schema, meta) {
  const r = {}
  if (!meta) return r
  for (const p of schema) {
    if (Object.prototype.hasOwnProperty.call(meta, p.id)) {
      const v = coercePropertyValue(p, meta[p.id])
      if (v !== null) r[p.id] = v
    }
  }
  return r
}

function matchesFilter(page, { filterText, filterType }) {
  if (filterType && page.page_type !== filterType) return false
  if (!filterText || !filterText.trim()) return true
  const q = filterText.trim().toLowerCase()
  if (page.title.toLowerCase().includes(q)) return true
  if (page.slug.toLowerCase().includes(q)) return true
  if (page.summary && page.summary.toLowerCase().includes(q)) return true
  const values = readPropertyValues(DEFAULT_PROPERTY_SCHEMA, page.page_metadata || {})
  for (const v of Object.values(values)) {
    if (v === null || v === undefined) continue
    if (Array.isArray(v)) { if (v.some((x) => String(x).toLowerCase().includes(q))) return true }
    else if (typeof v === 'string' || typeof v === 'number') {
      if (String(v).toLowerCase().includes(q)) return true
    } else if (typeof v === 'boolean') {
      if ((v ? 'true' : 'false').includes(q)) return true
    }
  }
  return false
}

function getCellValue(page, propertyId) {
  const values = readPropertyValues(DEFAULT_PROPERTY_SCHEMA, page.page_metadata || {})
  return values[propertyId] ?? null
}

function compare(a, b, k, sortDir) {
  let av, bv
  if (k === 'title') { av = a.title; bv = b.title }
  else if (k === 'page_type') { av = a.page_type; bv = b.page_type }
  else if (k === 'status') { av = a.status; bv = b.status }
  else { av = getCellValue(a, k); bv = getCellValue(b, k) }
  const aNull = av === null || av === undefined
  const bNull = bv === null || bv === undefined
  if (aNull && bNull) return 0
  if (aNull) return 1
  if (bNull) return -1
  let cmp
  if (typeof av === 'number' && typeof bv === 'number') cmp = av - bv
  else if (typeof av === 'boolean' && typeof bv === 'boolean') cmp = Number(av) - Number(bv)
  else cmp = String(av).localeCompare(String(bv))
  return sortDir === 'asc' ? cmp : -cmp
}

function applyView(pages, { filterText, filterType, sortKey, sortDir }) {
  const filtered = pages.filter((p) => matchesFilter(p, { filterText, filterType }))
  return [...filtered].sort((a, b) => compare(a, b, sortKey, sortDir))
}

// --- filter logic ---

const samplePages = [
  { id: '1', slug: 'getting-started', title: 'Getting Started', page_type: 'guide', status: 'published',
    summary: 'How to start using WeKnora', page_metadata: { status: 'Published', priority: 'High', owner: 'Alice' } },
  { id: '2', slug: 'api-reference', title: 'API Reference', page_type: 'doc', status: 'draft',
    summary: 'REST and gRPC endpoints', page_metadata: { status: 'Draft', priority: 'Medium', owner: 'Bob', tags: ['api', 'rest'] } },
  { id: '3', slug: 'faq', title: 'Frequently Asked Questions', page_type: 'faq', status: 'published',
    summary: 'Common questions', page_metadata: { status: 'Published', priority: 'Low', owner: 'Alice', confidential: true } },
  { id: '4', slug: 'roadmap', title: 'Product Roadmap', page_type: 'doc', status: 'archived',
    summary: 'Q3/Q4 plans', page_metadata: { status: 'Archived', priority: 'Critical' } },
]

test('filterText matches title (case-insensitive)', () => {
  const r = applyView(samplePages, { filterText: 'getting', filterType: '', sortKey: 'title', sortDir: 'asc' })
  assert.equal(r.length, 1)
  assert.equal(r[0].slug, 'getting-started')
})

test('filterText matches summary', () => {
  const r = applyView(samplePages, { filterText: 'gRPC', filterType: '', sortKey: 'title', sortDir: 'asc' })
  assert.equal(r.length, 1)
  assert.equal(r[0].slug, 'api-reference')
})

test('filterText matches multi-select tag values', () => {
  const r = applyView(samplePages, { filterText: 'rest', filterType: '', sortKey: 'title', sortDir: 'asc' })
  assert.equal(r.length, 1)
  assert.equal(r[0].slug, 'api-reference')
})

test('filterText matches owner property value', () => {
  const r = applyView(samplePages, { filterText: 'alice', filterType: '', sortKey: 'title', sortDir: 'asc' })
  assert.equal(r.length, 2)
})

test('filterType narrows by page_type', () => {
  const r = applyView(samplePages, { filterText: '', filterType: 'doc', sortKey: 'title', sortDir: 'asc' })
  assert.equal(r.length, 2)
  assert.ok(r.every((p) => p.page_type === 'doc'))
})

test('empty filterText returns all pages', () => {
  const r = applyView(samplePages, { filterText: '', filterType: '', sortKey: 'title', sortDir: 'asc' })
  assert.equal(r.length, samplePages.length)
})

// --- sort logic ---

test('sort by title asc orders alphabetically', () => {
  const r = applyView(samplePages, { filterText: '', filterType: '', sortKey: 'title', sortDir: 'asc' })
  assert.equal(r[0].slug, 'api-reference')
  assert.equal(r[3].slug, 'roadmap')
})

test('sort by title desc reverses order', () => {
  const r = applyView(samplePages, { filterText: '', filterType: '', sortKey: 'title', sortDir: 'desc' })
  assert.equal(r[0].slug, 'roadmap')
  assert.equal(r[3].slug, 'api-reference')
})

test('sort by priority puts nulls last (asc)', () => {
  // Sort by string; nulls always last regardless of direction.
  // Roadmap has Critical but is NOT null — orphan (no priority property) is null.
  const withNull = [...samplePages, {
    id: '5', slug: 'orphan', title: 'Orphan', page_type: 'guide', status: 'draft',
    summary: '', page_metadata: {},
  }]
  const r = applyView(withNull, { filterText: '', filterType: '', sortKey: 'priority', sortDir: 'asc' })
  assert.equal(r[r.length - 1].slug, 'orphan')
  // All non-null entries appear before the null entry
  assert.ok(r.slice(0, -1).every((p) => p.page_metadata && p.page_metadata.priority !== undefined))
})

test('sort by priority puts nulls last regardless of direction', () => {
  // Construct a page without priority property
  const withNull = [...samplePages, {
    id: '5', slug: 'orphan', title: 'Orphan', page_type: 'guide', status: 'draft',
    summary: '', page_metadata: {},
  }]
  const asc = applyView(withNull, { filterText: '', filterType: '', sortKey: 'priority', sortDir: 'asc' })
  const desc = applyView(withNull, { filterText: '', filterType: '', sortKey: 'priority', sortDir: 'desc' })
  // In both asc and desc, the orphan (null) is at the end
  assert.equal(asc[asc.length - 1].slug, 'orphan')
  assert.equal(desc[desc.length - 1].slug, 'orphan')
})

test('sort by status chip orders by string compare', () => {
  const r = applyView(samplePages, { filterText: '', filterType: '', sortKey: 'status', sortDir: 'asc' })
  // archived < draft < published (alphabetical)
  assert.equal(r[0].status, 'archived')
})

// --- getCellValue ---

test('getCellValue returns null when property absent', () => {
  const page = samplePages[0]
  assert.equal(getCellValue(page, 'due_date'), null)
})

test('getCellValue returns coerced value when present', () => {
  const page = samplePages[0]
  assert.equal(getCellValue(page, 'priority'), 'High')
})

test('getCellValue for confidential checkbox returns boolean', () => {
  assert.equal(getCellValue(samplePages[2], 'confidential'), true)
  assert.equal(getCellValue(samplePages[0], 'confidential'), null)
})

// --- combined ---

test('combined filter + sort produces expected order', () => {
  // Alice owns 1 and 3; sort by page_type asc gives 'faq' (type=faq) first.
  const r = applyView(samplePages, {
    filterText: 'alice', filterType: '', sortKey: 'page_type', sortDir: 'asc',
  })
  assert.equal(r.length, 2)
  assert.equal(r[0].slug, 'faq')           // page_type 'faq' < 'guide' alphabetically
  assert.equal(r[1].slug, 'getting-started')
})
