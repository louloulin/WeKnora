<!--
  CollabSlidesView — v0.7.37 Build #44 / v0.7.38 Build #46.x.

  Wraps CollabSlideDeckEditor with auth + tenant context. Higher-level
  "演示文稿" surface distinct from per-page collab-doc editor.
-->
<template>
  <div class="collab-slides-view">
    <main class="collab-slides-view__main">
      <div class="collab-slides-view__heading"><div><span class="collab-slides-view__eyebrow">WORKSPACE / PRESENTATIONS</span><h1>演示文稿</h1><p>创建并协作编辑团队演示文稿。</p></div><span class="collab-slides-view__status"><i></i>实时协作已就绪</span></div>
      <CollabSlideDeckEditor />
    </main>
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
.collab-slides-view { min-height:100%; padding:32px clamp(16px, 4vw, 56px) 48px; box-sizing:border-box; background:var(--app-page-bg); color:var(--app-text); display:grid; gap:20px; grid-template-columns:minmax(0, 1fr) 320px; align-items:start; }
.collab-slides-view__main { min-width:0; max-width:1180px; width:100%; justify-self:end; } .collab-slides-view__heading { display:flex; align-items:flex-end; justify-content:space-between; gap:16px; margin:0 0 14px; } .collab-slides-view__eyebrow { display:block; color:var(--td-brand-color); font-size:10px; font-weight:700; letter-spacing:.14em; margin-bottom:8px; } .collab-slides-view__heading h1 { margin:0 0 5px; font-size:26px; letter-spacing:-.03em; } .collab-slides-view__heading p { margin:0; color:var(--app-text-muted); font-size:13px; } .collab-slides-view__status { display:flex; align-items:center; gap:7px; color:var(--app-text-muted); font-size:12px; white-space:nowrap; } .collab-slides-view__status i { width:7px; height:7px; border-radius:50%; background:var(--td-brand-color); }
.collab-slides-view__theme-aside { position:sticky; top:16px; padding:10px; border:1px solid var(--app-border); border-radius:10px; background:var(--app-surface-bg); box-shadow:0 10px 26px rgba(0,0,0,.14); }
@media (max-width: 900px) {
  .collab-slides-view { grid-template-columns: minmax(0, 1fr); padding:24px 14px 40px; } .collab-slides-view__theme-aside { position:static; }
}
</style>
