<!--
  CollabEditorRibbon.vue — shared tabbed Ribbon used by DOC / SHEET / SLIDE
  editors. Pattern lifted from GenOffice `apps/docs/src/renderer/Ribbon.tsx`:
  a horizontal tab strip selects one panel; the default slot is the active
  panel content. Editors contribute their own toolbar-group nodes and
  toggle visibility with v-show="activeTab === 'xxx'".

  Keeping this as a tiny wrapper (no built-in toolbar groups) so each
  editor keeps full control over its commands and dark surface — same
  architecture as GenOffice's per-tab child components
  (RibbonHomeTab / RibbonInsertTab / ...).
-->
<template>
  <nav
    class="collab-editor-ribbon"
    :data-rb-theme="theme"
    :aria-label="ariaLabel"
    role="tablist"
  >
    <div class="collab-editor-ribbon__tabbar">
      <div ref="tabsContainerRef" class="collab-editor-ribbon__tabs ribbon-tabs" :style="{ '--rb-underline-left': underlineStyle.left, '--rb-underline-width': underlineStyle.width }">
        <button
          v-for="t in tabs"
          :key="t.id"
          type="button"
          role="tab"
          class="collab-editor-ribbon__tab"
          :class="{ 'is-active': modelValue === t.id }"
          :aria-selected="modelValue === t.id"
          :data-testid="`${testIdPrefix}-tab-${t.id}`"
          @click="$emit('update:modelValue', t.id)"
        >{{ t.label }}</button>
      </div>
      <button
        v-if="collapsible"
        class="collab-editor-ribbon__fold"
        type="button"
        :title="collapsed ? '展开 Ribbon' : '折叠 Ribbon'"
        :data-testid="`${testIdPrefix}-fold`"
        @click="toggle"
      >{{ collapsed ? '展开' : '折叠' }}</button>
    </div>
    <div class="collab-editor-ribbon__panel ribbon-body" role="tabpanel" v-show="!collapsed">
      <slot v-if="$slots[modelValue]" :name="modelValue" :active-tab="modelValue" />
      <slot v-else :active-tab="modelValue" />
    </div>
  </nav>


</template>

<script setup lang="ts">
export interface RibbonTab {
  id: string
  label: string
}

import { ref, watch, onMounted } from 'vue'

const props = withDefaults(defineProps<{
  tabs: RibbonTab[]
  modelValue: string
  ariaLabel?: string
  testIdPrefix?: string
  collapsible?: boolean
  storageKey?: string
  /** 'dark' | 'light' — caller decides based on document/slide surface. */
  theme?: 'dark' | 'light'
}>(), {
  collapsible: false,
  storageKey: 'collab-editor-ribbon-collapsed',
  theme: 'light',
})

const collapsed = ref(false)
onMounted(() => {
  try {
    const v = localStorage.getItem(props.storageKey)
    if (v === '1') collapsed.value = true
  } catch {}
})
watch(collapsed, (v) => {
  try { localStorage.setItem(props.storageKey, v ? '1' : '0') } catch {}
})
const toggle = () => { collapsed.value = !collapsed.value }

defineEmits<{
  (e: 'update:modelValue', id: string): void
}>()

/* v0.7.131 — 共享 active-tab 下划线位置：由 active tab 的 DOM 矩形换算到
 * .collab-editor-ribbon__tabs 容器内的 left/width，::after 平滑滑动过去。
 * 用 inset 13px 模拟 GenOffice 「精准对齐到文字宽度」的视觉。 */
const tabsContainerRef = ref<HTMLElement | null>(null)
const underlineStyle = ref<{ left: string; width: string; right: string }>({
  left: '13px',
  width: '0px',
  right: 'auto',
})

