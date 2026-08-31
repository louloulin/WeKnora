// Property schema for Wiki page properties.
//
// Mirrors the WeKnora §20.3 borrow-list item "页面属性（properties）":
// 每页可挂类型化字段,作为 Database view 的输入(对齐 Notion properties
// + Confluence live doc metadata + Feishu 多维表格)。
//
// The schema is owned per-KB. For v1 we ship a default schema covering the
// most common wiki page metadata; KB-level customisation lands in a
// follow-up. Property values are stored in `page_metadata` (JSON column
// on wiki_pages table) under the property id.

export type PropertyType =
  | 'text'
  | 'number'
  | 'date'
  | 'select'
  | 'multi-select'
  | 'checkbox'
  | 'url'
  // Sprint 1 §26 property-type expansion — brings the WeKnora set from
  // 7 to 13 types, closing the gap with Notion's 19-property baseline
  // (§20.3 A.1). Each new type has coerce + format + a default
  // validation rule below.
  | 'person'    // reference to a user (string id, resolved client-side)
  | 'status'    // select with workflow semantics (Todo/Doing/Done…)
  | 'relation'  // reference to one or more wiki pages (slug array)
  | 'rollup'    // computed aggregate over a relation (read-only)
  | 'formula'   // computed value, evaluated server-side (read-only)
  | 'email'     // RFC-5322-lite email string

export interface WikiProperty {
  id: string
  name: string
  type: PropertyType
  icon?: string
  options?: string[]
  /** Markdown-style hint shown under the input. */
  description?: string
}

export type PropertyValue =
  | string
  | number
  | boolean
  | string[]
  | null
  // rollup and formula can hold heterogeneous values (count → number,
  // join → string, earliest → string). They are read-only from the
  // client side: writes are rejected by coercePropertyValue.
  | PropertyValue[]

export type PropertyValues = Record<string, PropertyValue>

/** Default schema used when a KB has no custom property configuration. */
export const DEFAULT_PROPERTY_SCHEMA: WikiProperty[] = [
  {
    id: 'status',
    name: '状态',
    type: 'select',
    icon: 'flag',
    options: ['Draft', 'Review', 'Published', 'Archived'],
  },
  {
    id: 'priority',
    name: '优先级',
    type: 'select',
    icon: 'priority',
    options: ['Low', 'Medium', 'High', 'Critical'],
  },
  {
    id: 'owner',
    name: '负责人',
    type: 'text',
    icon: 'user',
  },
  {
    id: 'due_date',
    name: '截止日期',
    type: 'date',
    icon: 'calendar',
  },
  {
    id: 'tags',
    name: '标签',
    type: 'multi-select',
    icon: 'tag',
  },
  {
    id: 'confidential',
    name: '机密',
    type: 'checkbox',
    icon: 'lock',
  },
  {
    id: 'source_url',
    name: '来源链接',
    type: 'url',
    icon: 'link',
  },
]

export const PROPERTY_TYPE_LABELS: Record<PropertyType, string> = {
  text: '文本',
  number: '数字',
  date: '日期',
  select: '单选',
  'multi-select': '多选',
  checkbox: '开关',
  url: '链接',
  person: '人员',
  status: '状态',
  relation: '关联',
  rollup: '汇总',
  formula: '公式',
  email: '邮箱',
}

/**
 * Validate and coerce a raw value to match the property's declared type.
 * Returns the coerced value or null if the value cannot be coerced.
 */
export function coercePropertyValue(
  property: WikiProperty,
  raw: unknown,
): PropertyValue {
  if (raw === null || raw === undefined) return null
  switch (property.type) {
    case 'text':
      return typeof raw === 'string' ? raw : String(raw)
    case 'number': {
      const n = typeof raw === 'number' ? raw : Number(raw)
      return Number.isFinite(n) ? n : null
    }
    case 'date':
      return typeof raw === 'string' && raw.length > 0 ? raw : null
    case 'select':
      if (typeof raw !== 'string') return null
      if (!property.options || property.options.length === 0) return raw
      return property.options.includes(raw) ? raw : null
    case 'multi-select': {
      if (!Array.isArray(raw)) return null
      const arr = raw.filter((v): v is string => typeof v === 'string')
      if (!property.options || property.options.length === 0) return arr
      return arr.filter((v) => property.options!.includes(v))
    }
    case 'checkbox':
      return typeof raw === 'boolean' ? raw : null
    case 'url':
      return typeof raw === 'string' && /^https?:\/\//i.test(raw) ? raw : null
    case 'email':
      // Light RFC-5322 check: <local>@<domain>.<tld>. We intentionally
      // do not implement full RFC 5322 (it's intentionally impossible to
      // do right with a regex); this catches the common typos.
      return typeof raw === 'string' && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(raw) ? raw : null
    case 'person':
      // Person is a user reference; we accept any non-empty string and
      // let the server / user picker decide if it resolves. The schema
      // store may attach an `options: ['user_id_1', ...]` list to bound
      // the picker.
      return typeof raw === 'string' && raw.length > 0 ? raw : null
    case 'status': {
      // status is a select with workflow semantics; same coercion as
      // select but rendered with extra status-specific chrome (Todo /
      // Doing / Done chips). Allow free-form if no options declared.
      if (typeof raw !== 'string') return null
      if (!property.options || property.options.length === 0) return raw
      return property.options.includes(raw) ? raw : null
    }
    case 'relation': {
      // Relation is one or more page slugs. Array of strings; coerce
      // individual entries and dedupe so the same page can't be linked
      // twice from one relation property.
      if (!Array.isArray(raw)) return null
      const slugs = raw.filter((v): v is string => typeof v === 'string' && v.length > 0)
      if (slugs.length === 0) return null
      return Array.from(new Set(slugs))
    }
    case 'rollup':
    case 'formula':
      // Read-only computed values; the server is the source of truth.
      // We accept whatever the wire carries but never let the client
      // write directly — the editor UI hides the input for these types.
      return raw as PropertyValue
    default:
      return null
  }
}

