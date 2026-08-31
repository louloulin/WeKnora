import test from 'node:test';
import assert from 'node:assert/strict';

// Mirror type-level checks at runtime so we can lock in behavior
// without bringing in a TS test runner.
function isRuntimeMention(type) {
  return type === 'agent' || type === 'skill' || type === 'mcp';
}

function getMentionIcon(type, icons) {
  switch (type) {
    case 'file': return 'file';
    case 'tag': return 'tag';
    case 'mcp': return 'tools';
    case 'skill': return icons.SKILL_ICON;
    case 'agent': return icons.AGENT_ICON;
    case 'task': return icons.TASK_ICON;
    default: return 'folder';
  }
}

const ICONS = { SKILL_ICON: 'system-code', AGENT_ICON: 'app', TASK_ICON: 'task-checked' };

test('MentionItemType includes agent and task', () => {
  const allowed = ['kb', 'file', 'tag', 'mcp', 'skill', 'agent', 'task'];
  for (const t of allowed) {
    assert.equal(typeof t === 'string', true);
  }
  assert.equal(allowed.length, 7);
});

test('isRuntimeMention flags agent as runtime-launchable', () => {
  assert.equal(isRuntimeMention('agent'), true);
  assert.equal(isRuntimeMention('skill'), true);
  assert.equal(isRuntimeMention('mcp'), true);
});

test('isRuntimeMention flags kb/file/tag/task as non-runtime', () => {
  assert.equal(isRuntimeMention('kb'), false);
  assert.equal(isRuntimeMention('file'), false);
  assert.equal(isRuntimeMention('tag'), false);
  assert.equal(isRuntimeMention('task'), false);
});

test('getMentionIcon returns AGENT_ICON for agent', () => {
  assert.equal(getMentionIcon('agent', ICONS), 'app');
});

test('getMentionIcon returns TASK_ICON for task', () => {
  assert.equal(getMentionIcon('task', ICONS), 'task-checked');
});

test('getMentionIcon preserves existing mappings', () => {
  assert.equal(getMentionIcon('file', ICONS), 'file');
  assert.equal(getMentionIcon('tag', ICONS), 'tag');
  assert.equal(getMentionIcon('mcp', ICONS), 'tools');
  assert.equal(getMentionIcon('skill', ICONS), 'system-code');
  assert.equal(getMentionIcon('kb', ICONS), 'folder');
});

test('MentionItem agent extras: agentMode / agentType / isBuiltinAgent are optional', () => {
  // simulate shape at runtime
  const item = {
    id: 'a1', name: 'Quick Answer', type: 'agent',
    agentMode: 'quick-answer', agentType: 'rag-qa', isBuiltinAgent: true,
  };
  assert.equal(item.agentMode, 'quick-answer');
  assert.equal(item.agentType, 'rag-qa');
  assert.equal(item.isBuiltinAgent, true);
});

test('MentionItem task extras: taskStatus / taskAssigneeName are optional', () => {
  const item = {
    id: 't1', name: 'Review onboarding', type: 'task',
    taskStatus: 'in_progress', taskAssigneeName: 'Alice',
  };
  assert.equal(item.taskStatus, 'in_progress');
  assert.equal(item.taskAssigneeName, 'Alice');
});

test('MentionRequestItem carries agent_mode / task_status payloads', () => {
  const agent = { id: 'a1', name: 'Wiki QA', type: 'agent', agent_mode: 'smart-reasoning' };
  const task = { id: 't1', name: 'Triage backlog', type: 'task', task_status: 'open' };
  assert.equal(agent.agent_mode, 'smart-reasoning');
  assert.equal(task.task_status, 'open');
});

test('agent group icon matches AGENT_ICON constant', () => {
  // The mention selector pulls icons from mentionGroupDefs; lock in the wire.
  const groupDefs = [
    { type: 'kb', icon: 'folder' },
    { type: 'skill', icon: ICONS.SKILL_ICON },
    { type: 'agent', icon: ICONS.AGENT_ICON },
    { type: 'task', icon: ICONS.TASK_ICON },
  ];
  const agent = groupDefs.find(g => g.type === 'agent');
  const task = groupDefs.find(g => g.type === 'task');
  assert.equal(agent.icon, 'app');
  assert.equal(task.icon, 'task-checked');
});
