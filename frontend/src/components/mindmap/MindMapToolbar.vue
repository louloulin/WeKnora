<!--
  MindMapToolbar.vue — Build #43 toolbar for the MindMap editor.

  Layout selector (5 layouts), theme picker (5 themes), auto-layout,
  export, add-child, reset-view, AI assistant trigger.
-->
<template>
  <div class="mm-toolbar">
    <select v-model="localLayout" @change="$emit('select-layout', localLayout)">
      <option value="tree">左对齐树</option>
      <option value="fishbone">鱼骨图</option>
      <option value="timeline">时间线</option>
      <option value="radial">放射</option>
      <option value="free">自由拖拽</option>
    </select>
    <select v-model="localTheme" @change="$emit('select-theme', localTheme)">
      <option value="feishu">飞书主题</option>
      <option value="notion">Notion 主题</option>
      <option value="tana">Tana 主题</option>
      <option value="coda">Coda 主题</option>
      <option value="dark">暗色</option>
    </select>
    <button class="btn" @click="$emit('auto-layout')" :disabled="saving">自动布局</button>
    <button class="btn" @click="$emit('reset-view')">重置视图</button>
    <button class="btn primary" @click="$emit('add-child')">+ 子节点</button>
    <div class="export">
      <button class="btn" @click="$emit('export', 'markdown')">Markdown</button>
      <button class="btn" @click="$emit('export', 'opml')">OPML</button>
      <button class="btn" @click="$emit('export', 'xmind')">XMind</button>
    </div>
    <div class="status">
      <span v-if="saving">保存中…</span>
      <span v-else-if="dirty">未保存</span>
      <span v-else>{{ nodeCount }} 节点</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{
  layout: string
  theme: string
  dirty: boolean
  saving: boolean
  nodeCount: number
}>()
defineEmits<{
  (e: 'select-layout', layout: string): void
  (e: 'select-theme', theme: string): void
  (e: 'auto-layout'): void
  (e: 'add-child'): void
  (e: 'reset-view'): void
  (e: 'export', fmt: string): void
}>()

const localLayout = ref(props.layout)
const localTheme = ref(props.theme)
watch(() => props.layout, (v) => (localLayout.value = v))
watch(() => props.theme, (v) => (localTheme.value = v))
</script>

<style scoped>
.mm-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  background: #161b22;
  border-bottom: 1px solid #30363d;
}
.btn {
  background: #21262d;
  color: #e6edf3;
  border: 1px solid #30363d;
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
}
.btn:hover:not(:disabled) {
  background: #30363d;
}
.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn.primary {
  background: #58a6ff;
  color: #fff;
  border-color: #58a6ff;
}
.export {
  display: flex;
  gap: 4px;
  margin-left: auto;
  padding-left: 12px;
  border-left: 1px solid #30363d;
}
.status {
  font-size: 12px;
  color: #9da7b3;
}
select {
  background: #21262d;
  color: #e6edf3;
  border: 1px solid #30363d;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 12px;
}
</style>
