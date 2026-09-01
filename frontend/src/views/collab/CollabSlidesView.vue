<!--
  CollabSlidesView — v0.7.37 Build #44 / v0.7.38 Build #46.x.

  Wraps CollabSlideDeckEditor with auth + tenant context. Higher-level
  "演示文稿" surface distinct from per-page collab-doc editor.
-->
<template>
  <div class="collab-slides-view">
    <CollabSlideDeckEditor />
    <!-- v0.7.92 — vendored OOXML scheme theme gallery (genoffice
         apps/slides/src/renderer/themes.ts). Lives below the deck list
         so deck-level SlideTheme picker stays first; theme:apply event
         is plumbed to the slide-editor side via a window event so the
         Konva editor (when open) can re-skin explicit fills. -->
    <aside class="collab-slides-view__theme-aside">
      <CollabSlideThemePanel @theme:apply="onThemeApply" />
    </aside>
  </div>
</template>

<script setup lang="ts">
import CollabSlideDeckEditor from '@/components/collab/CollabSlideDeckEditor.vue'
import CollabSlideThemePanel from '@/components/collab/CollabSlideThemePanel.vue'
import type { SlideThemePreset } from '@/editor/slides/themes/genofficeThemes'

function onThemeApply(preset: SlideThemePreset) {
  // Bubble out to the active slide editor (Konva) so its fills follow.
  // Editors listen on 'wk-slide-theme-apply' and remap explicit srgbClr
  // references into the new accent palette.
  window.dispatchEvent(
    new CustomEvent('wk-slide-theme-apply', { detail: preset }),
  )
}
</script>

<style scoped>
.collab-slides-view { padding: 0; display: grid; gap: 16px; grid-template-columns: minmax(0, 1fr) 320px; align-items: start; }
.collab-slides-view__theme-aside { position: sticky; top: 16px; }
@media (max-width: 900px) {
  .collab-slides-view { grid-template-columns: minmax(0, 1fr); }
}
</style>
