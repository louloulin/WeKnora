<!--
  MindMapSidebar.vue — selected-node inspector for the MindMap editor.
-->
<template>
  <aside class="mm-sidebar" v-if="node">
    <header>
      <strong>{{ node.label }}</strong>
      <button class="close" @click="$emit('close')">×</button>
    </header>
    <div class="row">
      <label>类型</label>
      <select v-model="form.node_type" @change="emit()">
        <option value="text">文本</option>
        <option value="image">图片</option>
        <option value="link">链接</option>
        <option value="doc_ref">关联文档</option>
        <option value="task">任务</option>
        <option value="formula">公式</option>
      </select>
    </div>
    <div class="row">
      <label>标签</label>
      <input v-model="form.label" @change="emit()" />
    </div>
    <div class="row">
      <label>正文</label>
      <textarea v-model="form.body" rows="4" @change="emit()"></textarea>
    </div>
    <div class="row">
      <label>颜色</label>
      <input v-model="form.color" type="color" @change="emit()" />
    </div>
    <div class="row" v-if="form.node_type === 'doc_ref'">
      <label>关联文档 ID</label>
      <input v-model="form.doc_ref" placeholder="wiki / collab doc id" @change="emit()" />
    </div>
    <div class="row" v-if="form.node_type === 'formula'">
      <label>公式</label>
      <input v-model="form.formula" placeholder="SUM(2+2)" @change="emit()" />
    </div>
    <div class="actions">
      <button class="btn danger" @click="$emit('delete')">删除</button>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { MindMapNode } from '../../api/mindmap'

const props = defineProps<{ node: MindMapNode | null; mapId: string; tenantId: number; kbId?: string }>()
const emit = defineEmits<{
  (e: 'update', patch: Partial<MindMapNode>): void
  (e: 'delete'): void
  (e: 'close'): void
}>()

const form = reactive({
  node_type: 'text',
  label: '',
  body: '',
  color: '#58a6ff',
  doc_ref: '',
  formula: '',
})

watch(() => props.node, (n) => {
  if (!n) return
  form.node_type = n.node_type || 'text'
  form.label = n.label || ''
  form.body = n.body || ''
  form.color = n.color || '#58a6ff'
  form.doc_ref = n.doc_ref || ''
  form.formula = n.formula || ''
}, { immediate: true })

function emit() {
  if (!props.node) return
  emit_({ ...form })
}

function emit_(patch: any) {
  emit('update', {
    node_type: patch.node_type,
    label: patch.label,
    body: patch.body,
    color: patch.color,
    doc_ref: patch.doc_ref || null,
    formula: patch.formula || null,
  })
}
</script>

<style scoped>
.mm-sidebar {
  position: absolute;
  top: 56px;
  right: 12px;
  width: 320px;
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 12px;
  z-index: 8;
}
header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.close {
  background: transparent;
  border: 0;
  color: #9da7b3;
  font-size: 18px;
  cursor: pointer;
}
.row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 8px;
}
label {
  font-size: 11px;
  color: #9da7b3;
}
input, textarea, select {
  background: #0d1117;
  color: #e6edf3;
  border: 1px solid #30363d;
  border-radius: 4px;
  padding: 4px 6px;
  font-size: 12px;
}
.actions {
  margin-top: 12px;
}
.btn.danger {
  background: #f85149;
  color: #fff;
  border: 0;
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
}
</style>
