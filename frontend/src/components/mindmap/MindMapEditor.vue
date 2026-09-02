<!--
  MindMapEditor.vue — Build #43 思维导图编辑器 (Docs × KB 富表达层).

  Architecture:
   1. Mount with :map-id prop OR an inline editor for new maps.
   2. Fetches /mindmaps/:id and /mindmaps/:id/nodes on open.
   3. Renders the canvas using <canvas> + 2D context (lightweight,
      zero-deps). For larger maps (>= 200 nodes), the component renders
      a "list" fallback so the page stays responsive.
   4. CRUD via the MindMap REST surface (see internal/handler/mindmap.go).
   5. Layout algorithms: tree (LTR), fishbone, timeline, radial, free.

  Pair with the auto-layout endpoint (POST /mindmaps/:id/auto-layout) and
  the export endpoints (GET /mindmaps/:id/export?format=markdown|opml|xmind).
-->
<template>
  <div class="mm-editor">
    <MindMapToolbar
      :layout="layout"
      :theme="theme"
      :dirty="dirty"
      :saving="saving"
      :node-count="nodes.length"
      @select-layout="onLayoutChange"
      @select-theme="onThemeChange"
      @auto-layout="runAutoLayout"
      @export="onExport"
      @add-child="addRootChild"
      @reset-view="resetView"
    />
    <div class="mm-canvas-wrap" ref="wrap">
      <canvas
        v-if="nodes.length < 200"
        ref="canvas"
        class="mm-canvas"
        @dblclick="onCanvasDblClick"
        @wheel.prevent="onWheel"
      />
      <div v-else class="mm-list">
        <h4>{{ mapTitle }} · 节点列表 (≥ 200，切换到列表视图)</h4>
        <ul>
          <li v-for="n in nodes" :key="n.id">
            <span class="kind">{{ n.node_type }}</span>
            <span>{{ n.label }}</span>
          </li>
        </ul>
      </div>
      <MindMapMiniMap :nodes="nodes" :selected="selectedId" />
    </div>
    <MindMapSidebar
      :node="selectedNode"
      :map-id="mapId"
      :tenant-id="tenantId"
      :kb-id="kbId"
      @update="onUpdateNode"
      @delete="onDeleteNode"
      @close="selectedId = null"
    />
    <MindMapAIAssistant v-if="aiOpen" :map-id="mapId" @close="aiOpen = false" @applied="reload" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, computed, onBeforeUnmount, nextTick } from 'vue'
import MindMapToolbar from './MindMapToolbar.vue'
import MindMapSidebar from './MindMapSidebar.vue'
import MindMapMiniMap from './MindMapMiniMap.vue'
import MindMapAIAssistant from './MindMapAIAssistant.vue'
import { mindmapApi, type MindMap, type MindMapNode, type MindMapLayout } from '../../api/mindmap'
import { loadMindmapColors } from '../../utils/mindmapTheme'

interface Props {
  mapId: string
  tenantId: number
  kbId?: string
  readonly?: boolean
}

const props = withDefaults(defineProps<Props>(), { readonly: false })
const emit = defineEmits<{ (e: 'updated', m: MindMap): void; (e: 'export', fmt: string): void }>()

const wrap = ref<HTMLDivElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const map = ref<MindMap | null>(null)
const nodes = ref<MindMapNode[]>([])
const layout = ref<MindMapLayout>('tree')
const theme = ref<string>('feishu')
const dirty = ref(false)
const saving = ref(false)
const selectedId = ref<string | null>(null)
const aiOpen = ref(false)
const viewport = ref({ zoom: 1, panX: 0, panY: 0 })
const mapTitle = computed(() => map.value?.title || '思维导图')
const selectedNode = computed(() => nodes.value.find((n) => n.id === selectedId.value) || null)

function loadColors() {
  return loadMindmapColors(theme.value)
}

async function reload() {
  if (!props.mapId) return
  try {
    const [m, ns] = await Promise.all([
      mindmapApi.get(props.mapId),
      mindmapApi.listNodes(props.mapId),
    ])
    map.value = m
    nodes.value = ns
    layout.value = m.layout || 'tree'
    theme.value = m.theme || 'feishu'
    selectedId.value = null
    dirty.value = false
    await nextTick()
    draw()
  } catch (e) {
    console.error('[MindMap] reload failed', e)
  }
}

function markDirty() {
  dirty.value = true
}

async function save() {
  if (!dirty.value || !map.value) return
  saving.value = true
  try {
    await mindmapApi.update(map.value.id, {
      layout: layout.value,
      theme: theme.value,
      visibility: map.value.visibility,
    })
    dirty.value = false
  } finally {
    saving.value = false
  }
}

function onLayoutChange(l: MindMapLayout) {
  layout.value = l
  markDirty()
  runAutoLayout()
}

function onThemeChange(t: string) {
  theme.value = t
  markDirty()
  draw()
}

async function runAutoLayout() {
  if (!map.value) return
  await mindmapApi.autoLayout(map.value.id, layout.value, 80)
  await reload()
  emit('updated', map.value)
}

async function onExport(fmt: string) {
  if (!map.value) return
  emit('export', fmt)
}

