/**
 * docHeaderFooter — v0.7.109 DOC header/footer UI helpers
 *
 * Vendored from genoffice apps/docs/src/renderer/components/HeaderFooterArea.tsx
 * (Word "Insert > Header & Footer > Edit"). This module exposes the pure
 * helpers the Vue UI consumes — the inline editor (rendering), the
 * commit function, and the small parse helpers. The reactive Vue
 * component itself lives at components/collab/DocHeaderFooterDialog.vue.
 */
import type { HeaderFooter } from '../engines/docx-engine/types'

/** Constants reused in templates. */
export const PAGE_TOKEN = '#'

/** Parse a header/footer text into visible segments; PAGE_TOKEN is preserved
 *  verbatim so the editor can render it as a chip and replace it with the
 *  engine's PAGE_MARK on save. */
export function hfSegmentsOf(text: string): { kind: 'text' | 'page' | 'pages' | 'date'; value: string }[] {
  if (!text) return []
  const out: { kind: 'text' | 'page' | 'pages' | 'date'; value: string }[] = []
  // Tokenize: split on PAGE_TOKEN ('#'), but support '#PAGES#' for NUMPAGES.
  let cursor = 0
  let buf = ''
  const flushText = () => {
    if (buf) { out.push({ kind: 'text', value: buf }); buf = '' }
  }
  while (cursor < text.length) {
    const ch = text[cursor]
    if (ch === PAGE_TOKEN) {
      // peek ahead for '#PAGES#' or '#DATE#' (consume trailing '#' if present).
      if (text.startsWith('PAGES#', cursor + 1)) {
        flushText()
        out.push({ kind: 'pages', value: 'PAGES' })
        cursor += 1 + 'PAGES#'.length
      } else if (text.startsWith('DATE#', cursor + 1)) {
        flushText()
        out.push({ kind: 'date', value: 'DATE' })
        cursor += 1 + 'DATE#'.length
      } else if (text.startsWith('PAGES', cursor + 1)) {
        // bare '#PAGES' (no closing '#' yet) — treat the '#' as a page marker,
        // let the rest be parsed normally on the next loop iteration.
        flushText()
        out.push({ kind: 'page', value: '#' })
        cursor += 1
      } else if (text.startsWith('DATE', cursor + 1)) {
        flushText()
        out.push({ kind: 'page', value: '#' })
        cursor += 1
      } else {
        flushText()
        out.push({ kind: 'page', value: '#' })
        cursor += 1
      }
    } else {
      buf += ch
      cursor += 1
    }
  }
  flushText()
  return out
}

/** Render a header / footer text into a single HTML line with chip spans. */
export function hfInlineHtml(text: string): string {
  const segs = hfSegmentsOf(text)
  return segs.map((s) => {
    if (s.kind === 'text') return escapeHtml(s.value)
    return `<span class="doc-hf-chip doc-hf-chip--${s.kind}">${escapeHtml(s.value)}</span>`
  }).join('')
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[c] as string))
}

/** Default seed values for the dialog. */
export function defaultHeader(): HeaderFooter {
  return { text: '' }
}
export function defaultFooter(pageNumber = true): HeaderFooter {
  return pageNumber ? { text: PAGE_TOKEN, pageNumber: true } : { text: '' }
}

/** A header/footer is "empty" when both text and pageNumber are absent. */
export function isEmptyHf(hf: HeaderFooter | undefined | null): boolean {
  if (!hf) return true
  return !hf.text && !hf.pageNumber && !(hf.paras && hf.paras.length)
}
