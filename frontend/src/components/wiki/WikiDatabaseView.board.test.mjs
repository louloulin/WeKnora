import test from 'node:test';
import assert from 'node:assert/strict';

// Mirror the Board view logic from WikiDatabaseView.vue so we can lock
// down grouping semantics without spinning up a Vue runtime.

function groupByProperty(pages, propId, options) {
  const buckets = {};
  for (const opt of options) buckets[opt] = [];
  buckets.__ungrouped__ = [];
  for (const page of pages) {
    const raw = (page.page_metadata || {})[propId];
    const keys = [];
    if (Array.isArray(raw)) keys.push(...raw.filter(x => typeof x === 'string'));
    else if (typeof raw === 'string' && raw) keys.push(raw);
    if (keys.length === 0) {
      buckets.__ungrouped__.push(page);
    } else {
      const seen = new Set();
      for (const k of keys) {
        if (!(k in buckets) || seen.has(k)) continue;
        seen.add(k);
        buckets[k].push(page);
      }
    }
  }
  const cols = options.map(opt => ({ key: opt, label: opt, pages: buckets[opt] }));
  if (buckets.__ungrouped__.length > 0) {
    cols.push({ key: '__ungrouped__', label: 'Ungrouped', pages: buckets.__ungrouped__ });
  }
  return cols;
}

function formatRelativeTime(value, now = Date.now()) {
  if (!value) return '';
  const ts = new Date(value).getTime();
  if (Number.isNaN(ts)) return '';
  const diff = now - ts;
  if (diff < 60_000) return 'just now';
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h`;
  if (diff < 30 * 86_400_000) return `${Math.floor(diff / 86_400_000)}d`;
  return new Date(value).toISOString().slice(0, 10);
}

const pages = [
  { id: '1', title: 'A', slug: 'a', page_metadata: { status: 'Draft' } },
  { id: '2', title: 'B', slug: 'b', page_metadata: { status: 'Review' } },
  { id: '3', title: 'C', slug: 'c', page_metadata: { status: 'Draft' } },
  { id: '4', title: 'D', slug: 'd', page_metadata: { status: 'Archived' } },
  { id: '5', title: 'E', slug: 'e', page_metadata: {} },
];

test('groupByProperty distributes pages into declared option buckets', () => {
  const cols = groupByProperty(pages, 'status', ['Draft', 'Review', 'Published', 'Archived']);
  const byKey = Object.fromEntries(cols.map(c => [c.key, c.pages.length]));
  assert.equal(byKey.Draft, 2);
  assert.equal(byKey.Review, 1);
  assert.equal(byKey.Published, 0);
  assert.equal(byKey.Archived, 1);
  assert.equal(byKey.__ungrouped__, 1);
});

test('groupByProperty preserves declared order', () => {
  const cols = groupByProperty(pages, 'status', ['Draft', 'Review', 'Published', 'Archived']);
  const keys = cols.map(c => c.key);
  assert.deepEqual(keys, ['Draft', 'Review', 'Published', 'Archived', '__ungrouped__']);
});

test('groupByProperty omits ungrouped column when no pages are ungrouped', () => {
  const all = [
    { id: '1', title: 'A', slug: 'a', page_metadata: { status: 'Draft' } },
  ];
  const cols = groupByProperty(all, 'status', ['Draft', 'Review']);
  assert.equal(cols.length, 2);
  assert.equal(cols.find(c => c.key === '__ungrouped__'), undefined);
});

test('groupByProperty deduplicates a page appearing once per option', () => {
  const dup = [{ id: 'x', title: 'X', slug: 'x', page_metadata: { tag: ['Draft', 'Draft', 'Review'] } }];
  const cols = groupByProperty(dup, 'tag', ['Draft', 'Review']);
  assert.equal(cols.find(c => c.key === 'Draft').pages.length, 1);
  assert.equal(cols.find(c => c.key === 'Review').pages.length, 1);
});

test('groupByProperty handles multi-select by emitting the page into every key', () => {
  const multi = [{ id: 'm', title: 'M', slug: 'm', page_metadata: { tag: ['Draft', 'Review'] } }];
  const cols = groupByProperty(multi, 'tag', ['Draft', 'Review', 'Published']);
  assert.equal(cols.find(c => c.key === 'Draft').pages.length, 1);
  assert.equal(cols.find(c => c.key === 'Review').pages.length, 1);
  assert.equal(cols.find(c => c.key === 'Published').pages.length, 0);
});

test('groupByProperty treats missing metadata as ungrouped', () => {
  const cols = groupByProperty(pages, 'missing', ['A', 'B']);
  assert.equal(cols.find(c => c.key === '__ungrouped__').pages.length, 5);
});

test('groupByProperty treats null metadata as ungrouped', () => {
  const list = [
    { id: '1', title: 'A', slug: 'a', page_metadata: { status: null } },
    { id: '2', title: 'B', slug: 'b', page_metadata: { status: 'Draft' } },
  ];
  const cols = groupByProperty(list, 'status', ['Draft']);
  assert.equal(cols.find(c => c.key === 'Draft').pages.length, 1);
  assert.equal(cols.find(c => c.key === '__ungrouped__').pages.length, 1);
});

test('groupByProperty returns empty columns when input is empty', () => {
  const cols = groupByProperty([], 'status', ['Draft', 'Review']);
  assert.equal(cols.length, 2);
  for (const c of cols) assert.equal(c.pages.length, 0);
});

test('formatRelativeTime: just now under 1 minute', () => {
  const now = 1_700_000_000_000;
  assert.equal(formatRelativeTime(new Date(now - 30_000).toISOString(), now), 'just now');
});

test('formatRelativeTime: minutes under 1 hour', () => {
  const now = 1_700_000_000_000;
  assert.equal(formatRelativeTime(new Date(now - 5 * 60_000).toISOString(), now), '5m');
});

test('formatRelativeTime: hours under 1 day', () => {
  const now = 1_700_000_000_000;
  assert.equal(formatRelativeTime(new Date(now - 3 * 3_600_000).toISOString(), now), '3h');
});

test('formatRelativeTime: days under 30 days', () => {
  const now = 1_700_000_000_000;
  assert.equal(formatRelativeTime(new Date(now - 5 * 86_400_000).toISOString(), now), '5d');
});

test('formatRelativeTime: ISO date after 30+ days', () => {
  const now = 1_700_000_000_000;
  const stamp = new Date(now - 60 * 86_400_000).toISOString();
  assert.equal(formatRelativeTime(stamp, now), stamp.slice(0, 10));
});

test('formatRelativeTime: empty / null / invalid inputs return empty string', () => {
  assert.equal(formatRelativeTime(null), '');
  assert.equal(formatRelativeTime(undefined), '');
  assert.equal(formatRelativeTime(''), '');
  assert.equal(formatRelativeTime('not-a-date'), '');
});
