<!--
  CollabIcon.vue — Vue 图标库，移植自 GenOffice icons.tsx（142 → 精选 36）
  视觉与原版 1:1（16x16 viewBox + stroke="currentColor" + 自适应 stroke 宽度）
-->
<template>
  <!--
    NOTE: no width/height attributes here. SVG presentation attributes set
    the intrinsic size and beat CSS unless `!important` is used. By omitting
    them, host CSS (.rb-big-icon svg, .rb-small svg, .rb-icon svg) controls
    the rendered size; the SVG's viewBox preserves aspect-ratio scaling.
  -->
  <svg
    viewBox="0 0 16 16"
    :style="{ width: size + 'px', height: size + 'px' }"
    fill="none"
    stroke="currentColor"
    :stroke-width="strokeWidth"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    preserveAspectRatio="xMidYMid meet"
    v-html="content"
  ></svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  name: string
  size?: number
  paint?: number
}>(), { size: 24, paint: undefined })

// `strokeWidth` rescaled by the rendered size; with no width/height attr,
// CSS controls the rendered size, so we treat `props.size` as the default.
// v0.7.153 — 加粗 icon stroke 让 dark theme 上更清晰 (GenOffice spec).
// - size>=24: painted 2.0 (was 1.5) → stroke-width 1.33, paints 2.0px effective
// - size>=20: painted 1.8 (was 1.5) → stroke-width 1.44
// - size>=14: painted 1.4 (was 1.25) → stroke-width 1.6
// - smaller: painted 1.2 (was 1.1)
// v0.7.153 — 加粗 icon stroke 让 dark theme 上更清晰 (GenOffice spec)。
// GenOffice 用 24x24 viewBox, painted 1.5 paints 1.5px effective.
// WeKnora 用 16x16 viewBox, painted 2.25 paints ~1.5px effective。
const strokeWidth = computed(() => {
  // v0.7.158 — bumped visual weight from 2.25/2.0/1.5/1.3 to 2.5/2.25/1.75/1.5
// for GenOffice icon density (Office-style icons read heavier than lucide)
// v0.7.159 — bumped visual weight from 2.5/2.25/1.75/1.5 to 3.0/2.75/2.0/1.75
// for GenOffice icon density match (GenOffice icons 8.4% pixels vs WeKnora 5.2%)
const painted = props.paint ?? (props.size >= 24 ? 3.0 : props.size >= 20 ? 2.75 : props.size >= 14 ? 2.0 : 1.75)
  return (painted * 16) / props.size
})