const updateUnderline = () => {
  const container = tabsContainerRef.value
  if (!container) return
  const active = container.querySelector<HTMLElement>('.collab-editor-ribbon__tab.is-active')
  if (!active) return
  // 用 getBoundingClientRect 拿到 active tab 在视口的 x 范围，
  // 再减去容器自身的 left，得到容器坐标系下的 left/width。
  const containerRect = container.getBoundingClientRect()
  const activeRect = active.getBoundingClientRect()
  const padLeft = 13 // 与 .collab-editor-ribbon__tab padding-x 对齐
  underlineStyle.value = {
    left: `${activeRect.left - containerRect.left + padLeft}px`,
    width: `${activeRect.width - padLeft * 2}px`,
    right: 'auto',
  }
}

onMounted(() => {
  updateUnderline()
  window.addEventListener('resize', updateUnderline)
})
import { onBeforeUnmount } from 'vue'
onBeforeUnmount(() => window.removeEventListener('resize', updateUnderline))

// 切换 tab 后等下一帧让新 active tab 完成 layout 再算位置
watch(() => props.modelValue, () => {
  nextTick(() => updateUnderline())
})
import { nextTick } from 'vue'
</script>

<style scoped>
/* v0.7.79 — Ribbon 全面对齐 GenOffice Word for Mac (`apps/docs/styles.css`):
 * - 品牌色 = --word-blue = #185abd（Office 经典蓝，非 TDesign 默认绿）
 * - Tab strip 紧凑：30px 高、13px font-size、semibold 全员
 * - Active tab 用 ::after 精准下划线（inset 13px，2.5px 高）
 * - Body 限高 92px，overflow-x: scroll（窄屏挤压不换行）
 * - 大按钮图标 28px（GenOffice .rb-big-icon 标准）
 * - Group label 默认隐藏（Word for Mac 不显示组标题）
 * - 编辑器浅色 surface + 边框替代深色 chrome
 */
.collab-editor-ribbon {
  --rb-accent: #4d8df6;
  --rb-accent-hover: #6ba1ff;
  --rb-accent-soft: rgba(77, 141, 246, 0.18);
  --rb-accent-strong: rgba(77, 141, 246, 0.28);
  --rb-chrome-bg: #f6f7fa;
  --rb-chrome-bg-deep: #eceef3;
  --rb-tab-strip-bg: #e8eaef;
  --rb-text: #1f232b;
  --rb-text-dim: #6a7382;
  --rb-icon: #4d5560;
  --rb-icon-hover: #1f232b;
  --rb-border: #d8dce2;
  --rb-border-strong: #c5cad2;
  --rb-hover: rgba(77, 141, 246, 0.10);
  --rb-pressed: rgba(77, 141, 246, 0.18);
  --rb-sep: rgba(0, 0, 0, 0.08);
  --rb-tooltip-bg: #1f232b;
  --rb-tooltip-text: #f4f6fa;
  --rb-drop-bg: #ffffff;
  --rb-drop-border: #c5cad2;
  --rb-drop-text: #1f232b;
  --rb-drop-text-dim: #6a7382;
  --rb-divider: rgba(0, 0, 0, 0.06);
}
/* v0.7.119 — auto-dark when slide canvas is dark (data-rb-theme="dark"
   set by CollabSlideKonvaEditor on first frame). Uses common dark-UI
   palette tokens; iconography & spacing stay the same. */
.collab-editor-ribbon[data-rb-theme="dark"] {
  --rb-accent: #6ba1ff;
  --rb-accent-hover: #8db6ff;
  --rb-accent-soft: rgba(107, 161, 255, 0.22);
  --rb-accent-strong: rgba(107, 161, 255, 0.34);
  /* v0.7.133 — match GenOffice neutral grays: tab strip #2a2a2a (lighter),
     background #252525 (panel slightly darker), drops the old cool-blue tint. */
  --rb-chrome-bg: #252525;
  --rb-chrome-bg-deep: #1e1e1e;
  --rb-tab-strip-bg: #2a2a2a;
  --rb-text: #e6eaf2;
  --rb-text-dim: #8b95a4;
  --rb-icon: #b8c0cc;
  --rb-icon-hover: #ffffff;
  --rb-border: #3a3a3a;
  --rb-border-strong: #454545;
  --rb-hover: rgba(255, 255, 255, 0.07);
  --rb-pressed: rgba(107, 161, 255, 0.22);
  --rb-sep: rgba(255, 255, 255, 0.08);
  --rb-divider: rgba(255, 255, 255, 0.06);
  --rb-tooltip-bg: #0e1116;
  --rb-tooltip-text: #f4f6fa;
  --rb-drop-bg: #2a2a2a;
  --rb-drop-border: #3a3a3a;
  --rb-drop-text: #e6eaf2;
  --rb-drop-text-dim: #8b95a4;
}

