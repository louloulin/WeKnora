import test from 'node:test';
import assert from 'node:assert/strict';

// Mirror the schema logic from wikiPropertySchema.ts so we can lock in
// behavior without bringing in the Vue / TS runtime.

const PROPERTY_TYPES = ['text', 'number', 'date', 'select', 'multi-select', 'checkbox', 'url', 'person', 'status', 'relation', 'rollup', 'formula', 'email'];

const PROPERTY_TYPE_LABELS = {
  text: '文本', number: '数字', date: '日期', select: '单选',
  'multi-select': '多选', checkbox: '开关', url: '链接',
  person: '人员', status: '状态', relation: '关联',
  rollup: '汇总', formula: '公式', email: '邮箱',
};

function isReadOnlyPropertyType(type) {
  return type === 'rollup' || type === 'formula';
}

function coerce(property, raw) {
  if (raw === null || raw === undefined) return null;
  switch (property.type) {
    case 'text': return typeof raw === 'string' ? raw : String(raw);
    case 'number': {
      const n = typeof raw === 'number' ? raw : Number(raw);
      return Number.isFinite(n) ? n : null;
    }
    case 'date': return typeof raw === 'string' && raw.length > 0 ? raw : null;
    case 'select':
      if (typeof raw !== 'string') return null;
      if (!property.options || property.options.length === 0) return raw;
      return property.options.includes(raw) ? raw : null;
    case 'multi-select': {
      if (!Array.isArray(raw)) return null;
      const arr = raw.filter(v => typeof v === 'string');
      if (!property.options || property.options.length === 0) return arr;
      return arr.filter(v => property.options.includes(v));
    }
    case 'checkbox': return typeof raw === 'boolean' ? raw : null;
    case 'url': return typeof raw === 'string' && /^https?:\/\//i.test(raw) ? raw : null;
    case 'email':
      return typeof raw === 'string' && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(raw) ? raw : null;
    case 'person':
      return typeof raw === 'string' && raw.length > 0 ? raw : null;
    case 'status':
      if (typeof raw !== 'string') return null;
      if (!property.options || property.options.length === 0) return raw;
      return property.options.includes(raw) ? raw : null;
    case 'relation': {
      if (!Array.isArray(raw)) return null;
      const slugs = raw.filter(v => typeof v === 'string' && v.length > 0);
      if (slugs.length === 0) return null;
      return Array.from(new Set(slugs));
    }
    case 'rollup':
    case 'formula':
      return raw;
    default: return null;
  }
}

function format(property, value) {
  if (value === null || value === undefined) return '';
  switch (property.type) {
    case 'checkbox': return value ? '✓' : '—';
    case 'multi-select': return Array.isArray(value) ? value.join(', ') : '';
    case 'date': case 'text': case 'select': case 'url': case 'number':
    case 'email': case 'person': return String(value);
    case 'status': return String(value);
    case 'relation': {
      if (!Array.isArray(value)) return '';
      if (value.length === 0) return '';
      if (value.length === 1) return String(value[0]);
      return `${value.length} pages`;
    }
    case 'rollup': case 'formula': {
      if (typeof value === 'number') return String(value);
      if (typeof value === 'boolean') return value ? '✓' : '—';
      if (Array.isArray(value)) return value.length === 1 ? String(value[0]) : `${value.length} items`;
      return String(value);
    }
    default: return '';
  }
}

const status = { id: 'workflow', name: '工作流', type: 'status', options: ['Backlog', 'Todo', 'Doing', 'Done'] };
const relation = { id: 'related', name: '关联页面', type: 'relation' };
const email = { id: 'mail', name: '邮箱', type: 'email' };
const person = { id: 'owner', name: '负责人', type: 'person' };
const rollup = { id: 'count', name: '关联数', type: 'rollup' };
const formula = { id: 'computed', name: '公式', type: 'formula' };

// --- PropertyType coverage ---

test('PROPERTY_TYPES includes 13 entries (was 7 before Sprint 1 §26)', () => {
  assert.equal(PROPERTY_TYPES.length, 13);
  assert.ok(PROPERTY_TYPES.includes('person'));
  assert.ok(PROPERTY_TYPES.includes('status'));
  assert.ok(PROPERTY_TYPES.includes('relation'));
  assert.ok(PROPERTY_TYPES.includes('rollup'));
  assert.ok(PROPERTY_TYPES.includes('formula'));
  assert.ok(PROPERTY_TYPES.includes('email'));
});

test('PROPERTY_TYPE_LABELS covers every type', () => {
  for (const t of PROPERTY_TYPES) {
    assert.ok(PROPERTY_TYPE_LABELS[t], `missing label for ${t}`);
    assert.notEqual(PROPERTY_TYPE_LABELS[t], '');
  }
});

// --- isReadOnlyPropertyType ---

