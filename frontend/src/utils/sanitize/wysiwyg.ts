import { sanitizeHTML as baseSanitizeHTML } from '@/utils/security'

/**
 * wysiwyg.ts — Tiptap editor exit-sanitizer (Build #2b).
 *
 * Difference vs. the generic `sanitizeHTML` in utils/security.ts:
 *   * Failure mode: this wrapper returns **empty string** when DOMPurify
 *     throws (e.g. version conflict, malformed DOM tree). The generic
 *     utility falls back to `escapeHTML(html)` so the call site never
 *     loses content — that's the right behaviour for the KB markdown
 *     rendering path. But for the WYSIWYG editor we want a different
 *     posture: an un-renderable cached HTML must NEVER end up in
 *     `wiki_pages.content_html`, otherwise readers see the broken
 *     payload until the editor is re-opened and saved again.
 *   * Logging: this wrapper emits `console.warn` (single line) so the
 *     operator can correlate it with the request — the generic utility
 *     uses `console.error` because the lost content is recoverable.
 *
 * Everything else is delegated to the shared utility so the allowed
 * tag list stays in lockstep with the rest of the frontend.
 */
export function sanitizeWysiwygHTML(html: string): string {
  try {
    return baseSanitizeHTML(html)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.warn('[sanitizeWysiwygHTML] DOMPurify threw; returning empty string:', error)
    return ''
  }
}