.collab-editor-ribbon {
  display: flex;
  flex-direction: column;
  background: var(--rb-chrome-bg);
  border-bottom: 1px solid var(--rb-border-strong);
  color: var(--rb-text);
  flex-shrink: 0;
  user-select: none;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
}

.collab-editor-ribbon__tabbar {
  display: flex;
  align-items: stretch;
  min-height: 35px;
  background: var(--rb-tab-strip-bg);
  border-bottom: 1px solid var(--rb-border);
}

/* --- tab strip --- */
.collab-editor-ribbon__tabs {
  flex: 1 1 auto;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px 10px 0;
  overflow-x: auto;
  min-height: 30px;
  /* v0.7.131 — 共享滑动下划线的定位上下文 */
  position: relative;
}
/* v0.7.131 — 共享滑动下划线：JS 计算 active tab 位置，smooth 滑动。
 * left/width 由 :style 注入的 --rb-underline-left/--rb-underline-width 控制。
 * 匹配 GenOffice 「active tab 用 2.5px 下划线」的风格。 */
.collab-editor-ribbon__tabs::after {
  content: '';
  position: absolute;
  left: var(--rb-underline-left, 13px);
  width: var(--rb-underline-width, 0px);
  bottom: -2.5px;
  height: 2.5px;
  background: var(--rb-accent);
  border-radius: 1px;
  pointer-events: none;
  z-index: 2;
  transition: left 220ms cubic-bezier(0.4, 0, 0.2, 1),
              width 220ms cubic-bezier(0.4, 0, 0.2, 1);
}
.collab-editor-ribbon__tab {
  position: relative;
  border: 0;
  background: transparent;
  padding: 5px 13px 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--rb-text-dim);
  cursor: pointer;
  white-space: nowrap;
  border-radius: 3px 3px 0 0;
  font-family: inherit;
  transition: background-color 120ms ease, color 120ms ease;
}
.collab-editor-ribbon__tab:hover {
  /* GenOffice pattern: hover = color-only */
  color: var(--rb-accent);
}
.collab-editor-ribbon__tab.is-active {
  /* GenOffice pattern: active tab uses color-only (var(--accent)),
   * not background highlight. The shared ::after on .tabs carries the underline. */
  color: var(--rb-accent);
}

/* --- fold button (right side of tab strip) --- */
.collab-editor-ribbon__fold {
  align-self: center;
  flex: 0 0 auto;
  margin: 0 10px 0 4px;
  padding: 4px 8px;
  background: transparent;
  border: 1px solid transparent;
  color: var(--rb-text-dim);
  cursor: pointer;
  font-size: 11px;
  border-radius: 4px;
}
.collab-editor-ribbon__fold:hover {
  color: var(--rb-accent);
  border-color: var(--rb-border);
  background: var(--rb-hover);
}

/* --- panel (active tab content) --- */
.collab-editor-ribbon__panel {
  display: flex;
  align-items: stretch;
  gap: 0;
  /* GenOffice PPT ribbon-body: 80px tall band so .rb-big (icon row 28px + 4px
   * gap + label) fits cleanly. overflow-x: scroll keeps narrow viewports
   * scrollable instead of wrapping the toolbar. */
  padding: 0 10px;
  height: 80px;
  min-height: 80px;
  max-height: 80px;
  overflow-x: auto;
  overflow-y: hidden;
  background: var(--rb-chrome-bg);
}
/* styled slim scrollbar — 5px 高，鼠标滚轮可达溢出 group */
.collab-editor-ribbon__panel::-webkit-scrollbar {
  height: 5px;
}
.collab-editor-ribbon__panel::-webkit-scrollbar-track {
  background: transparent;
}
.collab-editor-ribbon__panel::-webkit-scrollbar-thumb {
  border-radius: 2.5px;
  background: var(--rb-border-strong);
}