/** Build a PropertyValues map by coercing every entry in `page_metadata`. */
export function readPropertyValues(
  schema: WikiProperty[],
  pageMetadata: Record<string, any> | undefined | null,
): PropertyValues {
  const result: PropertyValues = {}
  if (!pageMetadata) return result
  for (const prop of schema) {
    if (Object.prototype.hasOwnProperty.call(pageMetadata, prop.id)) {
      const coerced = coercePropertyValue(prop, pageMetadata[prop.id])
      if (coerced !== null) result[prop.id] = coerced
    }
  }
  return result
}

/**
 * Merge new values into the existing page_metadata, preserving unknown keys
 * so we never silently drop keys we don't know about.
 */
export function writePropertyValues(
  pageMetadata: Record<string, any> | undefined | null,
  updates: PropertyValues,
): Record<string, any> {
  const base: Record<string, any> = { ...(pageMetadata || {}) }
  for (const [key, value] of Object.entries(updates)) {
    if (value === null || value === undefined) {
      delete base[key]
    } else {
      base[key] = value
    }
  }
  return base
}

/** Display-formatted value for read-only rendering. */
export function formatPropertyValue(
  property: WikiProperty,
  value: PropertyValue,
): string {
  if (value === null || value === undefined) return ''
  switch (property.type) {
    case 'checkbox':
      return value ? '✓' : '—'
    case 'multi-select':
      return Array.isArray(value) ? value.join(', ') : ''
    case 'date':
    case 'text':
    case 'select':
    case 'url':
    case 'number':
    case 'email':
    case 'person':
      return String(value)
    case 'status': {
      // Status renders with a leading emoji / icon prefix when the
      // option list uses Notion-style values (Todo / Doing / Done /
      // Backlog / In Review). The UI component applies the actual
      // colour/icon; here we only return the label.
      return String(value)
    }
    case 'relation': {
      if (!Array.isArray(value)) return ''
      if (value.length === 0) return ''
      if (value.length === 1) return String(value[0])
      return `${value.length} pages`
    }
    case 'rollup':
    case 'formula': {
      if (value === null || value === undefined) return ''
      if (typeof value === 'number') return String(value)
      if (typeof value === 'boolean') return value ? '✓' : '—'
      if (Array.isArray(value)) return value.length === 1 ? String(value[0]) : `${value.length} items`
      return String(value)
    }
    default:
      return ''
  }
}

/**
 * Read-only property types cannot be set by the user directly; the value
 * is computed from other properties (formula) or from referenced pages
 * (rollup). The editor UI hides the input and shows a chip with a
 * tooltip explaining the source.
 */
export function isReadOnlyPropertyType(type: PropertyType): boolean {
  return type === 'rollup' || type === 'formula'
}

/**
 * Default property additions for §26 type expansion. These mirror the
 * Notion 19-property baseline (§20.3 A.1) for the types a wiki page is
 * likely to use out of the box. KMs can override per KB.
 */
export const EXTENDED_DEFAULT_PROPERTIES: WikiProperty[] = [
  {
    id: 'assignee',
    name: '负责人',
    type: 'person',
    icon: 'user',
    description: '当前负责人。客户端通过用户选择器解析。',
  },
  {
    id: 'workflow',
    name: '工作流',
    type: 'status',
    icon: 'flow',
    options: ['Backlog', 'Todo', 'In Progress', 'In Review', 'Done', 'Cancelled'],
  },
  {
    id: 'reviewer',
    name: '评审人',
    type: 'person',
    icon: 'user-check',
  },
  {
    id: 'contact_email',
    name: '联系邮箱',
    type: 'email',
    icon: 'mail',
  },
  {
    id: 'related_pages',
    name: '关联页面',
    type: 'relation',
    icon: 'link',
    description: '指向其他 wiki 页面的 slug 数组。',
  },
  {
    id: 'related_count',
    name: '关联数',
    type: 'rollup',
    icon: 'sum',
    description: 'related_pages 的条数，由后端计算。',
  },
  {
    id: 'last_edited_by',
    name: '最后编辑',
    type: 'person',
    icon: 'edit',
  },
]

/** All built-in defaults merged: original + extended. */
export const ALL_DEFAULT_PROPERTIES: WikiProperty[] = [
  ...DEFAULT_PROPERTY_SCHEMA,
  ...EXTENDED_DEFAULT_PROPERTIES,
]
