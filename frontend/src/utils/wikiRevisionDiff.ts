/**
 * Pure line-level LCS diff for wiki revision comparison (Build #9-B).
 *
 * Algorithm: classic Myers-style LCS dynamic programming. O(n*m) time
 * and memory — both inputs are split into lines first, so the
 * multiplier is on the line count, not character count. For our typical
 * wiki page (≤ 500 lines) this runs in single-digit ms.
 *
 * Output is a sequence of segments. Each segment is a list of lines from
 * one side only, plus an optional marker describing the kind:
 *   - `eq`: lines present on both sides, unchanged
 *   - `add`: lines present only on the new side (right)
 *   - `del`: lines present only on the old side (left)
 *
 * Segments are coalesced — a single `add` segment may contain many
 * consecutive added lines. Callers render these as side-by-side or
 * unified diffs.
 *
 * Long lines (> 120 chars) are truncated to keep the diff readable in
 * tight UI panels.
 */

export type DiffKind = 'eq' | 'add' | 'del'

export interface DiffSegment {
  kind: DiffKind
  /**
   * For `eq` and `del`: lines from the old side.
   * For `add`: undefined (no old-side lines).
   */
  oldLines?: string[]
  /**
   * For `eq` and `add`: lines from the new side.
   * For `del`: undefined (no new-side lines).
   */
  newLines?: string[]
}

export const DIFF_LONG_LINE_THRESHOLD = 120

/**
 * Split text into lines, keeping `\n` semantics. Empty trailing line
 * from a final `\n` is dropped to match common line counts.
 */
function splitLines(text: string): string[] {
  if (text.length === 0) return []
  const parts = text.split(/\r?\n/u)
  if (parts.length > 0 && parts[parts.length - 1] === '') parts.pop()
  return parts
}

/**
 * Truncate a single line for display in the diff panel. Long URLs / code
 * blocks / embedded base64 are common in wiki pages and break panel
 * layouts. We keep the leading 80 + 40 tail with an ellipsis, which
 * preserves enough context for human review.
 */
export function truncateLine(line: string): string {
  if (line.length <= DIFF_LONG_LINE_THRESHOLD) return line
  const head = line.slice(0, 80)
  const tail = line.slice(-40)
  return `${head}…${tail}`
}

/**
 * Build LCS dynamic-programming table.
 *   dp[i][j] = length of LCS of a[:i] and b[:j]
 */
function buildLcsTable(a: string[], b: string[]): Uint32Array {
  const cols = b.length + 1
  const dp = new Uint32Array((a.length + 1) * cols)
  for (let i = 1; i <= a.length; i++) {
    for (let j = 1; j <= b.length; j++) {
      const idx = i * cols + j
      if (a[i - 1] === b[j - 1]) {
        dp[idx] = (dp[(i - 1) * cols + (j - 1)] ?? 0) + 1
      } else {
        const up = dp[(i - 1) * cols + j] ?? 0
        const left = dp[i * cols + (j - 1)] ?? 0
        dp[idx] = up >= left ? up : left
      }
    }
  }
  return dp
}

/**
 * Walk the LCS table from bottom-right to top-left, emitting segments.
 * Each segment is a contiguous run of one kind (eq / add / del).
 */
function backtrackLcs(
  a: string[],
  b: string[],
  dp: Uint32Array,
  cols: number,
): DiffSegment[] {
  const segments: DiffSegment[] = []
  let i = a.length
  let j = b.length

  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && a[i - 1] === b[j - 1]) {
      pushSegment(segments, 'eq', a[i - 1], b[j - 1])
      i--
      j--
    } else if (j > 0 && (i === 0 || (dp[(i - 1) * cols + j] ?? 0) < (dp[i * cols + (j - 1)] ?? 0))) {
      pushSegment(segments, 'add', undefined, b[j - 1])
      j--
    } else {
      pushSegment(segments, 'del', a[i - 1], undefined)
      i--
    }
  }

  segments.reverse()
  return segments
}

/**
 * Append a line to the trailing segment of the same kind. If the last
 * segment is empty or different, create a new one. Prepending a line via
 * `unshift` is avoided by collecting runs in reverse and reversing once
 * at the end (in `diffLines`).
 */