/* --- separator between groups --- */
.collab-editor-ribbon__panel > * + * {
  border-left: 0;
}
.collab-editor-ribbon.is-collapsed .collab-editor-ribbon__panel {
  display: none;
}

/* --- Ribbon content primitives (lifted from GenOffice apps/slides/src/renderer/styles.css) --- */
.ribbon-group {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  justify-content: flex-start;
  height: 100%;
  padding: 2px 4px 0;
  flex-shrink: 0;
  min-width: 0;
  color: var(--rb-text);
}
.ribbon-group > .ribbon-group-items { flex: 1 1 auto; min-height: 0; }
.ribbon-group > .ribbon-group-label--visible { flex: 0 0 auto; align-self: stretch; text-align: center; }
.ribbon-group-items {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: 1;
}
/* GenOffice convention: powerpoint-density ribbons hide the inline group
 * label so the 80px band stays compact. Editors that still ship a visible
 * label via `.ribbon-group-label--visible` can opt back in. */
.ribbon-group-label { display: none; }
.ribbon-group-label--visible {
  display: block;
  text-align: center;
  font-size: 10px;
  line-height: 13px;
  color: var(--rb-text-dim);
  letter-spacing: 0.02em;
  padding: 0 4px;
}
.ribbon-sep {
  /* v0.7.131 — hide explicit sep divs: they collapsed to 0 width inside
   * .collab-slide-konva__ribbon-groups (flex stretch) and produced double
   * lines next to the border-left separator. Use border-left on the next
   * tool-group as the single source of truth (GenOffice pattern). */
  display: none;
}

/* big button (icon-on-top + label-below): min-width comes from label length,
 * the icon row stretches to fill the 80px band. PowerPoint density. */
.rb-big {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  border: none;
  background: none;
  border-radius: 4px;
  padding: 4px 7px 6px;
  font-size: 12px;
  cursor: pointer;
  color: var(--rb-text);
  transition: background-color 120ms ease;
  /* Don't compress button width when space is tight; otherwise CJK labels
   * wrap character-by-character (table → 垂直). */
  flex-shrink: 0;
  white-space: nowrap;
  font-family: inherit;
}
/* The icon row stretches across the button width and gets a hover/active
 * tint. Bleed (-4px) cancels the 7px horizontal padding so layout is unchanged
 * while the tint spans the button box. */
.rb-big-icon {
  align-self: stretch;
  min-height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
  border-radius: 4px;
  padding: 3px 4px;
  margin: 0 -4px;
  color: var(--rb-text);
}
.rb-big-icon :deep(svg) {
  display: block;
  /* force-override the SVG `width`/`height` attributes written by CollabIcon */
  width: 24px !important;
  height: 24px !important;
  flex-shrink: 0;
  pointer-events: none;
}
.rb-big:hover:not(:disabled) .rb-big-icon { background: var(--rb-hover); color: var(--rb-icon-hover); }
.rb-big:disabled { opacity: 0.4; cursor: default; }
.rb-big.active .rb-big-icon { background: var(--rb-pressed); }
.rb-big.active .rb-big-icon :deep(svg) { color: var(--rb-accent); }

