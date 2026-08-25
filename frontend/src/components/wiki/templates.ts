/**
 * Built-in + user-defined wiki page templates (Build #3 + #4).
 *
 * Built-ins are pure markdown — they're sent verbatim as `content` on the
 * create-page API, which today does not accept `content_html`. The
 * WYSIWYG editor (Build #2b) renders these as the starting document the
 * first time the user opens the page, so even rich-text authors benefit.
 *
 * Adding a built-in template: append a `WikiPageTemplate` to
 * `WIKI_PAGE_TEMPLATES` with a unique `id`, a translation key for
 * `labelKey`, and a markdown body in `content`. No other code needs to
 * change — the new page dialog enumerates this list automatically.
 *
 * User-defined templates are stored in localStorage by
 * `composables/useUserTemplates.ts` (Build #4). They carry a `label`
 * instead of an i18n `labelKey`. `findTemplate()` resolves either
 * source so callers don't have to track which list to consult.
 */
export interface WikiPageTemplate {
  /** Stable identifier; used as the v-model value in the template selector. */
  id: string
  /** i18n key for built-in templates; ignored when `label` is set. */
  labelKey?:
    | 'newPageTemplateBlank'
    | 'newPageTemplateMeeting'
    | 'newPageTemplateWeekly'
  /** Direct label for user-defined templates; overrides labelKey when present. */
  label?: string
  /** Markdown body to seed `createPageForm.content`. */
  content: string
}

export const WIKI_PAGE_TEMPLATES: WikiPageTemplate[] = [
  {
    id: 'blank',
    labelKey: 'newPageTemplateBlank',
    content: '',
  },
  {
    id: 'meeting',
    labelKey: 'newPageTemplateMeeting',
    content: [
      '## 元信息',
      '',
      '- **日期**：YYYY-MM-DD',
      '- **时间**：HH:MM – HH:MM',
      '- **主持人**：',
      '- **参会人**：',
      '- **记录人**：',
      '',
      '## 议程',
      '',
      '1. ',
      '2. ',
      '',
      '## 讨论要点',
      '',
      '- ',
      '',
      '## 决议',
      '',
      '| 编号 | 决议 | 负责人 | 截止 |',
      '| --- | --- | --- | --- |',
      '|  |  |  |  |',
      '',
      '## Action Items',
      '',
      '- [ ] ',
      '',
      '## 备注',
      '',
      '',
    ].join('\n'),
  },
  {
    id: 'weekly',
    labelKey: 'newPageTemplateWeekly',
    content: [
      '## 本周概览',
      '',
      '- **周期**：YYYY-MM-DD – YYYY-MM-DD',
      '- **Owner**：',
      '- **本周主题**：',
      '',
      '## 本周完成',
      '',
      '- ',
      '',
      '## 进行中',
      '',
      '- ',
      '',
      '## 下周计划',
      '',
      '- ',
      '',
      '## 风险与阻塞',
      '',
      '| 风险 | 影响 | 缓解措施 | 状态 |',
      '| --- | --- | --- | --- |',
      '|  |  |  |  |',
      '',
      '## 关键数据',
      '',
      '- ',
      '',
      '## 备注',
      '',
      '',
    ].join('\n'),
  },
]

export const DEFAULT_TEMPLATE_ID = 'blank'

/**
 * Returns the markdown body for a built-in template `id`. Returns an empty
 * string for unknown ids or the blank template — callers can rely on
 * "empty" being a safe default rather than special-casing the blank id.
 */
export function templateContentById(id: string | null | undefined): string {
  if (!id) return ''
  const match = WIKI_PAGE_TEMPLATES.find((t) => t.id === id)
  return match ? match.content : ''
}

/**
 * Looks up a built-in template by id and returns the full record (with
 * labelKey). Returns undefined when the id is blank, unknown, or
 * reserved for user templates (`user_*` prefix).
 */
export function findBuiltinTemplate(id: string | null | undefined): WikiPageTemplate | undefined {
  if (!id || id === DEFAULT_TEMPLATE_ID || id.startsWith('user_')) return undefined
  return WIKI_PAGE_TEMPLATES.find((t) => t.id === id)
}