function pushSegment(
  segments: DiffSegment[],
  kind: DiffKind,
  oldLine: string | undefined,
  newLine: string | undefined,
): void {
  const last = segments[segments.length - 1]
  if (last && last.kind === kind) {
    if (oldLine !== undefined) {
      ;(last.oldLines ??= []).unshift(oldLine)
    }
    if (newLine !== undefined) {
      ;(last.newLines ??= []).unshift(newLine)
    }
    return
  }
  const seg: DiffSegment = { kind }
  if (oldLine !== undefined) seg.oldLines = [oldLine]
  if (newLine !== undefined) seg.newLines = [newLine]
  segments.push(seg)
}

/**
 * Compute line-level LCS diff between two strings. Returns a sequence
 * of segments, each a contiguous run of one kind. Both inputs are
 * normalized by splitting on `\n` (or `\r\n`).
 *
 * Inputs are passed through `truncateLine` so very long lines don't
 * blow up the segment payload size — important for memory in 5000+
 * line documents.
 */
export function diffLines(oldText: string, newText: string): DiffSegment[] {
  const a = splitLines(oldText)
  const b = splitLines(newText)
  if (a.length === 0 && b.length === 0) return []
  if (a.length === 0) {
    return [{ kind: 'add', newLines: b.map(truncateLine) }]
  }
  if (b.length === 0) {
    return [{ kind: 'del', oldLines: a.map(truncateLine) }]
  }
  const cols = b.length + 1
  const dp = buildLcsTable(a, b)
  return backtrackLcs(a, b, dp, cols).map((seg) => ({
    kind: seg.kind,
    oldLines: seg.oldLines?.map(truncateLine),
    newLines: seg.newLines?.map(truncateLine),
  }))
}

/**
 * Render a side-by-side diff as HTML. Returns a string suitable for
 * `v-html`. Caller is responsible for escaping user-controlled strings
 * before this; we escape `oldLines` and `newLines` because the input
 * strings are wiki page content (markdown that may contain raw HTML).
 *
 * Layout: two columns. Each row carries at most one side's content;
 * `add` rows show empty cell on the left + line on the right, and
 * `del` the opposite. `eq` rows show the same line in both cells.
 */
export function diffSideBySideHtml(segments: DiffSegment[]): string {
  const out: string[] = ['<div class="wiki-diff-side-by-side">']
  for (const seg of segments) {
    if (seg.kind === 'eq') {
      const lines = seg.newLines ?? seg.oldLines ?? []
      for (const line of lines) {
        out.push(
          `<div class="wiki-diff-row wiki-diff-eq">` +
            `<div class="wiki-diff-cell">${escapeHtml(line)}</div>` +
            `<div class="wiki-diff-cell">${escapeHtml(line)}</div>` +
            `</div>`,
        )
      }
    } else if (seg.kind === 'add') {
      const lines = seg.newLines ?? []
      for (const line of lines) {
        out.push(
          `<div class="wiki-diff-row wiki-diff-add">` +
            `<div class="wiki-diff-cell wiki-diff-empty"></div>` +
            `<div class="wiki-diff-cell"><ins>${escapeHtml(line)}</ins></div>` +
            `</div>`,
        )
      }
    } else {
      const lines = seg.oldLines ?? []
      for (const line of lines) {
        out.push(
          `<div class="wiki-diff-row wiki-diff-del">` +
            `<div class="wiki-diff-cell"><del>${escapeHtml(line)}</del></div>` +
            `<div class="wiki-diff-cell wiki-diff-empty"></div>` +
            `</div>`,
        )
      }
    }
  }
  out.push('</div>')
  return out.join('')
}

/**
 * Render a unified diff as HTML. Returns a string suitable for `v-html`.
 * Each `add` line is prefixed with `+`, each `del` with `-`, each `eq`
 * with a space — matching `diff -u` style. `<ins>` / `<del>` wrappers
 * mark the changes visually.
 */