/* small button (icon-left + label-right): compact toolbar row entry */
.rb-small {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: none;
  background: none;
  border-radius: 3px;
  padding: 3px 8px;
  font-size: 12px;
  cursor: pointer;
  color: var(--rb-text);
  white-space: nowrap;
  flex-shrink: 0;
  font-family: inherit;
}
.rb-small :deep(svg) {
  flex-shrink: 0;
  width: 16px !important;
  height: 16px !important;
  color: var(--rb-text);
  pointer-events: none;
}
.rb-small:hover:not(:disabled) { background: var(--rb-hover); color: var(--rb-icon-hover); }
.rb-small.active { background: var(--rb-pressed); color: var(--rb-accent); }
.rb-small:disabled { opacity: 0.4; cursor: default; }

/* square icon-only button */
.rb-icon {
  min-width: 28px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: none;
  border-radius: 3px;
  padding: 0 4px;
  font-size: 15px;
  cursor: pointer;
  color: var(--rb-icon);
  transition: background-color 120ms ease, color 120ms ease;
  font-family: inherit;
}
.rb-icon:hover:not(:disabled) { background: var(--rb-hover); color: var(--rb-icon-hover); }
.rb-icon.active { background: var(--rb-pressed); color: var(--rb-accent); }
.rb-icon:disabled { opacity: 0.35; cursor: default; }
.rb-icon :deep(svg) {
  display: block;
  width: 18px !important;
  height: 18px !important;
  pointer-events: none;
}

