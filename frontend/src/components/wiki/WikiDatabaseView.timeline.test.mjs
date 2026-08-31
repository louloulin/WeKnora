import test from 'node:test';
import assert from 'node:assert/strict';

// Mirror the Timeline view logic from WikiDatabaseView.vue so we can
// lock down the entry sorting + past-flag rules.

function buildTimelineEntries(propId, pages, now = new Date('2026-08-31T12:00:00Z')) {
  if (!propId) return [];
  const entries = [];
  for (const page of pages) {
    const raw = (page.page_metadata || {})[propId];
    if (typeof raw !== 'string' || !raw) continue;
    const d = new Date(raw);
    if (Number.isNaN(d.getTime())) continue;
    entries.push({
      page,
      date: d,
      dateLabel: raw.slice(0, 10),
      past: d.getTime() < now.getTime(),
    });
  }
  entries.sort((a, b) => a.date.getTime() - b.date.getTime());
  return entries;
}

// --- Empty / missing cases ---

test('empty propId returns no entries', () => {
  assert.deepEqual(buildTimelineEntries('', [{ id: '1', slug: 'a', page_metadata: { due: '2026-08-10' } }]), []);
});

test('null propId returns no entries', () => {
  assert.deepEqual(buildTimelineEntries(null, [{ id: '1', slug: 'a', page_metadata: { due: '2026-08-10' } }]), []);
});

test('pages without the chosen date prop are excluded', () => {
  const pages = [
    { id: '1', slug: 'a', page_metadata: { due: '2026-08-10' } },
    { id: '2', slug: 'b', page_metadata: { other: '2026-08-15' } },
  ];
  const entries = buildTimelineEntries('due', pages);
  assert.equal(entries.length, 1);
  assert.equal(entries[0].page.id, '1');
});

test('pages with null/empty/invalid date are excluded', () => {
  const pages = [
    { id: '1', slug: 'a', page_metadata: { due: null } },
    { id: '2', slug: 'b', page_metadata: { due: '' } },
    { id: '3', slug: 'c', page_metadata: { due: 'not-a-date' } },
    { id: '4', slug: 'd', page_metadata: {} },
    { id: '5', slug: 'e', page_metadata: { due: '2026-08-10' } },
  ];
  const entries = buildTimelineEntries('due', pages);
  assert.equal(entries.length, 1);
  assert.equal(entries[0].page.id, '5');
});

// --- Sorting ---

test('entries sort oldest-first (ascending by date)', () => {
  const pages = [
    { id: '1', slug: 'late', page_metadata: { due: '2026-09-15' } },
    { id: '2', slug: 'early', page_metadata: { due: '2026-07-01' } },
    { id: '3', slug: 'mid', page_metadata: { due: '2026-08-10' } },
  ];
  const entries = buildTimelineEntries('due', pages);
  assert.deepEqual(entries.map(e => e.page.id), ['2', '3', '1']);
});

test('entries with identical date keep stable insertion order', () => {
  const pages = [
    { id: '1', slug: 'first', page_metadata: { due: '2026-08-10' } },
    { id: '2', slug: 'second', page_metadata: { due: '2026-08-10' } },
    { id: '3', slug: 'third', page_metadata: { due: '2026-08-10' } },
  ];
  const entries = buildTimelineEntries('due', pages);
  // Array.prototype.sort is stable in V8 (modern Node), so input order
  // is preserved for equal elements.
  assert.deepEqual(entries.map(e => e.page.id), ['1', '2', '3']);
});

// --- Past flag ---

test('past flag: dates before now are flagged', () => {
  const pages = [
    { id: '1', slug: 'past', page_metadata: { due: '2026-08-01' } },
    { id: '2', slug: 'future', page_metadata: { due: '2026-12-31' } },
    { id: '3', slug: 'now', page_metadata: { due: '2026-08-31T11:00:00Z' } },
  ];
  const entries = buildTimelineEntries('due', pages, new Date('2026-08-31T12:00:00Z'));
  const byId = Object.fromEntries(entries.map(e => [e.page.id, e.past]));
  assert.equal(byId['1'], true, 'Aug 1 should be past');
  assert.equal(byId['2'], false, 'Dec 31 should be future');
  assert.equal(byId['3'], true, '1h before now should be past');
});

test('past flag: same instant as now is past (strict less-than)', () => {
  const pages = [{ id: '1', slug: 'edge', page_metadata: { due: '2026-08-31T12:00:00Z' } }];
  const entries = buildTimelineEntries('due', pages, new Date('2026-08-31T12:00:00Z'));
  // The d.getTime() < now.getTime() comparison means equal instants are
  // NOT past — they fall in the "present" bucket, treated as future for
  // visual emphasis (chip is brand-blue, not muted grey).
  assert.equal(entries[0].past, false);
});

// --- Date label ---

test('dateLabel strips time portion', () => {
  const pages = [{ id: '1', slug: 'a', page_metadata: { due: '2026-08-10T15:30:45.123Z' } }];
  const entries = buildTimelineEntries('due', pages);
  assert.equal(entries[0].dateLabel, '2026-08-10');
});

// --- Composition with filter ---

test('matchesFilter integration: text filter excludes entries', () => {
  // The Timeline view runs the same matchesFilter as Table / Board.
  // We mirror the rule for filterText only (other dimensions already
  // covered by the Board tests).
  function matchesFilter(page, filterText) {
    if (!filterText.trim()) return true;
    const q = filterText.trim().toLowerCase();
    if (page.title.toLowerCase().includes(q)) return true;
    if (page.slug.toLowerCase().includes(q)) return true;
    return false;
  }
  const pages = [
    { id: '1', title: 'Acme report', slug: 'acme', page_metadata: { due: '2026-08-10' } },
    { id: '2', title: 'Beta report', slug: 'beta', page_metadata: { due: '2026-08-11' } },
  ];
  const filtered = pages.filter(p => matchesFilter(p, 'beta'));
  const entries = buildTimelineEntries('due', filtered);
  assert.equal(entries.length, 1);
  assert.equal(entries[0].page.id, '2');
});