export function diffUnifiedHtml(segments: DiffSegment[]): string {
  const out: string[] = ['<pre class="wiki-diff-unified">']
  for (const seg of segments) {
    if (seg.kind === 'eq') {
      const lines = seg.newLines ?? seg.oldLines ?? []
      for (const line of lines) {
        out.push(`<div class="wiki-diff-line wiki-diff-eq"> ${escapeHtml(line)}</div>`)
      }
    } else if (seg.kind === 'add') {
      const lines = seg.newLines ?? []
      for (const line of lines) {
        out.push(`<div class="wiki-diff-line wiki-diff-add"><ins>+ ${escapeHtml(line)}</ins></div>`)
      }
    } else {
      const lines = seg.oldLines ?? []
      for (const line of lines) {
        out.push(`<div class="wiki-diff-line wiki-diff-del"><del>- ${escapeHtml(line)}</del></div>`)
      }
    }
  }
  out.push('</pre>')
  return out.join('')
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/**
 * Quick stats: how many lines were added / deleted / kept. Useful for
 * the revision-list badge ("+12 -3") without rendering the full diff.
 */
export interface DiffSummary {
  added: number
  deleted: number
  unchanged: number
}

export function summarizeDiff(segments: DiffSegment[]): DiffSummary {
  let added = 0
  let deleted = 0
  let unchanged = 0
  for (const seg of segments) {
    if (seg.kind === 'add') added += seg.newLines?.length ?? 0
    else if (seg.kind === 'del') deleted += seg.oldLines?.length ?? 0
    else unchanged += seg.newLines?.length ?? 0
  }
  return { added, deleted, unchanged }
}

/**
 * Hard ceiling: very long documents are returned as a single "too long"
 * sentinel segment so the UI can show "diff suppressed (X vs Y lines)"
 * rather than freeze the browser. The constant is exposed for tests.
 */
export const DIFF_MAX_LINES = 5000

export function diffLinesSafe(oldText: string, newText: string): {
  segments: DiffSegment[]
  truncated: boolean
} {
  const a = splitLines(oldText)
  const b = splitLines(newText)
  if (a.length > DIFF_MAX_LINES || b.length > DIFF_MAX_LINES) {
    return { segments: [], truncated: true }
  }
  return { segments: diffLines(oldText, newText), truncated: false }
}

/**
 * Field-level diff entry-points used by `WikiRevisionDrawer.vue`.
 *
 * The drawer renders side-by-side diffs for each page field (title,
 * summary, content). Rather than call `diffLines` three times in the
 * component, we expose a small wrapper that handles each field with
 * consistent semantics.
 *
 * `WikiRevisionSnapshot` mirrors the public fields the drawer needs —
 * it intentionally omits metadata (editor, source) because we never
 * diff those.
 */
export interface WikiRevisionSnapshot {
  title: string
  summary: string
  content: string
}

export type WikiRevisionDiffField = 'title' | 'summary' | 'content'

export interface WikiRevisionDiffLine {
  type: 'same' | 'add' | 'del'
  text: string
}

export interface WikiRevisionDiffSection {
  field: WikiRevisionDiffField
  lines: WikiRevisionDiffLine[]
}

const REVISION_FIELDS: WikiRevisionDiffField[] = ['title', 'summary', 'content']

/**
 * Build a per-line diff of one snapshot field, returned in a shape the
 * drawer can render directly. Coalesces adjacent same-kind lines for
 * readability.
 */
function diffField(
  field: WikiRevisionDiffField,
  fromText: string,
  toText: string,
): WikiRevisionDiffSection {
  const fromLines = splitLines(fromText)
  const toLines = splitLines(toText)
  if (
    fromLines.length > DIFF_MAX_LINES ||
    toLines.length > DIFF_MAX_LINES
  ) {
    return {
      field,
      lines: [
        { type: 'same', text: fromText.slice(0, 200) },
        { type: 'add', text: toText.slice(0, 200) },
      ],
    }
  }
  const segments = diffLines(fromText, toText)
  const lines: WikiRevisionDiffLine[] = []
  for (const seg of segments) {
    if (seg.kind === 'eq') {
      for (const t of seg.newLines ?? []) lines.push({ type: 'same', text: t })
    } else if (seg.kind === 'add') {
      for (const t of seg.newLines ?? []) lines.push({ type: 'add', text: t })
    } else {
      for (const t of seg.oldLines ?? []) lines.push({ type: 'del', text: t })
    }
  }
  return { field, lines }
}

/**
 * Diff two snapshots field-by-field. Fields whose lines are all `same`
 * are still returned — the drawer uses the empty-line state to show a
 * "no changes" hint per field.
 */
export function diffWikiRevision(
  from: WikiRevisionSnapshot,
  to: WikiRevisionSnapshot,
): WikiRevisionDiffSection[] {
  const sections: WikiRevisionDiffSection[] = []
  for (const field of REVISION_FIELDS) {
    sections.push(diffField(field, from[field] ?? '', to[field] ?? ''))
  }
  return sections
}