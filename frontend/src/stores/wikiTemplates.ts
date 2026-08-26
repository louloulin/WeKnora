import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  type WikiApplyTemplateRequest,
  type WikiApplyTemplateResult,
  applyWikiTemplate,
  previewWikiTemplate,
} from '../api/wiki/templates'

/**
 * Wiki page templates Pinia store (Build #18 / P1.2).
 *
 * Owns two concerns:
 *
 *   1. The dry-run preview state — `lastPreview` lets the dialog
 *      render the rewritten body before the user commits.
 *   2. The apply result — `lastApply` lets the parent page-list
 *      re-render with the new child slugs after a successful apply.
 *
 * All mutations go through the API client. The store never optimistically
 * edits the cache because the apply path mutates multiple rows across
 * wiki_pages / wiki_page_tags in one round-trip and a stale cache would
 * show the old child set until the next page-list fetch.
 */
export const useWikiTemplatesStore = defineStore('wikiTemplates', () => {
  /** Last preview result, surfaced by the dialog. */
  const lastPreview = ref<WikiApplyTemplateResult | null>(null)
  /** Last apply result, surfaced by the parent page-list refresh. */
  const lastApply = ref<WikiApplyTemplateResult | null>(null)
  /** Per-page in-flight status. Keyed by `${kbId}::${slug}`. */
  const inFlight = ref<Record<string, boolean>>({})
  /** Last error message keyed by the same composite key. */
  const errors = ref<Record<string, string>>({})

  function key(kbId: string, slug: string): string {
    return `${kbId}::${slug}`
  }

  async function preview(
    kbId: string,
    slug: string,
    body: WikiApplyTemplateRequest,
  ): Promise<WikiApplyTemplateResult> {
    const k = key(kbId, slug)
    inFlight.value[k] = true
    delete errors.value[k]
    try {
      const result = await previewWikiTemplate(kbId, slug, body)
      lastPreview.value = result
      return result
    } catch (err) {
      errors.value[k] = err instanceof Error ? err.message : String(err)
      throw err
    } finally {
      inFlight.value[k] = false
    }
  }

  async function apply(
    kbId: string,
    slug: string,
    body: WikiApplyTemplateRequest,
  ): Promise<WikiApplyTemplateResult> {
    const k = key(kbId, slug)
    inFlight.value[k] = true
    delete errors.value[k]
    try {
      const result = await applyWikiTemplate(kbId, slug, body)
      lastApply.value = result
      // Invalidate the preview cache — the next apply would otherwise
      // re-render the previous apply's rewrite on top of the new one.
      if (lastPreview.value && lastPreview.value.parent_slug === slug) {
        lastPreview.value = null
      }
      return result
    } catch (err) {
      errors.value[k] = err instanceof Error ? err.message : String(err)
      throw err
    } finally {
      inFlight.value[k] = false
    }
  }

  function isInFlight(kbId: string, slug: string): boolean {
    return Boolean(inFlight.value[key(kbId, slug)])
  }

  function lastError(kbId: string, slug: string): string | null {
    return errors.value[key(kbId, slug)] ?? null
  }

  return {
    lastPreview,
    lastApply,
    inFlight,
    errors,
    preview,
    apply,
    isInFlight,
    lastError,
  }
})