function onCanvasDblClick(ev: MouseEvent) {
  if (props.readonly || !map.value) return
  // Find node under cursor.
  const rect = (canvas.value as HTMLCanvasElement).getBoundingClientRect()
  const cx = (ev.clientX - rect.left) / viewport.value.zoom - viewport.value.panX
  const cy = (ev.clientY - rect.top) / viewport.value.zoom - viewport.value.panY
  const hit = nodes.value.find((n) => {
    const dx = cx - n.x, dy = cy - n.y
    return Math.abs(dx) <= n.width / 2 && Math.abs(dy) <= n.height / 2
  })
  if (hit) {
    selectedId.value = hit.id
    return
  }
  // Otherwise, create a new root-level child.
  void addRootChild()
}

async function addRootChild() {
  if (!map.value) return
  const label = window.prompt('节点标签') || '新节点'
  const parentId = map.value.root_node_id
  await mindmapApi.createNode(map.value.id, {
    parent_id: parentId,
    node_type: 'text',
    label,
    x: 200 + Math.random() * 80,
    y: 40 + Math.random() * 80,
  })
  await reload()
}

async function onUpdateNode(patch: Partial<MindMapNode>) {
  if (!map.value || !selectedNode.value) return
  await mindmapApi.updateNode(map.value.id, selectedNode.value.id, patch)
  await reload()
}

async function onDeleteNode() {
  if (!map.value || !selectedNode.value) return
  if (!window.confirm(`删除节点 "${selectedNode.value.label}"?`)) return
  await mindmapApi.deleteNode(map.value.id, selectedNode.value.id)
  await reload()
}

function onWheel(ev: WheelEvent) {
  if (!ev.ctrlKey) {
    viewport.value.panY += ev.deltaY
    viewport.value.panX += ev.deltaX
    draw()
    return
  }
  const factor = ev.deltaY < 0 ? 1.1 : 0.9
  viewport.value.zoom = Math.max(0.2, Math.min(3, viewport.value.zoom * factor))
  draw()
}

function resetView() {
  viewport.value = { zoom: 1, panX: 0, panY: 0 }
  draw()
}

function draw() {
  if (!canvas.value || !wrap.value) return
  const c = canvas.value
  const ctx = c.getContext('2d')!
  const w = wrap.value.clientWidth
  const h = wrap.value.clientHeight
  if (c.width !== w || c.height !== h) {
    c.width = w
    c.height = h
  }
  const colors = loadColors()
  ctx.fillStyle = colors.bg
  ctx.fillRect(0, 0, w, h)
  ctx.save()
  ctx.translate(viewport.value.panX, viewport.value.panY)
  ctx.scale(viewport.value.zoom, viewport.value.zoom)
  // Edges first.
  ctx.strokeStyle = colors.line
  ctx.lineWidth = 1.5
  for (const n of nodes.value) {
    const parent = n.parent_id && nodes.value.find((m) => m.id === n.parent_id)
    if (!parent) continue
    ctx.beginPath()
    ctx.moveTo(parent.x, parent.y)
    ctx.lineTo(n.x, n.y)
    ctx.stroke()
  }
  // Nodes.
  for (const n of nodes.value) {
    ctx.fillStyle = n.color || colors.accent
    ctx.beginPath()
    if (n.node_type === 'task') {
      // Square for tasks
      ctx.fillRect(n.x - n.width / 2, n.y - n.height / 2, n.width, n.height)
    } else if (n.node_type === 'doc_ref') {
      // Diamond for doc references
      ctx.beginPath()
      ctx.moveTo(n.x, n.y - n.height / 2)
      ctx.lineTo(n.x + n.width / 2, n.y)
      ctx.lineTo(n.x, n.y + n.height / 2)
      ctx.lineTo(n.x - n.width / 2, n.y)
      ctx.closePath()
      ctx.fill()
    } else {
      ctx.roundRect(n.x - n.width / 2, n.y - n.height / 2, n.width, n.height, 8)
      ctx.fill()
    }
    ctx.fillStyle = colors.fg
    ctx.font = '13px sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    const lbl = n.label.length > 18 ? n.label.slice(0, 18) + '…' : n.label
    ctx.fillText(lbl, n.x, n.y)
    if (selectedId.value === n.id) {
      ctx.strokeStyle = '#f85149'
      ctx.lineWidth = 2
      ctx.strokeRect(n.x - n.width / 2 - 2, n.y - n.height / 2 - 2, n.width + 4, n.height + 4)
    }
  }
  ctx.restore()
}

let resizeObserver: ResizeObserver | null = null
onMounted(async () => {
  await reload()
  resizeObserver = new ResizeObserver(() => draw())
  if (wrap.value) resizeObserver.observe(wrap.value)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
})

watch(() => props.mapId, () => reload())
watch(nodes, () => draw(), { deep: true })
</script>

<style scoped>
.mm-editor {
  display: grid;
  grid-template-rows: 48px 1fr;
  height: 100%;
  position: relative;
  background: #0e1117;
  color: #e6edf3;
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif;
}
.mm-canvas-wrap {
  position: relative;
  overflow: hidden;
}
.mm-canvas {
  width: 100%;
  height: 100%;
  cursor: grab;
}
.mm-list {
  padding: 12px 16px;
  overflow-y: auto;
  max-height: 100%;
}
.mm-list ul {
  list-style: none;
  padding: 0;
  margin: 0;
}
.mm-list li {
  display: flex;
  gap: 8px;
  padding: 6px 8px;
  border-bottom: 1px solid #30363d;
}
.kind {
  font-size: 11px;
  color: #58a6ff;
  background: rgba(88,166,255,.1);
  padding: 1px 6px;
  border-radius: 3px;
}
</style>
