<!--
  MindMapView.vue — top-level page that hosts a MindMap editor.

  Route: /mindmaps/:id
  Loads the map on mount and renders MindMapEditor. Provides the export
  download flow (Markdown / OPML / XMind) triggered by the toolbar.
-->
<template>
  <div class="mm-page">
    <header class="head">
      <button class="btn ghost" @click="$router.back()">← 返回</button>
      <h2>{{ mapTitle || '思维导图' }}</h2>
      <div class="meta">
        <span>{{ nodes.length }} 节点</span>
        <a v-if="mapId" class="btn" :href="exportURL('markdown')" download>导出 Markdown</a>
        <a v-if="mapId" class="btn" :href="exportURL('opml')" download>导出 OPML</a>
        <a v-if="mapId" class="btn" :href="exportURL('xmind')" download>导出 XMind</a>
      </div>
    </header>
    <MindMapEditor
      v-if="mapId"
      :map-id="mapId"
      :tenant-id="tenantId"
      :kb-id="kbId"
      @export="onExport"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import MindMapEditor from '../../components/mindmap/MindMapEditor.vue'
import { mindmapApi, type MindMap } from '../../api/mindmap'

const route = useRoute()
const mapId = computed(() => (route.params.id as string) || '')
const kbId = computed(() => (route.query.kb_id as string) || '')
const tenantId = computed(() => Number(route.query.tenant_id) || 0)
const mapTitle = ref('')
const nodes = ref<any[]>([])

async function loadTitle() {
  if (!mapId.value) return
  try {
    const m = await mindmapApi.get(mapId.value) as MindMap
    mapTitle.value = m.title
  } catch {
    mapTitle.value = ''
  }
  try {
    nodes.value = await mindmapApi.listNodes(mapId.value)
  } catch {
    nodes.value = []
  }
}
loadTitle()

function exportURL(fmt: string) {
  const base = import.meta.env.VITE_API_BASE_URL || '/api/v1'
  return `${base}/mindmaps/${mapId.value}/export?format=${fmt}`
}

function onExport(_fmt: string) {
  // Editor handles the emission; the toolbar buttons also offer direct
  // download links via <a href download>. This hook is reserved for
  // analytics + telemetry in v0.7.39+.
}
</script>

<style scoped>
.mm-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #0e1117;
  color: #e6edf3;
}
.head {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px 20px;
  border-bottom: 1px solid #30363d;
  background: #161b22;
}
.head h2 {
  margin: 0;
  font-size: 18px;
  flex: 1;
}
.meta {
  display: flex;
  gap: 8px;
  align-items: center;
}
.btn {
  background: #21262d;
  color: #e6edf3;
  border: 1px solid #30363d;
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
  text-decoration: none;
  font-size: 12px;
}
.btn.ghost {
  background: transparent;
  border: 1px solid #30363d;
}
</style>
