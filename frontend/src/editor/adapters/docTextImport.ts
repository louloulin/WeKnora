/**
 * docTextImport — plain-text / Markdown → DOC paragraphs for the Tencent Docs
 * compatibility layer (v0.7.58). Tencent Docs 在线文档 imports .txt/.md; this
 * mirrors that path: text lines become paragraphs, Markdown headings/lists/
 * quotes/code become DOC paragraph kinds. Inline Markdown syntax is stripped
 * (bold/italic/code/link) so the imported text reads cleanly.
 */
import { marked } from 'marked'

export interface DocImportParagraph {
  text: string
  kind: 'paragraph' | 'heading' | 'listItem'
  level?: number
}

/** Strip inline Markdown syntax: **bold**, *italic*, `code`, [text](url). */
export function stripInlineMarkdown(text: string): string {
  return text
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/~~([^~]+)~~/g, '$1')
}

/** Plain text → DOC paragraphs (one line per paragraph). */
export function textToDocParagraphs(text: string): DocImportParagraph[] {
  const lines = text.replace(/\r\n?/g, '\n').split('\n')
  const out: DocImportParagraph[] = []
  for (const line of lines) {
    if (line.trim() === '') continue
    out.push({ text: line, kind: 'paragraph' })
  }
  if (out.length === 0) out.push({ text: '', kind: 'paragraph' })
  return out
}

/** Markdown → DOC paragraphs via marked.lexer (headings/lists/quotes/code). */
export function markdownToDocParagraphs(md: string): DocImportParagraph[] {
  const tokens = marked.lexer(md, { gfm: true, breaks: false })
  const out: DocImportParagraph[] = []
  for (const token of tokens) {
    switch (token.type) {
      case 'heading': {
        const t = token as { depth: number; text: string }
        out.push({ text: stripInlineMarkdown(t.text), kind: 'heading', level: Math.min(Math.max(t.depth, 1), 6) })
        break
      }
      case 'paragraph':
        out.push({ text: stripInlineMarkdown((token as { text: string }).text), kind: 'paragraph' })
        break
      case 'list': {
        const t = token as { items: Array<{ text: string }> }
        for (const item of t.items) {
          out.push({ text: stripInlineMarkdown(item.text), kind: 'listItem' })
        }
        break
      }
      case 'blockquote':
        out.push({ text: stripInlineMarkdown((token as { text: string }).text), kind: 'paragraph' })
        break
      case 'code':
        out.push({ text: (token as { text: string }).text, kind: 'paragraph' })
        break
      default:
        break
    }
  }
  if (out.length === 0) out.push({ text: '', kind: 'paragraph' })
  return out
}

/** Detect Markdown by unambiguous constructs (heading / fence / link / list). */
export function looksLikeMarkdown(text: string): boolean {
  const strong = [
    /^#{1,6}\s+\S/m,
    /^\s{0,3}```/m,
    /\[[^\]\n]+\]\([^\s)]+\)/,
    /(?:^|\W)\*\*[^*\n]+\*\*(?:\W|$)/,
  ]
  if (strong.some((re) => re.test(text))) return true
  const lines = text.split('\n')
  const listLines = lines.filter((l) => /^\s{0,3}(?:[-*+]|\d{1,3}[.)])\s+\S/.test(l)).length
  return listLines >= 2
}

/** Convert imported text to DOC paragraphs, auto-detecting Markdown. */
export function importTextToDocParagraphs(text: string): DocImportParagraph[] {
  return looksLikeMarkdown(text) ? markdownToDocParagraphs(text) : textToDocParagraphs(text)
}
