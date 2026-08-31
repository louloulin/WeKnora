// Unit test for AgentLibrary component logic.
//
// Mirrors the .vue file's filter / metadata-extraction logic so we can
// run it under Node without the Vue runtime. Locks in:
//   - filterText matches name / description / creator_name
//   - filterType scopes to a specific agent_type
//   - filterMode scopes to quick-answer vs smart-reasoning
//   - filterSource distinguishes builtin vs custom
//   - getKbLabel returns the right summary for all/none/one/many
//   - getToolCount reads from config.allowed_tools

import assert from 'node:assert/strict'
import test from 'node:test'

function truncate(s, n) {
  if (!s) return ''
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}

function getMode(agent) {
  return agent.config?.agent_mode || 'quick-answer'
}

function getTypeLabel(agent) {
  const type = agent.config?.agent_type || 'custom'
  return type
}

function getToolCount(agent) {
  return agent.config?.allowed_tools?.length || 0
}

function getKbLabel(agent, t) {
  const mode = agent.config?.kb_selection_mode
  if (!mode || mode === 'none') return ''
  const kbs = agent.config?.knowledge_bases || []
  if (mode === 'all') return t('metaKbAll')
  if (kbs.length === 0) return t('metaKbNone')
  if (kbs.length === 1) return t('metaKbOne')
  return t('metaKbMany', { count: kbs.length })
}

function matchesFilter(agent, f) {
  if (f.filterSource === 'builtin' && !agent.is_builtin) return false
  if (f.filterSource === 'custom' && agent.is_builtin) return false
  if (f.filterType && getTypeLabel(agent) !== f.filterType) return false
  if (f.filterMode && getMode(agent) !== f.filterMode) return false
  if (!f.filterText || !f.filterText.trim()) return true
  const q = f.filterText.trim().toLowerCase()
  if (agent.name.toLowerCase().includes(q)) return true
  if (agent.description && agent.description.toLowerCase().includes(q)) return true
  if (agent.creator_name && agent.creator_name.toLowerCase().includes(q)) return true
  return false
}

const sampleAgents = [
  {
    id: 'builtin-quick-answer',
    name: 'Quick Answer',
    description: 'Built-in classic RAG QA agent',
    is_builtin: true,
    creator_name: 'system',
    config: {
      agent_mode: 'quick-answer',
      agent_type: 'rag-qa',
      allowed_tools: [],
      kb_selection_mode: 'all',
    },
  },
  {
    id: 'builtin-smart-reasoning',
    name: 'Smart Reasoning',
    description: 'Built-in ReAct agent with tool calling',
    is_builtin: true,
    creator_name: 'system',
    config: {
      agent_mode: 'smart-reasoning',
      agent_type: 'hybrid-rag-wiki',
      allowed_tools: ['web_search', 'code_interpreter', 'calculator'],
      kb_selection_mode: 'all',
    },
  },
  {
    id: 'custom-1',
    name: 'HR Policy Helper',
    description: 'Answers questions about employee handbook',
    is_builtin: false,
    creator_name: 'Alice',
    config: {
      agent_mode: 'smart-reasoning',
      agent_type: 'wiki-qa',
      allowed_tools: ['web_search'],
      kb_selection_mode: 'selected',
      knowledge_bases: ['kb-1'],
    },
  },
  {
    id: 'custom-2',
    name: 'Data Analyst',
    description: 'Analyse CSV / XLSX files',
    is_builtin: false,
    creator_name: 'Bob',
    config: {
      agent_mode: 'smart-reasoning',
      agent_type: 'data-analysis',
      allowed_tools: ['code_interpreter', 'shell'],
      kb_selection_mode: 'none',
    },
  },
  {
    id: 'custom-3',
    name: 'Multi-KB Helper',
    description: 'Searches across multiple knowledge bases',
    is_builtin: false,
    creator_name: 'Alice',
    config: {
      agent_mode: 'quick-answer',
      agent_type: 'rag-qa',
      allowed_tools: [],
      kb_selection_mode: 'selected',
      knowledge_bases: ['kb-1', 'kb-2', 'kb-3'],
    },
  },
]

const tStub = (key, params) => {
  const labels = {
    metaKbAll: 'all KBs',
    metaKbNone: 'no KB',
    metaKbOne: '1 KB',
    metaKbMany: `${params?.count} KBs`,
  }
  return labels[key] || key
}

// --- truncate ---

test('truncate returns empty for empty input', () => {
  assert.equal(truncate('', 10), '')
  assert.equal(truncate(undefined, 10), '')
})

test('truncate short strings unchanged', () => {
  assert.equal(truncate('hello', 10), 'hello')
})

test('truncate long strings with ellipsis at n-1 chars', () => {
  const r = truncate('a'.repeat(20), 10)
  assert.equal(r.length, 10)
  assert.ok(r.endsWith('…'))
})

