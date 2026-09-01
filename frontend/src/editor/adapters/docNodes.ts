/**
 * v0.7.49 — lightweight DOC nodes for the collab editor (copy from genoffice
 * apps/docs/src/renderer/editor/extensions.ts DocInlineMath + a slimmed
 * docProtected, imports adapted to WeKnora).
 *
 * docInlineMath: atomic inline formula flowing with the text. `omml` is the
 * exact <m:oMath> fragment that saves verbatim; `mathml` renders natively in
 * Chromium; `latex` is kept for editor-created formulas so double-click can
 * re-edit.
 *
 * docProtected: block-level protected object. WeKnora only needs the formula
 * subset (blockType 'passthrough' + formulaDisplay), so the attr surface is
 * much smaller than genoffice's full image/table/chart/textbox set.
 */
import { Node } from '@tiptap/core'

export const DocInlineMath = Node.create({
  name: 'docInlineMath',
  inline: true,
  group: 'inline',
  atom: true,
  selectable: true,
  addAttributes() {
    return {
      omml: { default: '' },
      mathml: { default: '' },
      latex: { default: null as string | null },
      /** flat token strip (word count / AI read fallback) */
      text: { default: '' },
    }
  },
  parseHTML() {
    return [{ tag: 'span[data-inline-math]' }]
  },
  renderText({ node }) {
    return String(node.attrs.text ?? '')
  },
  renderHTML({ node }) {
    return [
      'span',
      {
        'data-inline-math': '1',
        class: 'doc-inline-math',
        title: String(node.attrs.latex ?? ''),
      },
      String(node.attrs.text ?? ''),
    ]
  },
  addNodeView() {
    return ({ node }) => {
      let currentNode = node
      const dom = document.createElement('span')
      const render = () => {
        dom.setAttribute('data-inline-math', '1')
        dom.className = 'doc-inline-math'
        const latex = String(currentNode.attrs.latex ?? '')
        if (latex) dom.title = latex
        const mathml = String(currentNode.attrs.mathml ?? '')
        if (mathml) dom.innerHTML = mathml
        else dom.textContent = String(currentNode.attrs.text ?? '')
      }
      render()
      return {
        dom,
        update: (n: typeof currentNode) => {
          if (n.type.name !== 'docInlineMath') return false
          currentNode = n
          render()
          return true
        },
      }
    }
  },
})

export const DocProtected = Node.create({
  name: 'docProtected',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: true,
  addAttributes() {
    return {
      docxIndex: { default: null as number | null },
      blockType: { default: 'passthrough' },
      label: { default: '' },
      previewText: { default: '' },
      /** self-contained OOXML fragment for editor-created content (formulas) */
      genXml: { default: null as string | null },
      /** editable OMML leaf tokens; formula structure remains protected */
      formulaDisplay: { default: null as Record<string, unknown> | null },
    }
  },
  parseHTML() {
    return [{ tag: 'div[data-doc-protected]' }]
  },
  renderHTML({ node }) {
    const attrs = node.attrs as Record<string, unknown>
    const fd = attrs.formulaDisplay as Record<string, unknown> | null
    return [
      'div',
      {
        'data-doc-protected': '1',
        class: 'doc-protected',
        'data-latex': String(fd?.latex ?? ''),
      },
      String(attrs.previewText ?? ''),
    ]
  },
  addNodeView() {
    return ({ node }) => {
      let currentNode = node
      const dom = document.createElement('div')
      const render = () => {
        dom.setAttribute('data-doc-protected', '1')
        dom.className = 'doc-protected'
        const attrs = currentNode.attrs as Record<string, unknown>
        const fd = attrs.formulaDisplay as Record<string, unknown> | null
        const latex = String(fd?.latex ?? '')
        if (latex) dom.setAttribute('data-latex', latex)
        const mathml = String(fd?.mathml ?? '')
        if (mathml) dom.innerHTML = mathml
        else dom.textContent = String(attrs.previewText ?? '')
      }
      render()
      return {
        dom,
        update: (n: typeof currentNode) => {
          if (n.type.name !== 'docProtected') return false
          currentNode = n
          render()
          return true
        },
      }
    }
  },
})
