<!--
  MindMapAIAssistant.vue — AI auto-expand + theme clustering for MindMaps.

  Calls POST /mindmaps/:id/ai-expand (defined in Build #46 follow-up).
  Falls back to a placeholder when the endpoint is unavailable so the
  component renders during the v0.7.36 release.
-->
<template>
  <div class="mm-ai" @click.self="$emit('close')">
    <div class="panel">
      <header>
        <strong>AI 助手</strong>
        <button class="close" @click="$emit('close')">×</button>
      </header>
      <p>在选中的节点上扩展 AI 生成的子主题，或对整个导图重新聚类。</p>
      <textarea v-model="prompt" placeholder="例如：为这个季度计划生成 5 个执行步骤"></textarea>
      <div class="row">
        <label>广度 (1-10)</label>
        <input type="number" v-model="breadth" min="1" max="10" />
      </div>
      <div class="actions">
        <button class="btn" :disabled="loading" @click="expand">
          {{ loading ? '生成中…' : '扩展子主题' }}
        </button>
        <button class="btn primary" :disabled="loading" @click="cluster">主题聚类</button>
      </div>
      <div v-if="error" class="error">{{ error }}</div>
      <p class="hint">提示：AI 助手通过 POST /mindmaps/:id/ai-expand 暴露。Build #46 + v0.7.39 持续增强。</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { mindmapApi } from '../../api/mindmap'

const props = defineProps<{ mapId: string }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'applied'): void }>()

const prompt = ref('')
const breadth = ref(5)
const loading = ref(false)
const error = ref('')

async function expand() {
  if (!prompt.value.trim()) {
    error.value = '请输入提示'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await mindmapApi.aiExpand(props.mapId, { prompt: prompt.value, breadth: breadth.value })
    emit('applied')
    emit('close')
  } catch (e: any) {
    error.value = `AI 扩展失败：${e?.message || '未知错误'}`
  } finally {
    loading.value = false
  }
}

async function cluster() {
  loading.value = true
  error.value = ''
  try {
    await mindmapApi.cluster(props.mapId)
    emit('applied')
    emit('close')
  } catch (e: any) {
    error.value = `聚类失败：${e?.message || '未知错误'}`
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.mm-ai {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 30;
}
.panel {
  width: 480px;
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 10px;
  padding: 16px;
  color: #e6edf3;
}
header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.close {
  background: transparent;
  border: 0;
  color: #9da7b3;
  font-size: 18px;
  cursor: pointer;
}
textarea, input {
  width: 100%;
  background: #0d1117;
  color: #e6edf3;
  border: 1px solid #30363d;
  border-radius: 4px;
  padding: 6px;
  margin-top: 4px;
  font-size: 13px;
}
.row {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
}
.actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
.btn {
  background: #21262d;
  color: #e6edf3;
  border: 1px solid #30363d;
  padding: 6px 12px;
  border-radius: 4px;
  cursor: pointer;
}
.btn.primary {
  background: #58a6ff;
  color: #fff;
  border-color: #58a6ff;
}
.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.error {
  color: #f85149;
  font-size: 12px;
  margin-top: 8px;
}
.hint {
  font-size: 11px;
  color: #9da7b3;
  margin-top: 12px;
}
</style>