// --- getMode / getTypeLabel / getToolCount ---

test('getMode returns quick-answer by default', () => {
  assert.equal(getMode({ config: {} }), 'quick-answer')
  assert.equal(getMode({}), 'quick-answer')
})

test('getMode reads from config.agent_mode when present', () => {
  assert.equal(getMode({ config: { agent_mode: 'smart-reasoning' } }), 'smart-reasoning')
})

test('getTypeLabel defaults to custom', () => {
  assert.equal(getTypeLabel({ config: {} }), 'custom')
})

test('getToolCount reads allowed_tools length, defaults to 0', () => {
  assert.equal(getToolCount({ config: { allowed_tools: ['a', 'b'] } }), 2)
  assert.equal(getToolCount({ config: {} }), 0)
  assert.equal(getToolCount({}), 0)
})

// --- getKbLabel ---

test('getKbLabel returns empty when mode is none', () => {
  assert.equal(getKbLabel({ config: { kb_selection_mode: 'none' } }, tStub), '')
})

test('getKbLabel returns "all" label when mode is all', () => {
  assert.equal(getKbLabel({ config: { kb_selection_mode: 'all' } }, tStub), 'all KBs')
})

test('getKbLabel returns "no KB" when selected mode but empty list', () => {
  assert.equal(
    getKbLabel({ config: { kb_selection_mode: 'selected', knowledge_bases: [] } }, tStub),
    'no KB',
  )
})

test('getKbLabel returns "1 KB" when exactly one KB', () => {
  assert.equal(
    getKbLabel({ config: { kb_selection_mode: 'selected', knowledge_bases: ['kb-1'] } }, tStub),
    '1 KB',
  )
})

test('getKbLabel returns "N KBs" when multiple', () => {
  assert.equal(
    getKbLabel({ config: { kb_selection_mode: 'selected', knowledge_bases: ['a', 'b', 'c'] } }, tStub),
    '3 KBs',
  )
})

// --- matchesFilter ---

test('no filters returns all agents', () => {
  const r = sampleAgents.filter((a) => matchesFilter(a, {}))
  assert.equal(r.length, sampleAgents.length)
})

test('filterSource=builtin returns only built-ins', () => {
  const r = sampleAgents.filter((a) => matchesFilter(a, { filterSource: 'builtin' }))
  assert.equal(r.length, 2)
  assert.ok(r.every((a) => a.is_builtin))
})

test('filterSource=custom returns only non-built-ins', () => {
  const r = sampleAgents.filter((a) => matchesFilter(a, { filterSource: 'custom' }))
  assert.equal(r.length, 3)
  assert.ok(r.every((a) => !a.is_builtin))
})

test('filterType=wiki-qa returns only wiki agents', () => {
  const r = sampleAgents.filter((a) => matchesFilter(a, { filterType: 'wiki-qa' }))
  assert.equal(r.length, 1)
  assert.equal(r[0].id, 'custom-1')
})

test('filterMode=quick-answer returns both built-ins and custom quick-answer', () => {
  const r = sampleAgents.filter((a) => matchesFilter(a, { filterMode: 'quick-answer' }))
  assert.equal(r.length, 2)
})

test('filterMode=smart-reasoning returns the rest', () => {
  const r = sampleAgents.filter((a) => matchesFilter(a, { filterMode: 'smart-reasoning' }))
  assert.equal(r.length, 3)
})

test('filterText matches name case-insensitively', () => {
  // Two agents (custom-1 HR Policy Helper, custom-3 Multi-KB Helper) match.
  const r = sampleAgents.filter((a) => matchesFilter(a, { filterText: 'HELPER' }))
  assert.equal(r.length, 2)
  assert.ok(r.every((a) => a.name.toLowerCase().includes('helper')))
})

test('filterText matches description', () => {
  const r = sampleAgents.filter((a) => matchesFilter(a, { filterText: 'employee' }))
  assert.equal(r.length, 1)
})

test('filterText matches creator_name', () => {
  const r = sampleAgents.filter((a) => matchesFilter(a, { filterText: 'Bob' }))
  assert.equal(r.length, 1)
  assert.equal(r[0].id, 'custom-2')
})

test('combined filter: builtin + smart-reasoning returns only built-in smart', () => {
  const r = sampleAgents.filter((a) =>
    matchesFilter(a, { filterSource: 'builtin', filterMode: 'smart-reasoning' }),
  )
  assert.equal(r.length, 1)
  assert.equal(r[0].id, 'builtin-smart-reasoning')
})

test('combined filter: custom + data-analysis', () => {
  const r = sampleAgents.filter((a) =>
    matchesFilter(a, { filterSource: 'custom', filterType: 'data-analysis' }),
  )
  assert.equal(r.length, 1)
  assert.equal(r[0].id, 'custom-2')
})
