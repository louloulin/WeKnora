<!--
  CollabDocThemePanel.vue — DOC 设计 tab 主题面板（v0.7.78）

  移植自 PPT CollabSlideThemePanel.vue，8 个主题色板（Office / Ember / Indigo /
  Forest / Cream / Rose / Graphite / Midnight），emit 'apply' 事件携带 theme id，
  父级 DOC 编辑器将其应用到 .collab-doc-pro__surface 的 CSS 变量。
-->
<template>
  <div class="collab-doc-theme-panel" data-testid="doc-theme-panel">
    <div class="collab-doc-theme-panel__title">文档主题</div>
    <div class="collab-doc-theme-panel__grid">
      <button
        v-for="theme in themes"
        :key="theme.id"
        class="collab-doc-theme-panel__swatch"
        :class="{ active: activeTheme === theme.id }"
        :data-testid="`doc-theme-${theme.id}`"
        @click="onPick(theme.id)"
      >
        <div class="collab-doc-theme-panel__swatch-strip">
          <span :style="{ background: theme.accent }" />
          <span :style="{ background: theme.bg }" />
          <span :style="{ background: theme.fg }" />
        </div>
        <span class="collab-doc-theme-panel__swatch-label">{{ theme.name }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
interface DocThemePreset {
  id: 'office' | 'ember' | 'indigo' | 'forest' | 'cream' | 'rose' | 'graphite' | 'midnight'
  name: string
  accent: string
  bg: string
  fg: string
}

const DOC_THEMES: DocThemePreset[] = [
  { id: 'office',   name: 'Office',   accent: '#5aa8ff', bg: '#ffffff', fg: '#1a1a1a' },
  { id: 'ember',    name: 'Ember',    accent: '#f06b3f', bg: '#fffaf6', fg: '#1a1a1a' },
  { id: 'indigo',   name: 'Indigo',   accent: '#6366f1', bg: '#f5f5ff', fg: '#1a1a1a' },
  { id: 'forest',   name: 'Forest',   accent: '#2f9e44', bg: '#f4faf4', fg: '#1a1a1a' },
  { id: 'cream',    name: 'Cream',    accent: '#b58863', bg: '#fdf6e3', fg: '#1a1a1a' },
  { id: 'rose',     name: 'Rose',     accent: '#e64980', bg: '#fff0f6', fg: '#1a1a1a' },
  { id: 'graphite', name: 'Graphite', accent: '#adb5bd', bg: '#f8f9fa', fg: '#1a1a1a' },
  { id: 'midnight', name: 'Midnight', accent: '#7c3aed', bg: '#1e1b4b', fg: '#e9ecef' },
]

const props = defineProps<{
  activeTheme?: DocThemePreset['id']
}>()

const emit = defineEmits<{
  (e: 'apply', id: DocThemePreset['id']): void
}>()

const themes = DOC_THEMES

function onPick(id: DocThemePreset['id']) {
  emit('apply', id)
}
</script>

<style scoped>
.collab-doc-theme-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border-radius: 6px;
  background: var(--app-surface-raised, #1d212a);
  border: 1px solid var(--app-border, #2c313b);
  min-width: 320px;
}
.collab-doc-theme-panel__title {
  font-size: 11px;
  font-weight: 600;
  color: #7c8696;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  user-select: none;
}
.collab-doc-theme-panel__grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}
.collab-doc-theme-panel__swatch {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 6px;
  border-radius: 4px;
  border: 2px solid transparent;
  background: var(--app-surface, #161a22);
  cursor: pointer;
  text-align: left;
  transition: border-color 120ms ease, transform 120ms ease;
}
.collab-doc-theme-panel__swatch:hover {
  border-color: rgba(90, 168, 255, 0.4);
  transform: translateY(-1px);
}
.collab-doc-theme-panel__swatch.active {
  border-color: var(--td-brand-color, #5aa8ff);
  background: rgba(90, 168, 255, 0.08);
}
.collab-doc-theme-panel__swatch-strip {
  display: flex;
  height: 24px;
  border-radius: 3px;
  overflow: hidden;
  border: 1px solid rgba(0, 0, 0, 0.2);
}
.collab-doc-theme-panel__swatch-strip span {
  flex: 1;
}
.collab-doc-theme-panel__swatch-label {
  font-size: 11px;
  color: var(--app-text, #dce4ed);
  text-align: center;
}
</style>
