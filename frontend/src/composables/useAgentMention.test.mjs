import test from 'node:test';
import assert from 'node:assert/strict';

// Mirror the runtime logic from useAgentMention.ts so we can lock the
// transformation from CustomAgent -> MentionItem without spinning up Vue.

function toAgentMentionItem(agent) {
  const mode = agent?.config?.agent_mode;
  const type = agent?.config?.agent_type;
  const builtin = Boolean(agent?.is_builtin);
  return {
    id: String(agent.id),
    name: agent.name || String(agent.id),
    type: 'agent',
    description: agent.description || '',
    agentMode: mode,
    agentType: type,
    isBuiltinAgent: builtin,
  };
}

function search(items, query) {
  const q = (query || '').trim().toLowerCase();
  if (!q) return items;
  return items.filter((item) => {
    const haystack = [item.name, item.description || '', item.agentType || '', item.agentMode || '']
      .join(' ')
      .toLowerCase();
    return haystack.includes(q);
  });
}

test('toAgentMentionItem maps builtin quick-answer', () => {
  const item = toAgentMentionItem({
    id: 'bqa',
    name: 'Quick Answer',
    is_builtin: true,
    config: { agent_mode: 'quick-answer', agent_type: 'rag-qa' },
  });
  assert.equal(item.id, 'bqa');
  assert.equal(item.name, 'Quick Answer');
  assert.equal(item.type, 'agent');
  assert.equal(item.isBuiltinAgent, true);
  assert.equal(item.agentMode, 'quick-answer');
  assert.equal(item.agentType, 'rag-qa');
});

test('toAgentMentionItem maps custom smart-reasoning', () => {
  const item = toAgentMentionItem({
    id: 'c1',
    name: 'Wiki QA',
    description: 'Custom wiki agent',
    is_builtin: false,
    config: { agent_mode: 'smart-reasoning', agent_type: 'wiki-qa' },
  });
  assert.equal(item.isBuiltinAgent, false);
  assert.equal(item.agentMode, 'smart-reasoning');
  assert.equal(item.agentType, 'wiki-qa');
  assert.equal(item.description, 'Custom wiki agent');
});

test('toAgentMentionItem falls back to id when name is missing', () => {
  const item = toAgentMentionItem({
    id: '42',
    is_builtin: false,
    config: {},
  });
  assert.equal(item.name, '42');
  assert.equal(item.agentMode, undefined);
});

test('search matches name case-insensitively', () => {
  const items = [
    toAgentMentionItem({ id: '1', name: 'Quick Answer', is_builtin: true, config: { agent_mode: 'quick-answer' } }),
    toAgentMentionItem({ id: '2', name: 'Wiki QA', is_builtin: false, config: { agent_mode: 'smart-reasoning' } }),
  ];
  const matches = search(items, 'wiki');
  assert.equal(matches.length, 1);
  assert.equal(matches[0].name, 'Wiki QA');
});

test('search matches description text', () => {
  const items = [
    toAgentMentionItem({ id: '1', name: 'QA', description: 'specialized triage agent', is_builtin: false, config: {} }),
  ];
  const matches = search(items, 'triage');
  assert.equal(matches.length, 1);
});

test('search matches agentType and agentMode', () => {
  const items = [
    toAgentMentionItem({ id: '1', name: 'A', is_builtin: true, config: { agent_mode: 'quick-answer', agent_type: 'rag-qa' } }),
    toAgentMentionItem({ id: '2', name: 'B', is_builtin: false, config: { agent_mode: 'smart-reasoning', agent_type: 'wiki-qa' } }),
  ];
  assert.equal(search(items, 'wiki').length, 1);
  assert.equal(search(items, 'smart').length, 1);
});

test('search with empty query returns all items', () => {
  const items = [
    toAgentMentionItem({ id: '1', name: 'A', is_builtin: true, config: {} }),
    toAgentMentionItem({ id: '2', name: 'B', is_builtin: false, config: {} }),
  ];
  assert.equal(search(items, '').length, 2);
  assert.equal(search(items, '   ').length, 2);
  assert.equal(search(items).length, 2);
});

test('search returns empty for unmatched query', () => {
  const items = [
    toAgentMentionItem({ id: '1', name: 'Quick Answer', is_builtin: true, config: {} }),
  ];
  assert.equal(search(items, 'nonexistent').length, 0);
});
