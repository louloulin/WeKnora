import { ref } from 'vue'
import { defineStore } from 'pinia'
import { getFeatures } from '@/api/features'

/**
 * Runtime feature flags store.
 *
 * Modelled after `useDeploymentCapabilitiesStore` (see
 * `stores/deploymentCapabilities.ts`): a single `ensureLoaded()` call,
 * an idempotent promise guard via module-local `loadingPromise`, and a
 * `loaded` boolean that flips once regardless of success/failure so
 * re-mounts don't repeatedly hit the backend. On error every flag
 * defaults to `false` — fail-open keeps the UI functional even when
 * the feature endpoint is down, and the backend is still the source
 * of truth for any RBAC-gated route.
 *
 * The only flag wired today is `wiki_wysiwyg` (Build #2b); additional
 * fields belong as new keys here rather than overloading this one.
 */
export const useFeatureFlagsStore = defineStore('featureFlags', () => {
  const flags = ref({
    wiki_wysiwyg: false,
  })
  const loaded = ref(false)
  const loadError = ref('')
  let loadingPromise: Promise<void> | null = null

  const ensureLoaded = async (force = false): Promise<void> => {
    if (loaded.value && !force) return
    if (loadingPromise) return loadingPromise

    loadingPromise = (async () => {
      try {
        const response = await getFeatures()
        const data = response?.data
        if (data && typeof data === 'object' && 'flags' in data) {
          flags.value = {
            wiki_wysiwyg: Boolean(data.flags?.wiki_wysiwyg),
          }
          loadError.value = ''
        } else {
          flags.value = { wiki_wysiwyg: false }
          loadError.value = 'unexpected response shape'
        }
      } catch (error) {
        // Fail-open: every flag defaults to false so the UI degrades
        // to the legacy markdown path rather than blocking on a 5xx.
        flags.value = { wiki_wysiwyg: false }
        loadError.value = error instanceof Error ? error.message : String(error)
      } finally {
        loaded.value = true
        loadingPromise = null
      }
    })()

    return loadingPromise
  }

  return {
    flags,
    loaded,
    loadError,
    ensureLoaded,
  }
})