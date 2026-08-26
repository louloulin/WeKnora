// Wiki page template skeleton types (Build #18 / P1.2).
//
// Mirrors the Go types in internal/types/wiki_template.go. The
// frontend parses a saved template's body for these placeholders and
// ships the resulting skeleton to POST .../apply-template:
//
//   {{child_pages}}        → catalog of all auto-generated children
//   {{child_section}}      → in-page anchor list of all sections
//   {{tagged_pages:foo}}   → list of pages tagged `foo` in this KB
//
// Server responses (`WikiApplyTemplateResult`) carry the post-apply
// view: created slugs, resolved section anchors, and the rewritten
// parent body.

export interface WikiTemplatePlaceholderChild {
  /** Optional slug; server derives "<parent>-<title>" when empty. */
  slug?: string
  /** Required: visible page title. */
  title: string
  /** Seed body; empty string leaves the new page blank. */
  content?: string
  /** Pre-applied tag names (Build #17 tag system). */
  default_tags?: string[]
}

export interface WikiTemplatePlaceholderSection {
  /** Optional markdown anchor id; server slugifies Title when empty. */
  anchor?: string
  /** Required: heading text (level 2). */
  title: string
  /** Body rendered under the heading. */
  body?: string
}

export interface WikiTemplateSkeleton {
  children?: WikiTemplatePlaceholderChild[]
  sections?: WikiTemplatePlaceholderSection[]
  /** Token names referenced via {{tagged_pages:foo}}. */
  tagged_tokens?: string[]
}

export interface WikiApplyTemplateRequest {
  /** Optional originating template id; stamped into SourceRefs. */
  template_id?: string
  /** Replaces parent body verbatim. */
  body_override?: string
  /** Appended after the rewritten body. */
  body_append?: string
  /** Required: what to materialise. */
  skeleton: WikiTemplateSkeleton
}

export interface WikiApplyTemplateCreatedPage {
  slug: string
  title: string
}

export interface WikiApplyTemplateResolvedSection {
  anchor: string
  title: string
}

export interface WikiApplyTemplateResult {
  parent_slug: string
  parent_title: string
  pages: WikiApplyTemplateCreatedPage[]
  sections: WikiApplyTemplateResolvedSection[]
  /** token → list of page slugs matched. */
  tagged_pages: Record<string, string[]>
  /** Rewritten parent body (empty when no placeholders resolved). */
  new_body?: string
}

/** Sentinel error codes returned by the handler. */
export type WikiApplyTemplateErrorCode =
  | 'empty_skeleton'
  | 'oversize_skeleton'
  | string

/** Helper to extract parsed placeholders out of a raw template body. */
export interface ParsedTemplatePlaceholders {
  children: number
  sections: number
  taggedTokens: string[]
  /** First matches of each token kind, used by the dialog for preview. */
  sample: {
    childPages?: string
    childSection?: string
    taggedPages: Record<string, string>
  }
}

/**
 * parseTemplateBody scans a template body for placeholder tokens and
 * returns the counts + first-match coordinates. Pure: no API calls.
 * Used by TemplateApplyDialog to surface what the apply will expand.
 */
export function parseTemplateBody(body: string): ParsedTemplatePlaceholders {
  const childPages = /\{\{child_pages\}\}/g
  const childSection = /\{\{child_section\}\}/g
  const taggedPages = /\{\{tagged_pages:([^}]+)\}\}/g

  const sample: ParsedTemplatePlaceholders['sample'] = {
    taggedPages: {},
  }
  const m1 = childPages.exec(body)
  if (m1) sample.childPages = m1[0]
  const m2 = childSection.exec(body)
  if (m2) sample.childSection = m2[0]

  const taggedTokens: string[] = []
  let m3: RegExpExecArray | null
  while ((m3 = taggedPages.exec(body)) !== null) {
    const token = (m3[1] || '').trim()
    if (token && !taggedTokens.includes(token)) {
      taggedTokens.push(token)
    }
    if (!sample.taggedPages[token]) {
      sample.taggedPages[token] = m3[0]
    }
  }

  return {
    children: body.match(/\{[\s\S]*?\{\{child_pages\}\}[\s\S]*?\}/g)?.length ?? (sample.childPages ? 1 : 0),
    sections: body.match(/\{[\s\S]*?\{\{child_section\}\}[\s\S]*?\}/g)?.length ?? (sample.childSection ? 1 : 0),
    taggedTokens,
    sample,
  }
}