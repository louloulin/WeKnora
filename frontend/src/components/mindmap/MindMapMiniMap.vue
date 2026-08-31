<!--
  MindMapMiniMap.vue — thumbnail navigator for the MindMap editor.
  Shows all nodes as tiny dots; clicking a dot focuses the canvas.
-->
<template>
  <div class="mm-minimap" @click="onClick">
    <svg :viewBox="viewBox" preserveAspectRatio="xMidYMid meet">
      <rect :width="bb.w" :height="bb.h" fill="#161b22" />
      <line
        v-for="(e, i) in edges"
        :key="i"
        :x1="e.x1" :y1="e.y1" :x2="e.x2" :y2="e.y2"
        stroke="#30363d"
        stroke-width="2"
      />
      <circle
        v-for="n in nodes"
        :key="n.id"
        :cx="n.x" :cy="n.y" :r="n.id === selected ? 8 : 4"
        :fill="n.id === selected ? '#f85149' : (n.color || '#58a6ff')"
      />
    </svg>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { MindMapNode } from '../../api/mindmap'

const props = defineProps<{ nodes: MindMapNode[]; selected?: string | null }>()
const emit = defineEmits<{ (e: 'focus', id: string): void }>()

const bb = computed(() => {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  for (const n of props.nodes) {
    minX = Math.min(minX, n.x - n.width / 2)
    minY = Math.min(minY, n.y - n.height / 2)
    maxX = Math.max(maxX, n.x + n.width / 2)
    maxY = Math.max(maxY, n.y + n.height / 2)
  }
  if (!isFinite(minX)) {
    return { x: 0, y: 0, w: 200, h: 120 }
  }
  // Padding around bounding box.
  const pad = 20
  return {
    x: minX - pad,
    y: minY - pad,
    w: (maxX - minX) + pad * 2,
    h: (maxY - minY) + pad * 2,
  }
})

const viewBox = computed(() => `${bb.value.x} ${bb.value.y} ${bb.value.w} ${bb.value.h}`)

const edges = computed(() => {
  const out: { x1: number; y1: number; x2: number; y2: number }[] = []
  const byID = new Map(props.nodes.map((n) => [n.id, n]))
  for (const n of props.nodes) {
    const parent = n.parent_id && byID.get(n.parent_id)
    if (!parent) continue
    out.push({ x1: parent.x, y1: parent.y, x2: n.x, y2: n.y })
  }
  return out
})

function onClick(ev: MouseEvent) {
  const target = ev.target as SVGElement
  if (target.tagName === 'circle') {
    const id = target.getAttribute('data-id')
    if (id) emit('focus', id)
  }
}
</script>

<style scoped>
.mm-minimap {
  position: absolute;
  right: 12px;
  bottom: 12px;
  width: 180px;
  height: 120px;
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 6px;
  overflow: hidden;
  z-index: 6;
}
svg {
  width: 100%;
  height: 100%;
}
</style>
