<!--
  CollabSlideThemePanel.vue — Design tab theme gallery (v0.7.92).

  Vendored from genoffice/apps/slides/src/renderer/themes.ts with the
  preset list exposed as a swatch grid. Picking a theme pushes its color
  scheme into the active slide deck via a `theme:apply` event so the
  parent SLIDE editor can re-skin explicit srgbClr fills + theme*.xml
  references through the same shape that PPTX uses.

  Why a separate component: genoffice's Design tab lives in the ribbon,
  WeKnora's editor exposes its Design actions through the slide-context
  toolbar. Keeping the panel isolated lets the toolbar stay narrow while
  this grid takes the full vertical height.
-->
<template>
  <div class="collab-slide-theme-panel" data-testid="slide-theme-panel">
    <div class="collab-slide-theme-panel__title">主题 (Themes)</div>
    <div class="collab-slide-theme-panel__grid">
      <button
        v-for="preset in presets"
        :key="preset.id"
        class="collab-slide-theme-panel__swatch"
        :class="{ active: activeId === preset.id }"
        :data-testid="`slide-theme-${preset.id}`"
        @click="onApply(preset)"
      >
        <div class="collab-slide-theme-panel__swatch-strip">
          <span :style="{ background: '#' + preset.colors.dk1 }" />
          <span :style="{ background: '#' + preset.colors.lt1 }" />
          <span :style="{ background: '#' + preset.colors.accent1 }" />
          <span :style="{ background: '#' + preset.colors.accent2 }" />
          <span :style="{ background: '#' + preset.colors.accent3 }" />
          <span :style="{ background: '#' + preset.colors.accent4 }" />
        </div>
        <span class="collab-slide-theme-panel__swatch-label">{{ preset.name }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { THEME_PRESETS, type SlideThemePreset } from '../../editor/slides/themes/genofficeThemes'

const props = defineProps<{
  initialId?: string
}>()

const emit = defineEmits<{
  (e: 'theme:apply', preset: SlideThemePreset): void
}>()

const presets = THEME_PRESETS
const activeId = ref<string>(props.initialId ?? '')

function onApply(preset: SlideThemePreset) {
  activeId.value = preset.id
  emit('theme:apply', preset)
}
</script>

<style scoped>
.collab-slide-theme-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border-radius: 8px;
  background: var(--collab-surface, #fff);
  border: 1px solid var(--collab-border, #e2e8f0);
}
.collab-slide-theme-panel__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--collab-text, #1e293b);
}
.collab-slide-theme-panel__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}
.collab-slide-theme-panel__swatch {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px;
  border-radius: 6px;
  border: 1px solid transparent;
  background: var(--collab-surface-2, #f8fafc);
  cursor: pointer;
  text-align: left;
  transition: border-color 120ms ease;
}
.collab-slide-theme-panel__swatch:hover {
  border-color: var(--collab-border-strong, #94a3b8);
}
.collab-slide-theme-panel__swatch.active {
  border-color: var(--collab-accent, #2563eb);
}
.collab-slide-theme-panel__swatch-strip {
  display: flex;
  height: 22px;
  border-radius: 4px;
  overflow: hidden;
}
.collab-slide-theme-panel__swatch-strip span {
  flex: 1;
}
.collab-slide-theme-panel__swatch-label {
  font-size: 12px;
  color: var(--collab-text, #1e293b);
}
</style>
