/**
 * Pure scoring / highlighting helpers for wiki full-text search
 * (Build #9-A). No Vue / Pinia / DOM dependencies so the algorithm
 * is trivially unit-testable.
 *
 * Scoring rules (mirrors spec.md A6):
 *   - title hit on a keyword: +10
 *   - content hit on a keyword: +1 per occurrence
 *   - case-insensitive (lowercase compare)
 *   - multi-keyword (split on whitespace) is AND: every keyword must
 *     hit at least once in (title + content); otherwise the page is
 *     not returned (returns null from scoreMatch).
 *   - returned score is `titleHits * 10 + contentHits * 1`, summed
 *     across all keywords.
 */

export interface ScoredPage {
  title: string
  content: string
  path?: string[]
}

export interface ScoreMatchResult {
  score: number
  titleHits: number
  contentHits: number
  matchedKeywords: string[]
}

/**
 * Split a raw query into lowercase keywords. Empty tokens are dropped
 * so "  weekly  meeting  " → ["weekly", "meeting"].
 */
export function splitKeywords(query: string): string[] {
  return query
    .toLowerCase()
    .split(/\s+/u)
    .filter((kw) => kw.length > 0)
}

/**
 * Score one page against the query keywords.
 *
 * Returns null when any keyword has zero hits in (title + content) —
 * the AND semantics of A5 require us to drop the page entirely.
 */
export function scoreMatch(page: ScoredPage, keywords: string[]): ScoreMatchResult | null {
  if (keywords.length === 0) return null
  const titleLc = page.title.toLowerCase()
  const contentLc = page.content.toLowerCase()

  let totalScore = 0
  let titleHits = 0
  let contentHits = 0
  const matched: string[] = []

  for (const rawKw of keywords) {
    const kw = rawKw.toLowerCase()
    let kwTitleHits = 0
    let kwContentHits = 0
    let cursor = 0
    while ((cursor = titleLc.indexOf(kw, cursor)) !== -1) {
      kwTitleHits += 1
      cursor += kw.length
    }
    cursor = 0
    while ((cursor = contentLc.indexOf(kw, cursor)) !== -1) {
      kwContentHits += 1
      cursor += kw.length
    }
    if (kwTitleHits + kwContentHits === 0) {
      return null
    }
    titleHits += kwTitleHits
    contentHits += kwContentHits
    totalScore += kwTitleHits * 10 + kwContentHits
    matched.push(rawKw)
  }

  return {
    score: totalScore,
    titleHits,
    contentHits,
    matchedKeywords: matched,
  }
}

/**
 * Build a short snippet around the first content hit, with a
 * configurable radius. Used by the API layer to populate
 * `WikiSearchResult.snippet`.
 */
export function buildSnippet(content: string, keywords: string[], radius: number = 80): string {
  if (content.length === 0) return ''
  for (const kw of keywords) {
    const idx = content.toLowerCase().indexOf(kw.toLowerCase())
    if (idx === -1) continue
    const start = Math.max(0, idx - radius)
    const end = Math.min(content.length, idx + kw.length + radius)
    const prefix = start > 0 ? '…' : ''
    const suffix = end < content.length ? '…' : ''
    return prefix + content.slice(start, end) + suffix
  }
  return content.length <= radius * 2 ? content : content.slice(0, radius * 2) + '…'
}

/**
 * Wrap keyword occurrences in a string with `<mark>` tags for the
 * results popup. The result is safe to inject with `v-html` only when
 * the caller controls `text` AND `keywords` (we never touch user-supplied
 * HTML here — we only wrap the user's input string).
 *
 * The output escapes `<`, `>`, `&`, `"` before insertion so the
 * marked fragment can be rendered with `v-html` without XSS risk.
 */
export function highlight(text: string, keywords: string[]): string {
  const escaped = escapeHtml(text)
  if (keywords.length === 0) return escaped
  // Build a regex that matches any of the keywords, longest first so
  // "meeting" wins over "meet". Escape regex metachars first.
  const sorted = [...new Set(keywords)].sort((a, b) => b.length - a.length)
  const pattern = sorted.map(escapeRegex).join('|')
  if (!pattern) return escaped
  const re = new RegExp(`(${pattern})`, 'giu')
  return escaped.replace(re, '<mark>$1</mark>')
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/gu, '\\$&')
}