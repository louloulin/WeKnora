// v0.7.20.x — Wiki Synced Block TipTap node.
//
// Storage shape: `[[sync:BLOCK_ID]]` markers persisted in markdown /
// `content_json` so existing wiki pages keep their data, while the
// renderer resolves them to the canonical block content fetched from
// /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id.
//
// The node is atomic at the schema level (no inner content editable
// directly) — edits must go through the canonical block's own editor
// so every reference re-renders to the latest version automatically.
// We expose an `attrs` bag for renderer hints (block_id, version,
// stale flag, last-rendered timestamp).

import { Node, mergeAttributes } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import SyncBlockView from './SyncBlockView.vue'

export interface SyncBlockAttrs {
  block_id: string
  version: number
  stale: boolean
  rendered_at: string
}

export const SYNC_BLOCK_REGEX = /\[\[sync:([a-zA-Z0-9_-]+)\]\]/g

export const SyncBlockNode = Node.create({
  name: 'syncBlock',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: false,

  addAttributes() {
    return {
      block_id: { default: '' },
      version: { default: 0 },
      stale: { default: false },
      rendered_at: { default: '' },
    }
  },

  parseHTML() {
    return [
      {
        tag: 'div[data-sync-block-id]',
        getAttrs: (el) => {
          if (typeof el === 'string') return false
          const node = el as HTMLElement
          const blockId = node.getAttribute('data-sync-block-id') || ''
          if (!blockId) return false
          return {
            block_id: blockId,
            version: Number(node.getAttribute('data-version') || '0'),
            stale: node.getAttribute('data-stale') === 'true',
            rendered_at: node.getAttribute('data-rendered-at') || '',
          }
        },
      },
    ]
  },

  renderHTML({ HTMLAttributes }) {
    const blockId = (HTMLAttributes as SyncBlockAttrs).block_id || ''
    const version = String((HTMLAttributes as SyncBlockAttrs).version || 0)
    const stale = (HTMLAttributes as SyncBlockAttrs).stale ? 'true' : 'false'
    const renderedAt = (HTMLAttributes as SyncBlockAttrs).rendered_at || ''
    return [
      'div',
      mergeAttributes(HTMLAttributes, {
        'data-sync-block-id': blockId,
        'data-version': version,
        'data-stale': stale,
        'data-rendered-at': renderedAt,
        class: 'wiki-sync-block-render',
      }),
      `[[sync:${blockId}]]`,
    ]
  },

  addNodeView() {
    return VueNodeViewRenderer(SyncBlockView)
  },

  // The picker inserts `[[sync:UUID]]` as plain text. When the editor
  // parses that into prosemirror, convert the text into the node so
  // the node-view takes over rendering.
  addInputRules() {
    return []
  },
})

// Helper: insert a synced-block node at the current selection, pulling
// block metadata out of the picker result.
export function makeSyncBlockAttrs(
  blockId: string,
  version = 0,
  stale = false,
  renderedAt = '',
): SyncBlockAttrs {
  return {
    block_id: blockId,
    version,
    stale,
    rendered_at: renderedAt,
  }
}

// Helper: parse a markdown string for `[[sync:UUID]]` markers and return
// the block IDs in document order. The backend uses the same regex in
// internal/application/service/wiki_sync_block.go when reconciling.
export function extractSyncMarkers(text: string): string[] {
  const out: string[] = []
  let m: RegExpExecArray | null
  SYNC_BLOCK_REGEX.lastIndex = 0
  while ((m = SYNC_BLOCK_REGEX.exec(text)) !== null) {
    if (m[1]) out.push(m[1])
  }
  return out
}
