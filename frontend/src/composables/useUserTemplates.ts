import { computed, ref, watch } from 'vue'
import type { WikiPageTemplate } from '@/components/wiki/templates'

/**
 * User-defined wiki page templates (Build #4).
 *
 * Storage: `localStorage` under `weknora.wiki.userTemplates.v1`. Per-browser
 * only — there is no backend sync today, so templates authored on device A
 * won't appear on device B. That's an intentional simplification: shipping
 * this as a backend-driven feature would mean a `wiki_user_templates` table
 * + handler + RLS, none of which the sandbox can verify. A future Build can
 * lift the storage layer without changing the call site (the composable
 * interface is the contract).
 *
 * IDs are namespaced (`user_<slug>`) so a future server-supplied template
 * with id `meeting` cannot collide with the built-in `meeting`.
 *
 * The composable returns a single source of truth shared across every
 * call site in the bundle via module-level reactive state. There is no
 * Pinia store here because the data is private to the browser session
 * and there is no cross-route synchronisation requirement.
 */

const STORAGE_KEY = 'weknora.wiki.userTemplates.v1'
const ID_PREFIX = 'user_'
const MAX_TEMPLATES = 50
const MAX_NAME_LENGTH = 64
const MAX_CONTENT_LENGTH = 20_000

export interface UserTemplateRecord {
  id: string
  label: string
  content: string
  /** Unix-ms timestamp; set on create + update. */
  updatedAt: number
}

function safeReadStorage(): UserTemplateRecord[] {
  if (typeof window === 'undefined' || !window.localStorage) return []
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter(
        (entry): entry is UserTemplateRecord =>
          entry &&
          typeof entry.id === 'string' &&
          typeof entry.label === 'string' &&
          typeof entry.content === 'string',
      )
      .slice(0, MAX_TEMPLATES)
  } catch {
    // localStorage may throw in private mode / SSR / quota-exceeded.
    // We degrade silently — losing user templates is acceptable; throwing
    // would break the new-page dialog.
    return []
  }
}

function safeWriteStorage(records: UserTemplateRecord[]): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(records))
  } catch {
    // quota / private mode — same fail-soft posture as the read side.
  }
}

function slugify(label: string): string {
  return (
    label
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9_]+/g, '_')
      .replace(/^_+|_+$/g, '')
      .slice(0, 32) || 'tmpl'
  )
}

function makeId(label: string, records: UserTemplateRecord[]): string {
  const base = `${ID_PREFIX}${slugify(label)}`
  let candidate = base
  let suffix = 2
  while (records.some((r) => r.id === candidate)) {
    candidate = `${base}_${suffix++}`
  }
  return candidate
}

// Module-level state — every call site reads/writes the same array.
const userTemplates = ref<UserTemplateRecord[]>(safeReadStorage())

// Persist on every mutation. Using a single watcher keeps the persistence
// policy in one place and avoids subtle bugs where some mutations skip
// the write step.
watch(
  userTemplates,
  (next) => {
    safeWriteStorage(next)
  },
  { deep: true },
)

export function useUserTemplates() {
  const templates = computed(() => userTemplates.value)

  /**
   * Resolves a template by id from the user-defined list only. Built-ins
   * are not enumerated here — the new-page dialog concatenates this list
   * with the canonical `WIKI_PAGE_TEMPLATES` and dedupes by id.
   */
  function findUserTemplate(id: string): WikiPageTemplate | undefined {
    if (!id || !id.startsWith(ID_PREFIX)) return undefined
    const match = userTemplates.value.find((r) => r.id === id)
    if (!match) return undefined
    return { id: match.id, label: match.label, content: match.content }
  }

  function addTemplate(label: string, content: string): UserTemplateRecord {
    const cleanLabel = label.trim().slice(0, MAX_NAME_LENGTH) || '未命名模板'
    const cleanContent = content.slice(0, MAX_CONTENT_LENGTH)
    const record: UserTemplateRecord = {
      id: makeId(cleanLabel, userTemplates.value),
      label: cleanLabel,
      content: cleanContent,
      updatedAt: Date.now(),
    }
    const withoutDup = userTemplates.value.filter((r) => r.id !== record.id)
    withoutDup.unshift(record)
    if (withoutDup.length > MAX_TEMPLATES) withoutDup.length = MAX_TEMPLATES
    userTemplates.value = withoutDup
    return record
  }

  function updateTemplate(id: string, patch: { label?: string; content?: string }): void {
    userTemplates.value = userTemplates.value.map((r) =>
      r.id === id
        ? {
            ...r,
            label: patch.label !== undefined ? patch.label.trim().slice(0, MAX_NAME_LENGTH) || r.label : r.label,
            content: patch.content !== undefined ? patch.content.slice(0, MAX_CONTENT_LENGTH) : r.content,
            updatedAt: Date.now(),
          }
        : r,
    )
  }

  function removeTemplate(id: string): void {
    userTemplates.value = userTemplates.value.filter((r) => r.id !== id)
  }

  return {
    templates,
    findUserTemplate,
    addTemplate,
    updateTemplate,
    removeTemplate,
    MAX_NAME_LENGTH,
    MAX_CONTENT_LENGTH,
  }
}