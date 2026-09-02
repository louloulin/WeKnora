/**
 * mindmapTheme — v0.7.111.x MindMap Phase 2 polish.
 *
 * Pure data + helpers for the MindMap theme palette. Extracted from
 * MindMapEditor.vue so the palette can be unit-tested without Vue/DOM.
 *
 * Each theme is { bg, fg, line, accent }:
 *   - bg:    canvas background
 *   - fg:    default text fill
 *   - line:  connector line color between nodes
 *   - accent:selected / hover highlight color
 */
export interface MindMapThemeColors {
  bg: string
  fg: string
  line: string
  accent: string
}

/** All built-in theme palettes keyed by theme name. */
export const MINDMAP_THEME_COLORS: Record<string, MindMapThemeColors> = {
  feishu: { bg: '#1f2328', fg: '#e6edf3', line: '#58a6ff', accent: '#58a6ff' },
  notion: { bg: '#ffffff', fg: '#37352f', line: '#d9d9d7', accent: '#2383e2' },
  tana:   { bg: '#f7f6f3', fg: '#2f2f2f', line: '#cc8c2c', accent: '#cc8c2c' },
  coda:   { bg: '#fff5ec', fg: '#2d2d2d', line: '#ffcda1', accent: '#ff6c2c' },
  dark:   { bg: '#0d1117', fg: '#e6edf3', line: '#8b949e', accent: '#a371f7' },
}

/** Built-in theme name list (matches MindMap editor dropdown order). */
export const MINDMAP_THEME_NAMES: readonly string[] = Object.keys(MINDMAP_THEME_COLORS)

/** Default fallback when the requested theme is unknown / missing. */
export const MINDMAP_DEFAULT_THEME = 'feishu'

/**
 * Resolve the palette for a theme name. Falls back to the default theme
 * (feishu) when the input is unknown / empty. Always returns the four
 * required keys.
 */
export function loadMindmapColors(theme: string | null | undefined): MindMapThemeColors {
  if (theme && MINDMAP_THEME_COLORS[theme]) {
    return MINDMAP_THEME_COLORS[theme]
  }
  return MINDMAP_THEME_COLORS[MINDMAP_DEFAULT_THEME]
}
