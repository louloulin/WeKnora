/**
 * mindmapTheme.test.ts — v0.7.111.x MindMap Phase 2 polish.
 *
 * Pure data + helpers, no DOM: validates that loadMindmapColors returns
 * the right palette per theme, falls back on unknown input, and the
 * raw palette map covers all five built-in themes with all four keys.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  MINDMAP_THEME_COLORS,
  MINDMAP_THEME_NAMES,
  MINDMAP_DEFAULT_THEME,
  loadMindmapColors,
  type MindMapThemeColors,
} from '../mindmapTheme'

test('palette covers every built-in theme with four keys (bg/fg/line/accent)', () => {
  for (const theme of MINDMAP_THEME_NAMES) {
    const c = MINDMAP_THEME_COLORS[theme]
    assert.ok(c, `palette missing for ${theme}`)
    const required: Array<keyof MindMapThemeColors> = ['bg', 'fg', 'line', 'accent']
    for (const key of required) {
      assert.ok(typeof c[key] === 'string' && c[key].length > 0, `${theme} missing ${key}`)
      assert.ok(c[key].startsWith('#'), `${theme}.${key} should be hex (got "${c[key]}")`)
    }
  }
  assert.equal(MINDMAP_THEME_NAMES.length, 5, 'expected 5 built-in themes')
})

test('loadMindmapColors returns the requested palette for known themes', () => {
  for (const theme of MINDMAP_THEME_NAMES) {
    const c = loadMindmapColors(theme)
    assert.deepEqual(c, MINDMAP_THEME_COLORS[theme])
  }
})

test('loadMindmapColors falls back to default (feishu) on unknown input', () => {
  const fb = loadMindmapColors('not-a-real-theme')
  assert.deepEqual(fb, MINDMAP_THEME_COLORS[MINDMAP_DEFAULT_THEME])
})

test('loadMindmapColors falls back on empty / null / undefined', () => {
  for (const input of ['', null, undefined]) {
    const fb = loadMindmapColors(input as unknown as string)
    assert.deepEqual(fb, MINDMAP_THEME_COLORS[MINDMAP_DEFAULT_THEME])
  }
})

test('dark theme has a dark bg (sanity)', () => {
  const dark = loadMindmapColors('dark')
  // very dark luminance: bg.startsWith('#0') or '#1' or '#2'
  assert.ok(/^#0|^#1|^#2/.test(dark.bg), `dark theme bg should start dark, got ${dark.bg}`)
})
