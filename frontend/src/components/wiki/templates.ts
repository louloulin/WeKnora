/**
 * Built-in wiki page templates (Build #3).
 *
 * Templates are pure markdown — they're sent verbatim as `content` on the
 * create-page API, which today does not accept `content_html`. The
 * WYSIWYG editor (Build #2b) renders these as the starting document the
 * first time the user opens the page, so even rich-text authors benefit.
 *
 * Adding a template: append a `WikiPageTemplate` to `WIKI_PAGE_TEMPLATES`
 * with a unique `id`, a translation key for `labelKey`, and a markdown
 * body in `content`. No other code needs to change — the new page dialog
 * enumerates this list automatically.
 */
export interface WikiPageTemplate {
  /** Stable identifier; used as the v-model value in the template selector. */
  id: string
  /** i18n key under knowledgeEditor.wikiBrowser for the display label. */
  labelKey:
    | 'newPageTemplateBlank'
    | 'newPageTemplateMeeting'
    | 'newPageTemplateWeekly'
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
 * Returns the template body for `id`, falling back to an empty string
 * when the id is unknown (e.g. a stale persisted form value).
 */
export function templateContentById(id: string | null | undefined): string {
  if (!id) return ''
  const match = WIKI_PAGE_TEMPLATES.find((t) => t.id === id)
  return match ? match.content : ''
}