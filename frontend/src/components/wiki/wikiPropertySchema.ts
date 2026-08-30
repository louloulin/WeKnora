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
      return String(value)
    default:
      return ''
  }
}
