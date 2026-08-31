<!--
  CollabDocEditorView — mounts the right editor for the doc kind.
  DOC → CollabDocEditor (TipTap + Yjs)
  SHEET → CollabSheetEditor (Y.Map / Y.Array MVP)
  SLIDE → CollabSlideEditor (Y.Array<slide> + .pptx export)
-->
<template>
  <div class="collab-editor-view">
    <div v-if="loading" class="collab-editor-view__loading">加载中...</div>
    <div v-else-if="error" class="collab-editor-view__error">{{ error }}</div>
    <template v-else-if="doc">
      <div class="collab-editor-view__sidebar">
        <router-link :to="{ name: 'collabDocList' }" class="collab-editor-view__back">← 返回列表</router-link>
        <div class="collab-editor-view__title">{{ doc.title }}</div>
        <div class="collab-editor-view__meta">类型：{{ kindLabel(doc.doc_kind) }}</div>
        <button class="collab-editor-view__sync-kb" @click="onSyncToKB" :disabled="syncing">
          {{ syncing ? '同步中...' : '同步到知识库' }}
        </button>
      </div>
      <div class="collab-editor-view__main">
        <CollabDocProEditor v-if="doc.doc_kind === 'doc'" :doc-id="doc.id" :title="doc.title" :token="token" :display-name="displayName" />
        <CollabSheetEditor v-else-if="doc.doc_kind === 'sheet'" :doc-id="doc.id" :title="doc.title" :token="token" :display-name="displayName" />
        <CollabSlideEditor v-else-if="doc.doc_kind === 'slide'" :doc-id="doc.id" :title="doc.title" :token="token" :display-name="displayName" />
        <div v-else>不支持的文档类型</div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getCollabDoc, type CollabDoc } from '@/api/collabDoc'
import CollabDocProEditor from '@/components/collab/CollabDocProEditor.vue'
import CollabSheetEditor from '@/components/collab/CollabSheetEditor.vue'
import CollabSlideEditor from '@/components/collab/CollabSlideEditor.vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const doc = ref<CollabDoc | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const syncing = ref(false)
const authStore = useAuthStore()

const token = ref('')
const displayName = ref('匿名用户')

const kindLabel = (k: string) => ({ doc: '文档', sheet: '表格', slide: '幻灯片' }[k] || k)

const load = async () => {
  loading.value = true
  error.value = null
  try {
    doc.value = await getCollabDoc(route.params.id as string)
    token.value = authStore.token || ''
    displayName.value = authStore.user?.username || '匿名用户'
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

const onSyncToKB = async () => {
  if (!doc.value) return
  syncing.value = true
  try {
    // Use the markdown export endpoint and re-ingest into the local KB.
    // Real impl: this would dispatch a fetch of /export, then a
    // /knowledge/:kbId/documents upload; for the MVP we POST to a single
    // round-trip endpoint that wraps both.
    const res = await fetch(`/api/v1/collaborative-docs/${doc.value.id}/sync-to-kb`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.value}` },
    })
    if (!res.ok) throw new Error(`sync failed: ${res.status}`)
    MessagePlugin.success('已提交同步任务')
  } catch (e: any) {
    MessagePlugin.error(`同步失败：${e?.message || e}`)
  } finally {
    syncing.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.collab-editor-view { display: flex; height: 100%; }
.collab-editor-view__sidebar { width: 240px; padding: 16px; border-right: 1px solid var(--td-component-stroke); display: flex; flex-direction: column; gap: 12px; background: var(--td-bg-color-container); }
.collab-editor-view__back { font-size: 13px; color: var(--td-brand-color-7); text-decoration: none; }
.collab-editor-view__title { font-size: 16px; font-weight: 600; }
.collab-editor-view__meta { font-size: 12px; color: var(--td-text-color-secondary); }
.collab-editor-view__sync-kb { margin-top: auto; padding: 8px 12px; background: var(--td-brand-color-7); color: white; border: none; border-radius: 4px; cursor: pointer; }
.collab-editor-view__main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.collab-editor-view__loading, .collab-editor-view__error { padding: 24px; }
.collab-editor-view__error { color: var(--td-error-color-7); }
</style>