test('rollup and formula are read-only', () => {
  assert.equal(isReadOnlyPropertyType('rollup'), true);
  assert.equal(isReadOnlyPropertyType('formula'), true);
});

test('everything else is editable', () => {
  for (const t of ['text', 'number', 'date', 'select', 'multi-select', 'checkbox', 'url', 'person', 'status', 'relation', 'email']) {
    assert.equal(isReadOnlyPropertyType(t), false, `${t} should be editable`);
  }
});

// --- coerce: person ---

test('coerce person: accepts non-empty string', () => {
  assert.equal(coerce(person, 'alice'), 'alice');
  assert.equal(coerce(person, 'user-123'), 'user-123');
});

test('coerce person: rejects empty string / wrong type', () => {
  assert.equal(coerce(person, ''), null);
  assert.equal(coerce(person, null), null);
  assert.equal(coerce(person, undefined), null);
  assert.equal(coerce(person, 42), null);
  assert.equal(coerce(person, { id: 'alice' }), null);
});

// --- coerce: status ---

test('coerce status: accepts declared option', () => {
  assert.equal(coerce(status, 'Todo'), 'Todo');
  assert.equal(coerce(status, 'Doing'), 'Doing');
});

test('coerce status: rejects undeclared option when options set', () => {
  assert.equal(coerce(status, 'NotInList'), null);
});

test('coerce status: free-form when no options declared', () => {
  const freeform = { id: 's', name: 'S', type: 'status' };
  assert.equal(coerce(freeform, 'Anything'), 'Anything');
});

test('coerce status: rejects non-string', () => {
  assert.equal(coerce(status, 42), null);
  assert.equal(coerce(status, ['Todo']), null);
});

// --- coerce: relation ---

test('coerce relation: accepts array of slugs', () => {
  assert.deepEqual(coerce(relation, ['a', 'b']), ['a', 'b']);
  assert.deepEqual(coerce(relation, ['only-one']), ['only-one']);
});

test('coerce relation: dedupes slugs', () => {
  assert.deepEqual(coerce(relation, ['a', 'b', 'a', 'b']), ['a', 'b']);
});

test('coerce relation: filters empty strings', () => {
  assert.deepEqual(coerce(relation, ['a', '', 'b']), ['a', 'b']);
});

test('coerce relation: returns null for empty / non-array', () => {
  assert.equal(coerce(relation, []), null);
  assert.equal(coerce(relation, 'a'), null);
  assert.equal(coerce(relation, null), null);
});

// --- coerce: email ---

test('coerce email: accepts valid address', () => {
  assert.equal(coerce(email, 'alice@example.com'), 'alice@example.com');
  assert.equal(coerce(email, 'x@y.z'), 'x@y.z');
});

test('coerce email: rejects malformed', () => {
  assert.equal(coerce(email, 'no-at-sign'), null);
  assert.equal(coerce(email, 'no@domain'), null);
  assert.equal(coerce(email, '@no-local.com'), null);
  assert.equal(coerce(email, 'spaces in@email.com'), null);
  assert.equal(coerce(email, ''), null);
  assert.equal(coerce(email, null), null);
  assert.equal(coerce(email, 42), null);
});

// --- coerce: rollup / formula ---

test('coerce rollup: pass-through (server is source of truth)', () => {
  assert.equal(coerce(rollup, 5), 5);
  assert.equal(coerce(rollup, 'count'), 'count');
  assert.deepEqual(coerce(rollup, [1, 2, 3]), [1, 2, 3]);
});

test('coerce formula: pass-through heterogeneous values', () => {
  assert.equal(coerce(formula, true), true);
  assert.equal(coerce(formula, null), null);
  assert.equal(coerce(formula, 3.14), 3.14);
  assert.deepEqual(coerce(formula, ['a', 'b']), ['a', 'b']);
});

// --- format ---

test('format email: shows the address as-is', () => {
  assert.equal(format(email, 'alice@example.com'), 'alice@example.com');
});

test('format person: shows the id (client resolves via user picker)', () => {
  assert.equal(format(person, 'alice'), 'alice');
});

test('format status: shows the option label', () => {
  assert.equal(format(status, 'Todo'), 'Todo');
});

test('format relation: single slug vs multiple', () => {
  assert.equal(format(relation, ['entity/acme']), 'entity/acme');
  assert.equal(format(relation, ['a', 'b', 'c']), '3 pages');
  assert.equal(format(relation, []), '');
});

test('format rollup: number / boolean / array', () => {
  assert.equal(format(rollup, 5), '5');
  assert.equal(format(rollup, true), '✓');
  assert.equal(format(rollup, false), '—');
  assert.equal(format(rollup, ['one']), 'one');
  assert.equal(format(rollup, ['a', 'b', 'c']), '3 items');
});

test('format formula: mirrors rollup rules', () => {
  assert.equal(format(formula, 0), '0');
  assert.equal(format(formula, false), '—');
  assert.equal(format(formula, ['x']), 'x');
});
