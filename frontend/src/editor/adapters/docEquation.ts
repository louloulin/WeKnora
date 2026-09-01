import { latexToOmml, mathParagraphXml, mathTokensOf, ommlToMathML } from '../engines/docx-engine'

/// Minimal i18n stub — equation.ts only uses t() for one tooltip string.
/// We return the key as-is so tests / production stay decoupled.
const t = (key: string): string => key

/// Minimal ProseMirror node shape (subset of the convert.ts PmNode).
export interface PmNode {
  type: string
  attrs?: Record<string, unknown>
  content?: PmNode[]
  text?: string
  marks?: Array<{ type: string; attrs?: Record<string, unknown> }>
}

/**
 * Protected display-equation block built from LaTeX (throws on syntax outside
 * the supported subset). Shared by the insert dialog/gallery and the AI
 * <formula> tag; genXml carries the OMML through the save path.
 */
/** MathML for inline flow (ommlToMathML emits display="block" for equations) */
export function inlineMathML(omml: string): string {
  return ommlToMathML(omml).replace(/ display="block"/g, ' display="inline"')
}

/**
 * Atomic inline formula node built from LaTeX (throws on unsupported syntax).
 * Flows with paragraph text; saves as a Run.math (<m:oMath> emitted verbatim).
 */
export function inlineEquationNodeJson(latex: string): PmNode {
  const inner = latexToOmml(latex)
  const omml = `<m:oMath>${inner}</m:oMath>`
  return {
    type: 'docInlineMath',
    attrs: {
      omml,
      mathml: inlineMathML(omml),
      latex: latex.trim(),
      text: mathTokensOf(omml).join(''),
    },
  }
}

export function equationBlockJson(latex: string): PmNode {
  const omml = latexToOmml(latex)
  return {
    type: 'docProtected',
    attrs: {
      docxIndex: null,
      blockType: 'passthrough',
      label: t('editorEquation'),
      previewText: latex.trim(),
      genXml: mathParagraphXml(omml),
      formulaDisplay: {
        tokens: mathTokensOf(omml),
        mathml: ommlToMathML(`<m:oMath>${omml}</m:oMath>`),
        omml: `<m:oMath>${omml}</m:oMath>`,
        latex: latex.trim(),
      },
    },
  }
}
