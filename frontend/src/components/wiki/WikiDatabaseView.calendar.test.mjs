import test from 'node:test';
import assert from 'node:assert/strict';

// Mirror the Calendar view logic from WikiDatabaseView.vue so we can
// lock down grid generation + bucketing without spinning up Vue.

function ymd(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function buildCalendarCells(cursor, propId, pages) {
  const year = cursor.getFullYear();
  const month = cursor.getMonth();
  const first = new Date(year, month, 1);
  const offset = (first.getDay() + 6) % 7; // Monday = 0
  const gridStart = new Date(year, month, 1 - offset);
  const cells = [];
  const buckets = {};
  for (const p of pages) {
    const raw = (p.page_metadata || {})[propId];
    if (typeof raw !== 'string' || !raw) continue;
    const key = raw.slice(0, 10);
    if (!buckets[key]) buckets[key] = [];
    buckets[key].push(p);
  }
  for (let i = 0; i < 42; i++) {
    const d = new Date(gridStart);
    d.setDate(gridStart.getDate() + i);
    cells.push({
      key: ymd(d),
      day: d.getDate(),
      outside: d.getMonth() !== month,
      pages: buckets[ymd(d)] || [],
    });
  }
  return cells;
}

function monthLabel(cursor) {
  return `${cursor.getFullYear()}-${String(cursor.getMonth() + 1).padStart(2, '0')}`;
}

function shiftMonth(cursor, delta) {
  const next = new Date(cursor);
  next.setMonth(next.getMonth() + delta);
  return next;
}

// --- Grid generation ---

test('grid generates exactly 42 cells (6 weeks × 7 days)', () => {
  const cells = buildCalendarCells(new Date(2026, 7, 15), 'due_date', []);
  assert.equal(cells.length, 42);
});

test('grid starts on Monday (ISO week)', () => {
  // 2026-08-01 is a Saturday → first Monday of the grid is 2026-07-27.
  const cells = buildCalendarCells(new Date(2026, 7, 15), 'due_date', []);
  assert.equal(cells[0].key, '2026-07-27');
  assert.equal(cells[0].day, 27);
});

test('cells outside the cursor month are flagged', () => {
  const cells = buildCalendarCells(new Date(2026, 7, 15), 'due_date', []);
  // The first 5 cells are July (offset 5 to reach Monday), so they're outside.
  for (let i = 0; i < 5; i++) {
    assert.equal(cells[i].outside, true, `cell ${i} should be outside`);
  }
  // 2026-08-01 (Saturday) is the first day in August.
  assert.equal(cells[5].key, '2026-08-01');
  assert.equal(cells[5].outside, false);
});

// --- Bucketing ---

test('pages bucket by yyyy-mm-dd prefix', () => {
  const pages = [
    { id: '1', slug: 'a', page_metadata: { due_date: '2026-08-10T09:00:00Z' } },
    { id: '2', slug: 'b', page_metadata: { due_date: '2026-08-10T15:30:00Z' } },
    { id: '3', slug: 'c', page_metadata: { due_date: '2026-08-15T00:00:00Z' } },
  ];
  const cells = buildCalendarCells(new Date(2026, 7, 15), 'due_date', pages);
  const aug10 = cells.find(c => c.key === '2026-08-10');
  const aug15 = cells.find(c => c.key === '2026-08-15');
  assert.equal(aug10.pages.length, 2);
  assert.equal(aug15.pages.length, 1);
});

test('pages without a date are excluded', () => {
  const pages = [
    { id: '1', slug: 'a', page_metadata: { due_date: '2026-08-10' } },
    { id: '2', slug: 'b', page_metadata: {} },
    { id: '3', slug: 'c', page_metadata: { due_date: null } },
    { id: '4', slug: 'd', page_metadata: { due_date: '' } },
  ];
  const cells = buildCalendarCells(new Date(2026, 7, 15), 'due_date', pages);
  const total = cells.reduce((sum, c) => sum + c.pages.length, 0);
  assert.equal(total, 1, 'only the dated page should land in a cell');
});

test('empty propId skips bucketing entirely', () => {
  const pages = [{ id: '1', slug: 'a', page_metadata: { due_date: '2026-08-10' } }];
  const cells = buildCalendarCells(new Date(2026, 7, 15), '', pages);
  assert.equal(cells.every(c => c.pages.length === 0), true);
});

// --- Month label / shift ---

test('monthLabel formats yyyy-mm', () => {
  assert.equal(monthLabel(new Date(2026, 0, 15)), '2026-01');
  assert.equal(monthLabel(new Date(2026, 11, 31)), '2026-12');
});

test('shiftMonth(+1) crosses year boundary', () => {
  const next = shiftMonth(new Date(2026, 11, 15), 1);
  assert.equal(next.getFullYear(), 2027);
  assert.equal(next.getMonth(), 0);
});

test('shiftMonth(-1) crosses year boundary backwards', () => {
  const next = shiftMonth(new Date(2026, 0, 15), -1);
  assert.equal(next.getFullYear(), 2025);
  assert.equal(next.getMonth(), 11);
});

test('shiftMonth(0) is a no-op', () => {
  const orig = new Date(2026, 7, 15);
  const next = shiftMonth(orig, 0);
  assert.equal(next.getFullYear(), 2026);
  assert.equal(next.getMonth(), 7);
});

// --- "more" indicator (truncated for >3 cards) ---

test('cell with 5 pages renders "+2 more" hint', () => {
  const pages = Array.from({ length: 5 }, (_, i) => ({
    id: String(i), slug: `s${i}`, page_metadata: { due_date: '2026-08-10' }
  }));
  const cells = buildCalendarCells(new Date(2026, 7, 15), 'due_date', pages);
  const cell = cells.find(c => c.key === '2026-08-10');
  assert.equal(cell.pages.length, 5);
  // Truncation logic mirrors the template: pages.slice(0, 3)
  const visible = cell.pages.slice(0, 3);
  const overflow = cell.pages.length - 3;
  assert.equal(visible.length, 3);
  assert.equal(overflow, 2);
});
