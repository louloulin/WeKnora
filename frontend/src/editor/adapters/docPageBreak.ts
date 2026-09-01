/**
 * docPageBreak — TipTap extension that adds `pageBreakBefore` to paragraph/heading
 * and `pageBreak` to hardBreak, so Word-style page breaks round-trip through the
 * docx-engine's <w:pageBreakBefore/> and <w:br w:type="page"/> XML elements.
 * Adapted from genoffice docs editor/page-break.ts (52 lines) for the TipTap
 * StarterKit used by CollabDocProEditor.
 */
import { Extension } from '@tiptap/core'

export const DocPageBreak = Extension.create({
  name: 'docPageBreak',
  addGlobalAttributes() {
    return [
      {
        types: ['paragraph', 'heading'],
        attributes: {
          pageBreakBefore: {
            default: false,
            parseHTML: (el: HTMLElement) => el.getAttribute('data-page-break-before') === 'true',
            renderHTML: (attrs: Record<string, unknown>) => {
              if (!attrs.pageBreakBefore) return {}
              return { 'data-page-break-before': 'true' }
            },
          },
        },
      },
      {
        types: ['hardBreak'],
        attributes: {
          pageBreak: {
            default: false,
            parseHTML: (el: HTMLElement) => el.getAttribute('data-page-break') === 'true',
            renderHTML: (attrs: Record<string, unknown>) => {
              if (!attrs.pageBreak) return {}
              return { 'data-page-break': 'true' }
            },
          },
        },
      },
    ]
  },
})