/* tonal variant used by the present-mode entry — green wash to signal "play" */
.rb-big.rb-big--present .rb-big-icon { background: color-mix(in srgb, #16a34a 14%, transparent); }
.rb-big.rb-big--present:hover:not(:disabled) .rb-big-icon { background: color-mix(in srgb, #16a34a 22%, transparent); }

/* Thin dropdown chevron (matches GenOffice ribbon-shared.tsx <RbCaret/>).
   Used inside .rb-big-icon (next to the icon) or .rb-small right-of-label. */
.rb-caret, :deep(.rb-caret) {
  flex-shrink: 0;
  display: block;
  width: 10px !important;
  height: 10px !important;
  color: var(--rb-text-dim);
  margin-left: 4px;
  pointer-events: none;
}
.rb-big-icon > :deep(.rb-caret) { margin-left: 3px; color: var(--rb-text-dim); }
.rb-small > :deep(.rb-caret) { margin-left: 1px; }
.rb-big:hover:not(:disabled) > .rb-big-icon :deep(.rb-caret),
.rb-big.active > .rb-big-icon :deep(.rb-caret) {
  color: var(--rb-accent);
}

/* When a button hosts a chevron, the icon-row text and chevron share the
   28px icon row horizontally; flex the row instead of stacking. */
.rb-big-icon.is-with-caret {
  flex-direction: row;
  gap: 2px;
  padding: 3px 4px;
}

/* ===== ScreenTip (Office-style hover tooltip) =====
 * Pure-CSS fallback using data-tip: tip pops below the button on hover.
 * Direction controlled by [data-tip-pos="top"|"right"|"bottom"|"left"];
 * default is bottom. For full Office parity (delay, reshow, kbd/detail) the
 * page can upgrade to a delegated handler; the basic behavior matches
 * expectations and never shows the slow native title tooltip because we
 * strip `title` from buttons that have data-tip. */
[data-tip] { position: relative; }
[data-tip]:hover::after,
[data-tip]:focus-visible::after {
  content: attr(data-tip);
  position: absolute;
  font-size: 12px;
  line-height: 1.4;
  padding: 4px 8px;
  background: var(--rb-tooltip-bg);
  color: var(--rb-tooltip-text);
  border-radius: 4px;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.28);
  pointer-events: none;
  z-index: 9999;
  opacity: 0;
  animation: rb-tip-show 200ms forwards;
  animation-delay: 350ms;
}
[data-tip]:hover::before,
[data-tip]:focus-visible::before {
  content: '';
  position: absolute;
  border: 4px solid transparent;
  pointer-events: none;
  z-index: 9999;
  opacity: 0;
  animation: rb-tip-show 200ms forwards;
  animation-delay: 350ms;
}

/* Default: bottom (tip + up-arrow below the button). */
[data-tip]:not([data-tip-pos])::after,
[data-tip][data-tip-pos="bottom"]::after {
  left: 50%;
  top: calc(100% + 8px);
  transform: translateX(-50%);
  white-space: nowrap;
}
[data-tip]:not([data-tip-pos])::before,
[data-tip][data-tip-pos="bottom"]::before {
  left: 50%;
  top: 100%;
  transform: translateX(-50%);
  border-bottom-color: var(--rb-tooltip-bg);
}
/* Top (tip + down-arrow above the button). */
[data-tip][data-tip-pos="top"]::after {
  left: 50%;
  bottom: calc(100% + 8px);
  transform: translateX(-50%);
  white-space: nowrap;
}
[data-tip][data-tip-pos="top"]::before {
  left: 50%;
  bottom: 100%;
  transform: translateX(-50%);
  border-top-color: var(--rb-tooltip-bg);
}
/* Right (tip + left-arrow to the right of the button). */
[data-tip][data-tip-pos="right"]::after {
  left: calc(100% + 8px);
  top: 50%;
  transform: translateY(-50%);
}
[data-tip][data-tip-pos="right"]::before {
  left: 100%;
  top: 50%;
  transform: translateY(-50%);
  border-left-color: var(--rb-tooltip-bg);
}
/* Left (tip + right-arrow to the left of the button). */
[data-tip][data-tip-pos="left"]::after {
  right: calc(100% + 8px);
  top: 50%;
  transform: translateY(-50%);
}
[data-tip][data-tip-pos="left"]::before {
  right: 100%;
  top: 50%;
  transform: translateY(-50%);
  border-right-color: var(--rb-tooltip-bg);
}
@keyframes rb-tip-show {
  to { opacity: 1; }
}
</style>

<!-- Global rb-* primitives — non-scoped so they apply to .rb-big/.rb-small/.rb-icon
     buttons rendered by parent editors (CollabSlideKonvaEditor etc.). The scoped
     `<style scoped>` block above only covers elements in this component's
     template (tabs, panel wrapper); child editors must put their buttons
     inside the slot and those are tagged with their own component scope id,
     not CollabEditorRibbon's. -->
<style>
.rb-big {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  border: none;
  background: none;
  border-radius: 4px;
  padding: 4px 7px 6px;
  font-size: 12px;
  cursor: pointer;
  color: inherit;
  flex-shrink: 0;
  white-space: nowrap;
  font-family: inherit;
}
.rb-big-icon {
  align-self: stretch;
  min-height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
  border-radius: 4px;
  padding: 3px 4px;
  margin: 0 -4px;
  color: var(--rb-text);
}
.rb-big-icon svg,
.rb-big-icon > svg {
  display: block;
  width: 24px !important;
  height: 24px !important;
  flex-shrink: 0;
  pointer-events: none;
}
.rb-big:hover:not(:disabled) .rb-big-icon { background: var(--rb-hover); }
.rb-big:disabled { opacity: 0.4; cursor: default; }
.rb-big.active .rb-big-icon { background: var(--rb-pressed); }
.rb-big.active .rb-big-icon svg { color: var(--rb-accent); }

.rb-small {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: none;
  background: none;
  border-radius: 3px;
  padding: 3px 8px;
  font-size: 12px;
  cursor: pointer;
  color: var(--rb-text);
  white-space: nowrap;
  flex-shrink: 0;
  font-family: inherit;
}
.rb-small svg,
.rb-small > svg {
  flex-shrink: 0;
  width: 16px !important;
  height: 16px !important;
  color: var(--rb-text);
  pointer-events: none;
}
.rb-small:hover:not(:disabled) { background: var(--rb-hover); }
.rb-small.active { background: var(--rb-pressed); color: var(--rb-accent); }
.rb-small:disabled { opacity: 0.4; cursor: default; }

.rb-icon {
  min-width: 28px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: none;
  border-radius: 3px;
  padding: 0 4px;
  font-size: 15px;
  cursor: pointer;
  color: inherit;
  font-family: inherit;
}
.rb-icon svg,
.rb-icon > svg {
  display: block;
  width: 18px !important;
  height: 18px !important;
  pointer-events: none;
}
.rb-icon:hover:not(:disabled) { background: var(--rb-hover); }
.rb-icon.active { background: var(--rb-pressed); color: var(--rb-accent); }
.rb-icon:disabled { opacity: 0.35; cursor: default; }

.rb-big.rb-big--present .rb-big-icon { background: color-mix(in srgb, #16a34a 14%, transparent); }
.rb-big.rb-big--present:hover:not(:disabled) .rb-big-icon { background: color-mix(in srgb, #16a34a 22%, transparent); }

/* Thin dropdown chevron (matches GenOffice ribbon-shared.tsx <RbCaret/>). */
.rb-caret {
  flex-shrink: 0;
  display: block;
  width: 10px !important;
  height: 10px !important;
  color: var(--rb-text-dim);
  margin-left: 4px;
  pointer-events: none;
}
.rb-big-icon > .rb-caret { margin-left: 3px; color: var(--rb-text-dim); }
.rb-small > .rb-caret { margin-left: 1px; }
.rb-big:hover:not(:disabled) > .rb-big-icon .rb-caret,
.rb-big.active > .rb-big-icon .rb-caret { color: var(--rb-accent); }

.rb-big-icon.is-with-caret { flex-direction: row; gap: 2px; padding: 3px 4px; }

/* ===== ScreenTip (Office-style hover tooltip) ===== */
[data-tip] { position: relative; }
[data-tip]:hover::after,
[data-tip]:focus-visible::after {
  content: attr(data-tip);
  position: absolute;
  left: 50%;
  top: calc(100% + 6px);
  transform: translateX(-50%);
  white-space: nowrap;
  font-size: 12px;
  line-height: 1.4;
  padding: 4px 8px;
  background: var(--rb-text);
  color: var(--rb-chrome-bg);
  border-radius: 4px;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.18);
  pointer-events: none;
  z-index: 9999;
  opacity: 0;
  animation: rb-tip-show 200ms forwards;
  animation-delay: 350ms;
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
}
[data-tip]:hover::before,
[data-tip]:focus-visible::before {
  content: '';
  position: absolute;
  left: 50%;
  top: 100%;
  transform: translateX(-50%);
  border: 4px solid transparent;
  border-bottom-color: var(--rb-text);
  pointer-events: none;
  z-index: 9999;
  opacity: 0;
  animation: rb-tip-show 200ms forwards;
  animation-delay: 350ms;
}
@keyframes rb-tip-show { to { opacity: 1; } }

/* ===== Split-button pattern (GenOffice rb-split) =====
   A `.rb-split` button is divided into two zones:
   - `.rb-split-main`: the main click action
   - `.rb-caret-hit`: the dropdown arrow (separate click target)
   Both share the icon-row wash on hover but each zone darkens a step
   when individually hovered, mirroring the PowerPoint split-button feel. */
.rb-split .rb-big-icon {
  gap: 0;
  padding: 0;
}
.rb-split .rb-split-main,
.rb-split .rb-caret-hit {
  display: inline-flex;
  align-items: center;
  align-self: stretch;
  border-radius: 0;
}
.rb-split .rb-split-main {
  padding: 3px 4px 3px 6px;
  border-radius: 4px 0 0 4px;
}
.rb-split .rb-caret-hit {
  padding: 3px 5px 3px 3px;
  border-radius: 0 4px 4px 0;
  cursor: pointer;
  color: var(--rb-text-dim);
}
.rb-split:hover:not(:disabled) .rb-big-icon { background: none; }
.rb-split:hover:not(:disabled) .rb-split-main,
.rb-split:hover:not(:disabled) .rb-caret-hit { background: var(--rb-hover); }
.rb-split:not(:disabled) .rb-split-main:hover,
.rb-split:not(:disabled) .rb-caret-hit:hover,
.rb-split .rb-caret-hit.active { background: var(--rb-pressed); }
.rb-split .rb-caret-hit.active { color: var(--rb-accent); }

/* v0.7.119 — Slide-ribbon home-tab additions (WeKnora-local). The split-button
   row below stacks a layout picker + add-section button under the big
   新建幻灯片 action, matching PowerPoint's home-tab grouping. */
.rb-slides-col {
  display: flex;
  flex-direction: column;
  gap: 3px;
  align-self: stretch;
  justify-content: stretch;
  min-width: 0;
}
.rb-slides-col .rb-small {
  justify-content: flex-start;
  text-align: left;
}

/* Alignment grid (2 columns × 3 rows = L/C/R × T/M/B), then a 1-row
   distribute + group + flip band. Each cell is a compact 28px button. */
.rb-arrange-grid {
  display: grid;
  grid-template-columns: repeat(3, 28px);
  grid-auto-rows: 22px;
  gap: 2px;
  padding: 2px 0 4px;
}
.rb-arrange-grid .rb-icon {
  min-width: 28px;
  height: 22px;
  padding: 0;
}
.rb-arrange-row {
  display: flex;
  gap: 2px;
  padding-top: 2px;
  border-top: 1px solid var(--app-border, rgba(0, 0, 0, 0.06));
  margin-top: 2px;
}

/* Show split-button reuses .rb-big.rb-split — we just add a green tint so the
   whole split reads as the entrypoint to present mode. */
.rb-big.rb-split.rb-show-split .rb-big-icon {
  background: color-mix(in srgb, #16a34a 16%, transparent);
}
.rb-big.rb-split.rb-show-split:hover:not(:disabled) .rb-big-icon,
.rb-big.rb-split.rb-show-split.is-open .rb-big-icon {
  background: color-mix(in srgb, #16a34a 26%, transparent);
}


/* The `.rb-drop-wrap` anchors a popover panel below its trigger.
   The panel positions itself absolutely so it escapes the ribbon
   body's overflow clip. */
.rb-drop-wrap { position: relative; display: inline-flex; align-items: stretch; }
.rb-drop {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  z-index: 80;
  background: var(--rb-drop-bg, #fff);
  border: 1px solid var(--rb-drop-border, #cbd0d7);
  border-radius: 8px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.32), 0 2px 6px rgba(0, 0, 0, 0.18);
  padding: 6px;
  color: var(--rb-drop-text, #1f232b);
  font-size: 12px;
  min-width: 180px;
}
.rb-drop::before {
  content: '';
  position: absolute;
  top: -5px;
  left: 14px;
  width: 10px;
  height: 10px;
  background: var(--rb-drop-bg, #fff);
  border-left: 1px solid var(--rb-drop-border, #cbd0d7);
  border-top: 1px solid var(--rb-drop-border, #cbd0d7);
  border-top-left-radius: 2px;
  transform: rotate(45deg);
}
.rb-drop-title {
  font-size: 11px;
  color: var(--rb-drop-text-dim, #5a6473);
  letter-spacing: 0.04em;
  text-transform: uppercase;
  padding: 4px 10px 6px;
  font-weight: 600;
}
.rb-drop button.rb-drop-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 6px 10px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
  font: inherit;
}
.rb-drop button.rb-drop-item:hover:not(:disabled) { background: var(--app-surface, #f1f5fb); }
.rb-drop button.rb-drop-item.active { background: var(--rb-pressed, rgba(24,90,189,0.16)); color: var(--rb-accent, #185abd); }
.rb-drop button.rb-drop-item:disabled { opacity: 0.4; cursor: default; }
.rb-drop-div { height: 1px; margin: 4px 6px; background: var(--app-border, #e1e4e8); }

/* Right-aligned caret variant: icon-left + label + chevron-right,
   like GenOffice's .rb-small used for dropdowns in collapsed rows. */
.rb-small.is-with-caret { gap: 4px; padding-right: 6px; }
.rb-small .rb-caret { margin-left: 2px; }

</style>