const CONTENT: Record<string, string> = {
  IconBullets: `<circle cx="3.66" cy="4.31" r="0.87" fill="currentColor" stroke="none" /> <circle cx="3.66" cy="8.21" r="0.87" fill="currentColor" stroke="none" /> <circle cx="3.66" cy="12.11" r="0.87" fill="currentColor" stroke="none" /> <path d="M 6.42 4.31 h 6.32 M 6.42 8.21 h 6.32 M 6.42 12.11 h 6.32" />`,
  IconNumbered: `<text x="1" y="5.4" fontSize="5.4" fill="currentColor" stroke="none" fontFamily="Segoe UI, sans-serif" > 1 </text> <text x="1" y="10.4" fontSize="5.4" fill="currentColor" stroke="none" fontFamily="Segoe UI, sans-serif" > 2 </text> <text x="1" y="15.4" fontSize="5.4" fill="currentColor" stroke="none" fontFamily="Segoe UI, sans-serif" > 3 </text> <path d="M6.5 3.5h8M6.5 8.5h8M6.5 13.5h8" />`,
  IconMultilevel: `<rect x="2.61" y="3.62" width="1.39" height="1.39" fill="currentColor" stroke="none" /> <path d="M 5.69 4.31 h 6.93" /> <rect x="4.54" y="7.52" width="1.39" height="1.39" fill="currentColor" stroke="none" /> <path d="M 7.62 8.21 h 5.01" /> <rect x="6.46" y="11.42" width="1.39" height="1.39" fill="currentColor" stroke="none" /> <path d="M 9.54 12.11 h 3.08" />`,
  IconIndentDec: `<path d="M 3.02 3.44 h 9.96 M 8 6.17 h 4.98 M 8 8.41 h 4.98 M 8 10.66 h 4.98 M 3.02 12.98 h 9.96" /> <path d="M 5.68 6.17 3.19 8.41 l 2.49 2.24 z" fill="currentColor" stroke="none" />`,
  IconIndentInc: `<path d="M 3.02 3.44 h 9.96 M 8 6.17 h 4.98 M 8 8.41 h 4.98 M 8 10.66 h 4.98 M 3.02 12.98 h 9.96" /> <path d="M 3.19 6.17 l 2.49 2.24 -2.49 2.24 z" fill="currentColor" stroke="none" />`,
  IconAlignLeft: `<path d="M 3.02 3.44 h 9.96 M 3.02 6.62 h 6.64 M 3.02 9.8 h 9.96 M 3.02 12.98 h 6.64" />`,
  IconAlignCenter: `<path d="M 3.02 3.44 h 9.96 M 4.68 6.62 h 6.64 M 3.02 9.8 h 9.96 M 4.68 12.98 h 6.64" />`,
  IconAlignRight: `<path d="M 3.02 3.44 h 9.96 M 6.34 6.62 h 6.64 M 3.02 9.8 h 9.96 M 6.34 12.98 h 6.64" />`,
  IconAlignJustify: `<path d="M 3.02 3.44 h 9.96 M 3.02 6.62 h 9.96 M 3.02 9.8 h 9.96 M 3.02 12.98 h 9.96" />`,
  IconDirLtr: `<path d="M 3.02 3.85 h 9.96 M 3.02 6.34 h 6.64 M 3.02 11.32 h 7.1" /> <path d="M 9.8 9.4 l 2.9 1.92 -2.9 1.92 z" fill="currentColor" stroke="none" />`,
  IconDirRtl: `<path d="M 3.02 3.85 h 9.96 M 6.34 6.34 h 6.64 M 5.88 11.32 h 7.1" /> <path d="M 6.2 9.4 l -2.9 1.92 2.9 1.92 z" fill="currentColor" stroke="none" />`,
  IconLineSpacing: `<path d="M 8 3.44 h 4.92 M 8 6.62 h 4.92 M 8 9.8 h 4.92 M 8 12.98 h 4.92" /> <path d="M 4.31 3.75 v 8.9 M 2.92 5.39 l 1.39 -1.64 1.39 1.64 M 2.92 11.01 l 1.39 1.64 1.39 -1.64" />`,
  IconClearFormat: `<text x="3" y="11.5" font-size="9" font-family="Segoe UI, sans-serif" fill="currentColor" stroke="none">A</text> <rect x="8.5" y="9" width="5.5" height="4" rx="0.5" transform="rotate(45 11.25 11)" /> <path d="M 7.5 9 l 1.5 -1.5" />`,
  IconGrowFont: `<text x="3" y="11.5" font-size="9" font-family="Segoe UI, sans-serif" fill="currentColor" stroke="none">A</text> <path d="M11.5 4.5 v8 m-2.5 -2.5 l2.5 -2.5 l2.5 2.5" />`,
  IconShrinkFont: `<text x="3" y="11.5" font-size="9" font-family="Segoe UI, sans-serif" fill="currentColor" stroke="none">A</text> <path d="M11.5 12.5 v-8 m-2.5 2.5 l2.5 2.5 l2.5 -2.5" />`,
  IconChangeCase: `<text x="2" y="11" font-size="8" font-family="Segoe UI, sans-serif" fill="currentColor" stroke="none">A</text> <text x="9" y="11" font-size="8" font-family="Segoe UI, sans-serif" fill="currentColor" stroke="none">a</text> <path d="M 8.5 3 v 10 M 6 11 h 5" />`,
  IconFontColorA: `<LetterA dx={2.33} />`,
  IconSuperscript: `<path d="M2.7 6.6 8.3 13M8.3 6.6 2.7 13" /> <path d="M10.6 4.7a1.5 1.5 0 0 1 3 0c0 .9-.85 1.6-3 3.1h3.15" />`,
  IconSubscript: `<path d="M2.7 4.6 8.3 11M8.3 4.6 2.7 11" /> <path d="M10.6 9.9a1.5 1.5 0 0 1 3 0c0 .9-.85 1.6-3 3.1h3.15" />`,
  IconHighlight: `<path d="M3 10.5 9.5 4a1.4 1.4 0 0 1 2 0l0.5 0.5a1.4 1.4 0 0 1 0 2L5.5 13H3z" fill="none" /> <path d="M2.2 13h4" />`,
  IconPaste: `<path d="M 5.43 12.11 H 3.93 C 3.23 12.11 2.67 11.54 2.67 10.84 V 4.53 C 2.67 3.83 3.23 3.26 3.93 3.26 H 4.88 M 11.51 5.16 V 4.53 C 11.51 3.83 10.94 3.26 10.25 3.26 H 9.3" /> <rect x="5.19" y="2" width="3.79" height="1.89" rx="0.63" /> <path d="M 12.14 5.16 H 6.46 C 5.76 5.16 5.19 5.72 5.19 6.42 V 12.74 C 5.19 13.43 5.76 14 6.46 14 H 10.32 L 13.4 10.68 V 6.42 C 13.4 5.72 12.84 5.16 12.14 5.16 Z" /> <path d="M 7.09 7.37 H 11.51 M 7.09 9.58 H 9.61" /> <path d="M 10.25 14 V 11.16 C 10.25 10.81 10.53 10.53 10.88 10.53 H 13.4" />`,
  IconCut: `<path d="M 5.27 12.85 L 5.94 11.71 L 11.32 2.4 M 4.68 2.33 L 10.05 11.65 L 10.73 12.85" /> <circle cx="3.89" cy="12.08" r="1.58" /> <circle cx="12.11" cy="12.08" r="1.58" />`,
  IconCopy: `<rect x="4.67" y="4.67" width="9.33" height="9.33" rx="2" /> <path d="M 9.67 2.67 H 4.67 C 3.56 2.67 2.67 3.56 2.67 4.67 V 9.67" />`,
  IconFormatPainter: `<rect x="7.1" y="2.7" width="1.8" height="3.4" rx="0.9" /> <rect x="3" y="6.1" width="10" height="7.2" rx="1" /> <path d="M 3 8.9 H 13" /> <path d="M 6.2 10.9 V 12.1 M 9.8 10.9 V 12.1" />`,
  IconBold: `<path d="M7 4 H10.5 a2.8 2.8 0 0 1 0 5.6 H7 M7 9.6 H11 a2.9 2.9 0 0 1 0 5.8 H7 M5 4 V16" />`,
  IconItalic: `<line x1="8" y1="4" x2="13" y2="4" /> <line x1="11" y1="20" x2="6" y2="20" /> <line x1="9.5" y1="4" x2="6.5" y2="20" />`,
  IconUnderline: `<path d="M5.5 4 V11 a3.5 3.5 0 0 0 7 0 V4" /> <line x1="4" y1="16" x2="14" y2="16" />`,
  IconAiImage: `<rect x="3" y="4" width="10" height="8" rx="1.5" /> <circle cx="6.5" cy="7" r="1.1" /> <path d="M3 12 L6.5 9 L9 11 L11 9 L13 11.5 V12 Z" />`,
  IconAiAsk: `<circle cx="8" cy="8" r="5" /> <path d="M11.5 11.5 L15 15" />`,
  IconTable: `<rect x="3.02" y="3.44" width="9.96" height="9.13" rx="0.66" /> <path d="M 3.02 6.51 h 9.96 M 3.02 9.58 h 9.96 M 6.34 3.44 v 9.13 M 9.66 3.44 v 9.13" />`,
  IconPicture: `<rect x="3.02" y="3.85" width="9.96" height="8.3" rx="0.66" /> <circle cx="5.84" cy="6.51" r="0.91" /> <path d="M 3.44 11.32 6.76 8 l 2.49 2.49 1.66 -1.66 1.66 1.66" />`,
  IconRemoveBg: `<rect x="3.02" y="3.85" width="9.96" height="8.3" rx="0.66" strokeDasharray="2.2 1.6" /> <circle cx="8" cy="6.92" r="1.41" /> <path d="M 5.43 12.15 c 0.33 -1.91 1.41 -2.9 2.57 -2.9 s 2.24 1 2.57 2.91" />`,
  IconCrop: `<path d="M 5.33 3.17 v 7.5 h 7.5" /> <path d="M 3.17 5.33 h 7.5 v 7.5" />`,
  IconRotateRight: `<path d="M 12.4 6.2 a 4.6 4.6 0 1 0 0.6 3.3" /> <path d="M 12.7 3.2 v 3 h -3" />`,
  IconRotateLeft: `<path d="M 3.6 6.2 a 4.6 4.6 0 1 1 -0.6 3.3" /> <path d="M 3.3 3.2 v 3 h 3" />`,
  IconFlipH: `<path d="M 8 2.6 v 10.8" strokeDasharray="1.7 1.5" /> <path d="M 6 5.2 L 2.6 8 L 6 10.8 Z" /> <path d="M 10 5.2 L 13.4 8 L 10 10.8 Z" fill="currentColor" />`,
  IconFlipV: `<path d="M 2.6 8 h 10.8" strokeDasharray="1.7 1.5" /> <path d="M 5.2 6 L 8 2.6 L 10.8 6 Z" /> <path d="M 5.2 10 L 8 13.4 L 10.8 10 Z" fill="currentColor" />`,
  IconReplacePicture: `<rect x="2.87" y="6.03" width="7.11" height="6.32" rx="0.63" /> <circle cx="4.92" cy="8" r="0.71" /> <path d="M 3.26 11.79 l 2.13 -2.13 1.5 1.5 1.11 -1.11 1.42 1.42" /> <path d="M 9.19 3.73 h 3.63 m 0 0 -1.34 -1.26 m 1.34 1.26 -1.34 1.26" />`,
  IconChart: `<path d="M 3.02 3.02 v 9.96 h 9.96" /> <rect x="5.09" y="8" width="1.83" height="3.32" fill="currentColor" stroke="none" /> <rect x="8" y="5.51" width="1.83" height="5.81" fill="currentColor" stroke="none" /> <rect x="10.91" y="6.75" width="1.83" height="4.57" fill="currentColor" stroke="none" />`,
  IconShapes: `<circle cx="6.28" cy="6.28" r="3.1" /> <rect x="7.57" y="7.57" width="5.59" height="5.59" rx="0.69" fill="var(--surface, #fff)" />`,
  IconLink: `<path d="M 6.91 9.09 9.09 6.91" /> <path d="M 7.55 5.09 8.91 3.72 a 2.37 2.37 0 0 1 3.37 3.37 L 10.91 8.46" /> <path d="M 8.46 10.91 7.09 12.28 a 2.37 2.37 0 0 1 -3.37 -3.37 l 1.37 -1.36" />`,
  IconComment: `<path d="M 2.99 3.91 h 10.01 v 6.83 h -5.46 L 4.81 13.46 v -2.73 h -1.82 z" /> <path d="M 5.27 6.18 h 5.46 M 5.27 8.46 h 3.64" />`,
  IconComments: `<path d="M 5.75 3.45 h 7.28 v 5.46 h -1.68" /> <path d="M 2.99 5.75 h 8.19 v 5.46 h -4.1 L 4.81 13.5 v -2.29 h -1.82 z" /> <path d="M 5.27 8.2 h 3.9" />`,
  IconPageBreak: `<path d="M 4.92 3 h 6.16 v 3.47 M 4.92 3 v 3.47 M 4.92 13.01 h 6.16 v -3.46 M 4.92 13.01 v -3.46" /> <path d="M 3 8 h 1.54 M 5.69 8 h 1.54 M 8.39 8 h 1.54 M 11.08 8 h 1.93" strokeDasharray="none" />`,
  IconHeader: `<rect x="4.15" y="3" width="7.7" height="10.01" rx="0.62" /> <path d="M 5.31 4.92 h 5.39 M 5.31 6.31 h 5.39" strokeWidth="1" opacity="0.9" />`,
  IconFooter: `<rect x="4.15" y="3" width="7.7" height="10.01" rx="0.62" /> <path d="M 5.31 9.69 h 5.39 M 5.31 11.08 h 5.39" strokeWidth="1" opacity="0.9" />`,
  IconPageNumber: `<rect x="4.15" y="3" width="7.7" height="10.01" rx="0.62" /> <TextGlyph x={6.15} y={10.31} s={5.39}> # </TextGlyph>`,
  IconSymbol: `<TextGlyph x={3.2} y={13} s={13}> Ω </TextGlyph>`,
  IconEquation: `<TextGlyph x={4} y={12.5} s={12}> π </TextGlyph>`,
  IconTableDelete: `<rect x="3.26" y="3.62" width="8.03" height="7.3" rx="0.58" /> <path d="M 3.26 6.03 h 8.03 M 3.26 8.51 h 8.03 M 5.96 3.62 v 7.3 M 8.58 3.62 v 7.3" strokeWidth="1" /> <path d="M 9.17 9.17 h 4.09 v 4.09 H 9.17 z" fill="var(--surface, #fff)" stroke="none" /> <path d="m 9.97 9.97 2.63 2.63 M 12.6 9.97 l -2.63 2.63" strokeWidth="1" />`,
  IconAutoFit: `<rect x="3.1" y="4" width="9.8" height="8" rx="0.65" /> <path d="M 6.35 4 v 8 M 9.65 4 v 8 M 3.1 8 h 9.8" strokeWidth="1" /> <path d="M 1.35 8 h 2.6 M 1.35 8 l 1 -1 M 1.35 8 l 1 1" /> <path d="M 14.65 8 h -2.6 M 14.65 8 l -1 -1 M 14.65 8 l -1 1" />`,
  IconRepeatHeader: `<rect x="3" y="3.45" width="8.8" height="9.1" rx="0.65" /> <path d="M 3 6.2 h 8.8 M 3 9.35 h 8.8 M 7.4 3.45 v 9.1" strokeWidth="1" /> <path d="M 3.6 4.8 h 7.6" strokeWidth="1.5" /> <path d="M 11.35 10.15 a 2.15 2.15 0 1 1 -0.5 2.25" /> <path d="m 10.2 10.15 1.3 -0.05 -0.35 1.22" />`,
  IconTableProperties: `<rect x="2.85" y="3.2" width="7.7" height="9.6" rx="0.65" /> <path d="M 2.85 6.4 h 7.7 M 2.85 9.6 h 7.7 M 6.7 3.2 v 9.6" strokeWidth="1" /> <path d="M 11.7 5.15 h 2.15 M 11.7 8 h 2.15 M 11.7 10.85 h 2.15" /> <circle cx="12.35" cy="5.15" r="0.55" fill="currentColor" stroke="none" /> <circle cx="13.15" cy="8" r="0.55" fill="currentColor" stroke="none" /> <circle cx="12.65" cy="10.85" r="0.55" fill="currentColor" stroke="none" />`,
  IconRowInsertAbove: `<path d="M 8 6.18 V 2.98 M 6.63 4.35 8 2.98 l 1.37 1.37" /> <rect x="3.44" y="7.62" width="9.12" height="5.32" rx="0.61" /> <path d="M 3.44 10.28 h 9.12 M 8 7.62 v 5.32" strokeWidth="1" />`,
  IconRowInsertBelow: `<rect x="3.44" y="3.06" width="9.12" height="5.32" rx="0.61" /> <path d="M 3.44 5.72 h 9.12 M 8 3.06 v 5.32" strokeWidth="1" /> <path d="M 8 9.82 v 3.19 M 6.63 11.65 8 13.02 l 1.37 -1.37" />`,
  IconColInsertLeft: `<path d="M 6.18 8 H 2.98 M 4.35 6.63 2.98 8 l 1.37 1.37" /> <rect x="7.62" y="3.44" width="5.32" height="9.12" rx="0.61" /> <path d="M 10.28 3.44 v 9.12 M 7.62 8 h 5.32" strokeWidth="1" />`,
  IconColInsertRight: `<rect x="3.06" y="3.44" width="5.32" height="9.12" rx="0.61" /> <path d="M 5.72 3.44 v 9.12 M 3.06 8 h 5.32" strokeWidth="1" /> <path d="M 9.82 8 h 3.19 M 11.65 6.63 13.02 8 l -1.37 1.37" />`,
  IconMergeCells: `<rect x="3" y="4.15" width="10.01" height="7.7" rx="0.62" /> <path d="M 8 4.15 v 1.54 M 8 10.31 v 1.54" strokeWidth="1" /> <path d="M 4.46 8 h 2.31 M 5.77 7 6.77 8 5.77 9" /> <path d="M 11.54 8 h -2.31 M 10.23 7 9.23 8 l 1 1" />`,
  IconSplitCells: `<rect x="3" y="4.15" width="10.01" height="7.7" rx="0.62" /> <path d="M 8 4.15 v 7.7" strokeWidth="1" /> <path d="M 6.92 8 h -2.31 M 5.61 7 4.61 8 l 1 1" /> <path d="M 9.08 8 h 2.31 M 10.39 7 11.39 8 l -1 1" />`,
  IconRowDelete: `<rect x="3.02" y="3.44" width="9.96" height="9.13" rx="0.66" /> <path d="M 3.02 6.51 h 9.96 M 3.02 9.49 h 9.96" strokeWidth="1" /> <path d="m 6.01 6.92 3.98 2.16 M 9.99 6.92 6.01 9.08" />`,
  IconColDelete: `<rect x="3.44" y="3.02" width="9.13" height="9.96" rx="0.66" /> <path d="M 6.51 3.02 v 9.96 M 9.49 3.02 v 9.96" strokeWidth="1" /> <path d="m 6.92 6.01 2.16 3.98 M 9.08 6.01 6.92 9.99" />`,
  IconCellAlignTop: `<rect x="3.02" y="3.44" width="9.96" height="9.13" rx="0.66" /> <path d="M 5.1 5.68 h 5.81 M 5.1 7.5 h 3.74" />`,
  IconCellAlignMiddle: `<rect x="3.02" y="3.44" width="9.96" height="9.13" rx="0.66" /> <path d="M 5.1 7.09 h 5.81 M 5.1 8.91 h 3.74" />`,
  IconCellAlignBottom: `<rect x="3.02" y="3.44" width="9.96" height="9.13" rx="0.66" /> <path d="M 5.1 8.5 h 5.81 M 5.1 10.32 h 3.74" />`,
  IconBorderAll: `<rect x="3.02" y="3.02" width="9.96" height="9.96" rx="0.42" /> <path d="M 3.02 8 h 9.96 M 8 3.02 v 9.96" />`,
  IconBorderOuter: `<rect x="3.02" y="3.02" width="9.96" height="9.96" rx="0.42" /> <path d="M 3.02 8 h 9.96 M 8 3.02 v 9.96" strokeWidth="1" strokeDasharray="1.5 1.7" opacity="0.55" />`,
  IconBorderInner: `<rect x="3.02" y="3.02" width="9.96" height="9.96" rx="0.42" strokeWidth="1" strokeDasharray="1.5 1.7" opacity="0.55" /> <path d="M 3.02 8 h 9.96 M 8 3.02 v 9.96" />`,
  IconBorderNone: `<rect x="3.02" y="3.02" width="9.96" height="9.96" rx="0.42" strokeWidth="1" strokeDasharray="1.5 1.7" opacity="0.55" /> <path d="M 3.02 8 h 9.96 M 8 3.02 v 9.96" strokeWidth="1" strokeDasharray="1.5 1.7" opacity="0.55" />`,
  IconTheme: `<TextGlyph x={1.5} y={11.5} s={11}> A </TextGlyph> <TextGlyph x={8.5} y={11.5} s={8}> a </TextGlyph> <path d="M2.5 13.8h11" strokeWidth="1" />`,
  IconThemeFonts: `<TextGlyph x={2} y={12} s={11}> F </TextGlyph> <path d="M9.5 12 12 4.5 14.5 12M10.3 9.6h3.4" strokeWidth="1" />`,
  IconThemeColors: `<circle cx="4.89" cy="5.33" r="1.87" /> <circle cx="11.11" cy="5.33" r="1.87" /> <circle cx="4.89" cy="11.11" r="1.87" /> <circle cx="11.11" cy="11.11" r="1.87" fill="currentColor" />`,
  IconPageColor: `<path d="M 8.56 3.36 4.4 7.52 a 1.04 1.04 0 0 0 0 1.44 l 2.64 2.64 a 1.04 1.04 0 0 0 1.44 0 l 4.16 -4.16 z" /> <path d="M 8.56 3.36 7.2 4.8" /> <path d="M 12.48 10.08 s 1.12 1.36 1.12 2.16 a 1.12 1.12 0 0 1 -2.24 0 c 0 -0.8 1.12 -2.16 1.12 -2.16 z" fill="currentColor" stroke="none" />`,
  IconWatermark: `{PAGE} <path d="M 5.84 10.7 10.16 5.69" strokeWidth="1" opacity="0.45" />`,
  IconPageBorders: `<rect x="3.02" y="3.02" width="9.96" height="9.96" rx="0.66" /> <rect x="5.01" y="5.01" width="5.98" height="5.98" />`,
  IconMargins: `<rect x="4.15" y="3" width="7.7" height="10.01" rx="0.62" /> <rect x="5.84" y="4.69" width="4.31" height="6.62" strokeDasharray="1.6 1.4" />`,
  IconOrientation: `<rect x="3.2" y="4.8" width="6" height="8" rx="0.64" /> <rect x="6" y="7.6" width="7.2" height="5.2" rx="0.64" fill="var(--surface, #fff)" /> <path d="M 10.4 3.36 a 4 4 0 0 1 2.4 2.08 M 12.8 3.6 v 2 h -2" strokeWidth="1" />`,
  IconPageSize: `<rect x="4.15" y="3" width="7.7" height="10.01" rx="0.62" /> <path d="M 6.08 8 h 3.85 M 8 6.08 v 3.85 M 7 7 6.08 6.08 m 3.85 0 -0.92 0.92 m 0 2.01 0.92 0.92 m -3.85 0 0.92 -0.92" strokeWidth="1" />`,
  IconColumns: `<path d="M 2.99 3.45 h 4.1 M 2.99 5.73 h 4.1 M 2.99 8 h 4.1 M 2.99 10.27 h 4.1 M 2.99 12.55 h 4.1" /> <path d="M 8.91 3.45 h 4.1 M 8.91 5.73 h 4.1 M 8.91 8 h 4.1 M 8.91 10.27 h 4.1 M 8.91 12.55 h 4.1" />`,
  IconToc: `{PAGE} <path d="M 6.08 5.69 h 3.85 M 7 7.46 h 2.93 M 7 9.23 h 2.93 M 6.08 11 h 3.85" />`,
  IconRefresh: `<path d="M 12.68 6.65 a 4.86 4.86 0 0 0 -9 -1.08 M 3.32 9.35 a 4.86 4.86 0 0 0 9 1.08" /> <path d="M 12.95 3.05 v 2.7 h -2.7 M 3.05 12.95 v -2.7 h 2.7" />`,
  IconFootnote: `<TextGlyph x={1.5} y={12} s={9}> AB </TextGlyph> <TextGlyph x={11.5} y={8} s={7} bold> 1 </TextGlyph>`,
  IconEndnote: `<TextGlyph x={1.5} y={12} s={9}> AB </TextGlyph> <TextGlyph x={11.3} y={8} s={7} bold> n </TextGlyph>`,
  IconCitation: `<path d="M6.9 4.9c-2 .7-3.3 2.3-3.3 4.3 0 1.3.9 2.3 2.1 2.3s2.1-1 2.1-2.2c0-1.2-.8-2.1-1.9-2.1.3-.8 1-1.4 1.9-1.8z" fill="currentColor" stroke="none" /> <path d="M12.9 4.9c-2 .7-3.3 2.3-3.3 4.3 0 1.3.9 2.3 2.1 2.3s2.1-1 2.1-2.2c0-1.2-.8-2.1-1.9-2.1.3-.8 1-1.4 1.9-1.8z" fill="currentColor" stroke="none" />`,
  IconBook: `<path d="M 8 4.09 C 6.96 3.3 5.22 2.95 3.22 3.13 v 9.05 c 2 -0.17 3.74 0.17 4.79 0.96 1.04 -0.78 2.78 -1.13 4.79 -0.96 V 3.13 c -2 -0.17 -3.74 0.17 -4.78 0.96 z" /> <path d="M 8 4.09 v 9.05" />`,
  IconCaption: `<path d="M 3.02 5.1 h 7.06 L 12.98 8 l -2.9 2.91 H 3.02 z" /> <circle cx="5.51" cy="8" r="0.75" fill="currentColor" stroke="none" />`,
  IconIndex: `<TextGlyph x={1.8} y={6.5} s={6.5}> A </TextGlyph> <TextGlyph x={1.8} y={13.5} s={6.5}> B </TextGlyph> <path d="M8 4.5h6M8 8h6M8 11.5h6" />`,
  IconWordCount: `<TextGlyph x={1.6} y={8} s={8}> 123 </TextGlyph> <path d="M2 11h12M2 13.5h8" />`,
  IconSpellcheck: `<TextGlyph x={1.4} y={8.5} s={7.5}> abc </TextGlyph> <path d="M6 11.5 8.5 13.5 13 7.5" strokeWidth="1" />`,
  IconSparkle: `<path d="M 8 3.03 C 8 5.78 10.22 8 12.97 8 C 10.22 8 8 10.22 8 12.97 C 8 10.22 5.78 8 3.03 8 C 5.78 8 8 5.78 8 3.03 Z" fill="currentColor" stroke="none" />`,
  IconWand: `<path d="M 3.6 12.4 9.6 6.4" strokeWidth="1" /> <path d="M 11.04 2.8 l 0.56 1.52 1.52 0.56 -1.52 0.56 -0.56 1.52 -0.56 -1.52 -1.52 -0.56 1.52 -0.56 z" fill="currentColor" stroke="none" /> <path d="M 12.4 8.4 l 0.32 0.88 0.88 0.32 -0.88 0.32 -0.32 0.88 -0.32 -0.88 -0.88 -0.32 0.88 -0.32 z" fill="currentColor" stroke="none" />`,
  IconTranslate: `<TextGlyph x={1.2} y={9} s={8.5}> 文 </TextGlyph> <path d="M8.8 13.5 11.5 6.5 14.2 13.5M9.7 11.3h3.6" strokeWidth="1" />`,
  IconTrackChanges: `{PAGE} <path d="M 6.08 6.08 h 3.85 M 6.08 8 h 2.31" /> <path d="M 8.38 12 12.31 8.08 l 0.92 0.92 -3.93 3.93 -1.39 0.46 z" fill="var(--surface, #fff)" />`,
  IconAccept: `<path d="M 3.91 9.36 7.09 12.55 l 6.83 -7.28" />`,
  IconReject: `<path d="m4.5 4.5 9 9M13.5 4.5l-9 9" />`,
  IconCompare: `<rect x="3" y="4.15" width="4.24" height="7.7" rx="0.62" /> <rect x="8.77" y="4.15" width="4.24" height="7.7" rx="0.62" /> <path d="M 6.46 8 h 3.08 M 8.46 6.92 9.54 8 l -1.08 1.08" strokeWidth="1" />`,
  IconLock: `<rect x="4.27" y="7.17" width="7.47" height="6.23" rx="0.83" /> <path d="M 5.93 7.17 V 5.51 a 2.07 2.07 0 0 1 4.15 0 v 1.66" /> <circle cx="8" cy="10.07" r="0.83" fill="currentColor" stroke="none" />`,
  IconKey: `<circle cx="5.4" cy="10.6" r="2.5" /> <path d="M 7.3 8.7 L 12.9 3.1 M 10.4 5.6 l 1.7 1.7 M 12.1 3.9 l 1.4 1.4" />`,
  IconEye: `<path d="M 1.7 8 C 3.1 5.15 5.35 3.6 8 3.6 s 4.9 1.55 6.3 4.4 C 12.9 10.85 10.65 12.4 8 12.4 S 3.1 10.85 1.7 8 Z" /> <circle cx="8" cy="8" r="1.95" />`,
  IconEyeOff: `<path d="M 3.5 4.65 C 2.78 5.5 2.18 6.62 1.7 8 c 1.4 2.85 3.65 4.4 6.3 4.4 c 1.02 0 1.97 -0.23 2.84 -0.68 M 6.4 3.82 C 6.91 3.67 7.44 3.6 8 3.6 c 2.65 0 4.9 1.55 6.3 4.4 c -0.4 0.82 -0.87 1.54 -1.4 2.15" /> <path d="M 6.62 6.62 a 1.95 1.95 0 0 0 2.76 2.76" /> <path d="M 2.7 2.7 l 10.6 10.6" />`,
  IconAlert: `<circle cx="8" cy="8" r="6.2" /> <path d="M 8 4.9 v 3.5" /> <circle cx="8" cy="10.9" r="0.75" fill="currentColor" stroke="none" />`,
  IconZoomOut: `<Magnifier> <path d="M 5.28 7.15 h 3.74" /> </Magnifier>`,
  IconZoomIn: `<Magnifier> <path d="M 5.28 7.15 h 3.74 M 7.15 5.28 v 3.74" /> </Magnifier>`,
  IconZoom100: `<Magnifier> <TextGlyph x={4.43} y={8.85} s={4.25} bold> 1:1 </TextGlyph> </Magnifier>`,
  IconPageWidth: `<rect x="3.02" y="3.02" width="9.96" height="9.96" rx="0.66" /> <path d="M 4.68 8 h 6.64 M 6.17 6.51 4.68 8 l 1.49 1.49 M 9.83 6.51 11.32 8 l -1.49 1.49" strokeWidth="1" />`,
  IconWholePage: `<rect x="4.15" y="3" width="7.7" height="10.01" rx="0.62" /> <path d="M 6.08 8 h 3.85 M 8 6.08 v 3.85" strokeWidth="1" />`,
  IconAiPanel: `<rect x="3" y="3.76" width="10.01" height="8.47" rx="0.62" /> <path d="M 9.39 3.76 v 8.47" /> <path d="M 10.31 6.61 l 0.39 1 1 0.39 -1 0.39 -0.38 1 -0.38 -1 -1 -0.38 1 -0.38 z" fill="currentColor" stroke="none" />`,
  IconMoon: `<path d="M 12.52 9.57 A 5.05 5.05 0 0 1 6.43 3.48 a 5.05 5.05 0 1 0 6.09 6.09 z" />`,
  IconOutlineView: `<circle cx="3.65" cy="4.09" r="0.87" fill="currentColor" stroke="none" /> <path d="M 5.83 4.09 h 6.96" /> <circle cx="5.83" cy="8" r="0.87" fill="currentColor" stroke="none" /> <path d="M 8 8 h 4.79" /> <circle cx="5.83" cy="11.92" r="0.87" fill="currentColor" stroke="none" /> <path d="M 8 11.92 h 4.79" />`,
  IconRuler: `<rect x="3" y="6.08" width="10.01" height="3.85" rx="0.62" /> <path d="M 5.31 6.08 v 1.54 M 7.23 6.08 v 2.31 M 9.16 6.08 v 1.54 M 11.08 6.08 v 2.31" strokeWidth="1" />`,
  IconNavPane: `<rect x="3" y="3.76" width="10.01" height="8.47" rx="0.62" /> <path d="M 6.46 3.76 v 8.47" /> <path d="M 4 5.69 h 1.54 M 4 7.62 h 1.54 M 4 9.54 h 1.54" strokeWidth="1" />`,
  IconSplit: `<rect x="3.02" y="3.02" width="9.96" height="9.96" rx="0.66" /> <path d="M 3.02 8 h 9.96" strokeWidth="1" />`,
  IconPrintLayout: `<rect x="4.54" y="3" width="6.93" height="10.01" rx="0.62" /> <path d="M 6.08 5.31 h 3.85 M 6.08 7.23 h 3.85 M 6.08 9.16 h 3.85 M 6.08 11.08 h 2.31" strokeWidth="1" />`,
  IconWebLayout: `<rect x="3" y="3.76" width="10.01" height="8.47" rx="0.62" /> <path d="M 3 5.69 h 10.01" /> <path d="M 4.54 7.62 h 6.93 M 4.54 9.16 h 6.93 M 4.54 10.7 h 4.62" strokeWidth="1" />`,
  IconGridlines: `<rect x="3.02" y="3.02" width="9.96" height="9.96" rx="0.66" /> <path d="M 3.02 6.34 h 9.96 M 3.02 9.66 h 9.96 M 6.34 3.02 v 9.96 M 9.66 3.02 v 9.96" strokeWidth="1" />`,
  IconNewWindow: `<rect x="3" y="5.31" width="7.7" height="7.7" rx="0.62" /> <path d="M 5.31 5.31 v -1.54 a 0.77 0.77 0 0 1 0.77 -0.77 h 6.16 a 0.77 0.77 0 0 1 0.77 0.77 v 6.16 a 0.77 0.77 0 0 1 -0.77 0.77 h -1.54" /> <path d="M 6.85 9.16 h 3.08 M 8.39 7.62 v 3.08" />`,
  IconArrangeAll: `<rect x="3" y="3.38" width="10.01" height="4" rx="0.62" /> <rect x="3" y="8.62" width="10.01" height="4" rx="0.62" />`,
  IconSwitchWindows: `<rect x="3" y="6.08" width="6.93" height="6.16" rx="0.62" /> <path d="M 5.69 6.08 v -1.54 a 0.77 0.77 0 0 1 0.77 -0.77 h 5.78 a 0.77 0.77 0 0 1 0.77 0.77 v 5.39 a 0.77 0.77 0 0 1 -0.77 0.77 h -2.31" />`,
  IconPosition: `<rect x="3" y="3" width="10.01" height="10.01" rx="0.77" /> <rect x="5.69" y="5.69" width="4.62" height="4.62" />`,
  IconWrapText: `<rect x="3" y="5.69" width="4.62" height="4.62" /> <path d="M 9.16 3.38 h 3.85 M 9.16 5.69 h 3.85 M 9.16 8 h 3.85 M 9.16 10.31 h 3.85 M 3 12.62 h 10.01 M 3 3.38 h 4.62" />`,
  IconDoc: `{PAGE} <path d="M 9.54 3 V 4.92 h 1.93" /> <path d="M 6.08 6.84 h 3.85 M 6.08 8.77 h 3.85 M 6.08 10.7 h 2.7" />`,
  IconSend: `<path d="M 3.01 8 12.99 3.36 10.58 12.64 7.66 9.38 z" strokeLinejoin="round" /> <path d="M 7.66 9.38 12.99 3.36" />`,
  IconStop: `<rect x="3" y="3" width="10" height="10" rx="1.88" fill="currentColor" stroke="none" />`,
  IconGear: `<circle cx="8" cy="8" r="1.78" /> <path d="M 8 2.98 v 1.62 M 8 11.4 v 1.62 M 13.02 8 h -1.62 M 4.6 8 h -1.62 M 11.56 4.44 l -1.13 1.13 M 5.57 10.43 l -1.13 1.13 M 11.56 11.56 10.43 10.43 M 5.57 5.57 4.44 4.44" />`,
  IconClock: `<circle cx="8" cy="8" r="4.98" /> <path d="M 8 5.34 V 8 l 1.91 1.33" />`,
  IconPaperclip: `<path d="M 12.5 7.28 8.18 11.6 a 3.06 3.06 0 0 1 -4.32 -4.32 l 4.5 -4.5 a 2.07 2.07 0 0 1 2.88 2.88 l -4.5 4.5 a 0.99 0.99 0 0 1 -1.44 -1.44 l 4.14 -4.14" strokeLinejoin="round" />`,
  IconNewChat: `<path d="M 12.68 7.32 v -2.55 A 1.44 1.44 0 0 0 11.23 3.33 H 4.77 a 1.44 1.44 0 0 0 -1.44 1.44 v 5.19 a 1.44 1.44 0 0 0 1.44 1.44 h 0.94 v 1.7 l 2.21 -1.7 h 1.11" strokeLinejoin="round" /> <path d="M 11.57 9.19 v 3.4 M 9.87 10.89 h 3.4" />`,
  IconCursor: `<path d="M 3.98 3.01 12.02 8.53 l -3.41 0.81 L 6.99 12.95 3.98 3.01 Z" />`,
  IconPen: `<path d="m3 13 .8-3L10.6 3.2a1.4 1.4 0 0 1 2 0l.2.2a1.4 1.4 0 0 1 0 2L6 12.2 3 13Z" /> <path d="M9.6 4.2 11.8 6.4" />`,
  IconHighlighterPen: `<path d="M 5.68 9.4 10.6 4.47 a 1.21 1.21 0 0 1 1.77 0 l -0.84 -0.84 0.84 0.84 a 1.21 1.21 0 0 1 0 1.77 L 7.44 11.16 l -2.42 0.65 0.65 -2.42 Z" /> <path d="M 3.35 13.58 h 9.3" strokeWidth="1" opacity="0.5" />`,
  IconEraser: `<path d="m8.3 3.6 4.1 4.1a1.2 1.2 0 0 1 0 1.7L9.6 12.2H6.8L3.6 9a1.2 1.2 0 0 1 0-1.7l3-3a1.2 1.2 0 0 1 1.7 0Z" /> <path d="M5.5 5.8 10.2 10.5" /> <path d="M6.8 12.2h6.4" />`,
  IconTextBox: `<rect x="2.99" y="3.77" width="10.01" height="8.47" rx="0.77" /> <path d="M 5.69 6.07 h 4.62 M 8 6.07 v 4.23" />`,
  IconWordArt: `{/* stylized A with gradient effect hint */} <path d="M8 3 3.5 13h2.3l1-2.5h2.4l1 2.5h2.3L8 3Z" /> <path d="M5.6 9.2h4.8" />`,
  IconTrash: `<path d="M3.11 4.89h9.79M6.4 4.89V3.73a.62.62 0 0 1 .62-.62h1.96a.62.62 0 0 1 .62.62v1.16" /> <path d="M4.44 4.89l.62 7.39a.89.89 0 0 0 .89.8h4.09a.89.89 0 0 0 .89-.8l.62-7.39" /> <path d="M6.75 7.11v3.56M9.25 7.11v3.56" />`,
  IconPalette: `<path d="M8 12.98a4.98 4.98 0 1 1 4.98-4.98c0 2.44-1.74 2.49-2.74 2.49-.8 0-1.25.5-1.25 1.25 0 .7-.45 1.25-1 1.25Z" /> <circle cx="8.83" cy="4.93" r="0.71" fill="currentColor" stroke="none" /> <circle cx="11.07" cy="6.71" r="0.71" fill="currentColor" stroke="none" /> <circle cx="6.09" cy="5.51" r="0.71" fill="currentColor" stroke="none" /> <circle cx="4.93" cy="8.25" r="0.71" fill="currentColor" stroke="none" />`,
  IconSort: `<TextGlyph x={1.5} y={7.2} s={7}> A </TextGlyph> <TextGlyph x={1.5} y={14.5} s={7}> Z </TextGlyph> <path d="M11.5 2.5V13M11.5 13 9.3 10.8M11.5 13l2.2-2.2" />`,
  IconPilcrow: `<path d="M 12.1 2.99 H 7.45 a 2.73 2.73 0 0 0 0 5.46 h 1.91 M 9.36 2.99 v 10.01 M 12.1 2.99 v 10.01" />`,
  IconShading: `<rect x="2.99" y="3.44" width="10.01" height="9.54" rx="0.46" /> <path d="M 2.99 7.7 7.25 3.44 M 2.99 11.9 11.45 3.44 M 5.5 12.98 13 5.48 M 9.2 12.98 13 9.18" opacity="0.55" />`,
  IconCheck: `<path d="M2.8 8.6 6.2 12l7-7.5" />`,
  IconCheckbox: `<rect x="2.99" y="2.99" width="10.01" height="10.01" rx="1.37" />`,
  IconCheckboxChecked: `<rect x="2.99" y="2.99" width="10.01" height="10.01" rx="1.37" /> <path d="M 5.45 8.27 l 1.91 2 3.37 -4.19" />`,
  IconClose: `<path d="M3.5 3.5l9 9M12.5 3.5l-9 9" />`,
  IconRectangle: `<rect x="3.02" y="4.15" width="9.96" height="7.7" rx="0.77" />`,
  IconUpload: `<path d="M 8 11.4 V 3.4" /> <path d="M 4.8 6.6 L 8 3.4 L 11.2 6.6" /> <path d="M 3 12.6 h 10" />`,
  IconArrowUp: `<path d="M 8 12.4 V 3.6" /> <path d="M 4.6 7 L 8 3.6 L 11.4 7" />`,
  IconArrowDown: `<path d="M 8 3.6 V 12.4" /> <path d="M 4.6 9 L 8 12.4 L 11.4 9" />`,
  IconGroup: `<rect x="3.02" y="3.02" width="6.32" height="6.32" rx="0.6" /> <rect x="6.66" y="6.66" width="6.32" height="6.32" rx="0.6" fill="var(--surface, #fff)" />`,
  IconLayers: `<path d="M 8 2.5 L 13.5 5.5 L 8 8.5 L 2.5 5.5 Z" /> <path d="M 2.5 8 L 8 11 L 13.5 8" /> <path d="M 2.5 10.5 L 8 13.5 L 13.5 10.5" />`,
  IconPlay: `<path d="M 4 3 L 13 8 L 4 13 Z" fill="currentColor" stroke="currentColor" />`,
  IconDownload: `<path d="M 8 3.4 V 11.4" /> <path d="M 4.8 8.2 L 8 11.4 L 11.2 8.2" /> <path d="M 3 13 h 10" />`,

  IconRoundRect: `<rect x="3.02" y="4.15" width="9.96" height="7.7" rx="2.3" />`,
  IconEllipse: `<ellipse cx="8" cy="8" rx="4.98" cy="3.85" />`,
  IconLine: `<path d="M 3.4 12.6 12.6 3.4" />`,
  IconArrow: `<path d="M 3.4 8 H 12.6 M 9.2 4.6 12.6 8 9.2 11.4" />`,
  IconTriangle: `<path d="M 8 3.4 12.6 12.4 H 3.4 Z" />`,
  IconStar: `<path d="M 8 2.6 L 9.6 6.4 L 13.6 6.6 L 10.5 9.2 L 11.5 13.0 L 8 11.0 L 4.5 13.0 L 5.5 9.2 L 2.4 6.6 L 6.4 6.4 Z" />`,
  IconHexagon: `<path d="M 8 2.6 12.8 5.4 V 10.6 L 8 13.4 3.2 10.6 V 5.4 Z" />`,
  IconCallout: `<path d="M 3 3.4 H 13 V 10.8 H 8.5 L 6.4 13.2 V 10.8 H 3 Z" />`,
    IconNewSlide: `<rect x="3.02" y="2.67" width="9.96" height="10.66" rx="1.2" /> <path d="M 5.43 6.49 H 10.57 M 5.43 9.07 H 10.57 M 5.43 11.65 H 8.71" /> <path d="M 10.92 2.67 H 12.85 V 4.6" /> <path d="M 3.16 2.67 H 5.09 V 4.6" /> <path d="M 10.92 13.33 H 12.85 V 11.4" /> <path d="M 3.16 13.33 H 5.09 V 11.4" />`,
  IconPlayFromStart: `<circle cx="8" cy="8" r="6.4" /> <path d="M 6.4 5.5 L 10.4 8 L 6.4 10.5 Z" fill="currentColor" stroke="none" />`,
  IconPlayCurrent: `<circle cx="8" cy="8" r="6.4" /> <path d="M 4.5 5.5 L 8.5 8 L 4.5 10.5 Z" fill="currentColor" stroke="none" /> <path d="M 8 5 L 12 8 L 8 11" fill="currentColor" stroke="none" />`,
  IconAddSection: `<path d="M 2.5 4.5 H 13.5" /> <path d="M 2.5 8 H 13.5" /> <path d="M 2.5 11.5 H 13.5" /> <path d="M 8 9.5 V 13.5 M 6 11.5 H 10" strokeWidth="1.4" />`,
  IconSlideLayout: `<rect x="2.5" y="2.5" width="11" height="11" rx="0.77" /> <rect x="4.2" y="4" width="7.6" height="1.2" /> <rect x="4.2" y="6.4" width="7.6" height="6" /> <rect x="4.2" y="6.4" width="7.6" height="6" fill="var(--surface, #fff)" opacity="0.9" /> <rect x="4.2" y="6.4" width="7.6" height="6" stroke="currentColor" strokeWidth="0.5" fill="none" opacity="0.4" />`,
  IconObjAlignLeft: `<path d="M 2 3 V 13" /> <rect x="3" y="4" width="9" height="2" /> <rect x="3" y="7" width="6" height="2" /> <rect x="3" y="10" width="8" height="2" />`,
  IconObjAlignCenter: `<path d="M 2 3 V 13 M 14 3 V 13" /> <rect x="3.5" y="4" width="8" height="2" /> <rect x="4.5" y="7" width="6" height="2" /> <rect x="3" y="10" width="9" height="2" />`,
  IconObjAlignRight: `<path d="M 14 3 V 13" /> <rect x="4" y="4" width="9" height="2" /> <rect x="7" y="7" width="6" height="2" /> <rect x="5" y="10" width="8" height="2" />`,
  IconObjAlignTop: `<path d="M 3 2 H 13" /> <rect x="4" y="3" width="2" height="9" /> <rect x="7" y="3" width="2" height="6" /> <rect x="10" y="3" width="2" height="8" />`,
  IconObjAlignMiddle: `<path d="M 3 2 H 13 M 3 14 H 13" /> <rect x="4" y="3.5" width="2" height="8" /> <rect x="7" y="4.5" width="2" height="6" /> <rect x="10" y="3" width="2" height="9" />`,
  IconObjAlignBottom: `<path d="M 3 14 H 13" /> <rect x="4" y="3" width="2" height="9" /> <rect x="7" y="5" width="2" height="6" /> <rect x="10" y="3" width="2" height="8" />`,
  IconDistributeH: `<path d="M 2 3 V 13" /> <path d="M 14 3 V 13" /> <rect x="3" y="6" width="3" height="4" /> <rect x="6.5" y="6" width="3" height="4" /> <rect x="10" y="6" width="3" height="4" />`,
  IconDistributeV: `<path d="M 3 2 H 13" /> <path d="M 3 14 H 13" /> <rect x="6" y="3" width="4" height="3" /> <rect x="6" y="6.5" width="4" height="3" /> <rect x="6" y="10" width="4" height="3" />`,
  IconReset: `<path d="M 5 4 V 8 H 9" /> <path d="M 5 4 a 5 5 0 1 1 -1 7" /> <path d="M 9 8 V 12 H 5" />`,
  IconObjectFlipH: `<path d="M 3 3 H 8 V 13 H 3 Z" fill="currentColor" stroke="none" opacity="0.4" /> <path d="M 3 3 H 8 V 13 H 3 Z" /> <path d="M 8 8 H 13 L 11.5 5 M 13 8 L 11.5 11" />`,
  IconObjectFlipV: `<path d="M 3 3 H 13 V 8 H 3 Z" fill="currentColor" stroke="none" opacity="0.4" /> <path d="M 3 3 H 13 V 8 H 3 Z" /> <path d="M 8 8 V 13 L 5 11.5 M 8 13 L 11 11.5" />`,
}

const content = computed(() => CONTENT[props.name] ?? "")
</script>
