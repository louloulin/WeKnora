<!--
  MindMapListView.vue — list all MindMaps for the current tenant/KB.
-->
<template>
  <div class="mm-list-page">
    <header>
      <h2>思维导图</h2>
      <button class="btn primary" @click="onCreate">+ 新建导图</button>
    </header>
    <div v-if="loading" class="loading">加载中…</div>
    <div v-else-if="maps.length === 0" class="empty">还没有导图，点右上角创建一个。</div>
    <ul v-else class="grid">
      <li v-for="m in maps" :key="m.id" class="card" @click="open(m)">
        <div class="title">{{ m.title }}</div>
        <div class="meta">
          <span class="layout">{{ m.layout }}</span>
          <span class="theme">{{ m.theme }}</span>
          <span class="updated">{{ formatShortDate(m.updated_at) }}</span>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { mindmapApi, type MindMap } from '../../api/mindmap'
// v0.7.111 — extract date helpers for testability.
import { formatShortDate } from '../../utils/mindmapFormat'

const router = useRouter()
const maps = ref<MindMap[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    const r = await mindmapApi.list()
    maps.value = r.items
  } finally {
    loading.value = false
  }
})

async function onCreate() {
  const title = window.prompt('导图标题') || '未命名导图'
  const m = await mindmapApi.create({ title, layout: 'tree' })
  router.push(`/mindmaps/${m.id}`)
}

function open(m: MindMap) {
  router.push(`/mindmaps/${m.id}`)
}

// v0.7.111 — formatDate moved to utils/mindmapFormat (formatShortDate).
</script>

<style scoped>
.mm-list-page {
  max-width: 1080px;
  margin: 0 auto;
  padding: 32px 24px;
  color: #e6edf3;
}
header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
header h2 {
  margin: 0;
  font-size: 22px;
}
.btn.primary {
  background: #58a6ff;
  color: #fff;
  border: 0;
  padding: 6px 12px;
  border-radius: 4px;
  cursor: pointer;
}
.loading, .empty {
  padding: 24px;
  text-align: center;
  color: #9da7b3;
}
.grid {
  list-style: none;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
  padding: 0;
  margin: 0;
}
.card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 12px 14px;
  cursor: pointer;
  transition: border-color 0.15s;
}
.card:hover {
  border-color: #58a6ff;
}
.title {
  font-weight: 600;
  font-size: 14px;
  margin-bottom: 8px;
}
.meta {
  display: flex;
  gap: 8px;
  font-size: 11px;
  color: #9da7b3;
}
.layout, .theme {
  background: rgba(88,166,255,.1);
  color: #58a6ff;
  padding: 1px 6px;
  border-radius: 3px;
}
</style>
