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
        <button
          type="button"
          class="collab-editor-view__audit-toggle"
          @click="auditVisible = !auditVisible"
          data-testid="audit-toggle"
        >
          {{ auditVisible ? '隐藏历史' : '查看历史' }}
        </button>
        <button
          type="button"
          class="collab-editor-view__share-toggle"
          @click="shareVisible = !shareVisible"
          data-testid="share-toggle"
        >
          {{ shareVisible ? '隐藏分享' : '分享设置' }}
        </button>
        <div v-if="auditVisible" class="collab-editor-view__audit-wrap">
          <CollabAuditTimeline :doc-id="doc.id" />
        </div>
        <div v-if="shareVisible" class="collab-editor-view__share-wrap">
          <CollabSharePasswordPanel
            :doc-id="doc.id"
            :initial-share-token="doc.share_token || ''"
            :initial-protected="!!doc.share_password_protected"
            :initial-expires-at="doc.share_expires_at || null"
            @updated="onShareUpdated"
            @disabled="onShareDisabled"
          />
        </div>
      </div>
      <div class="collab-editor-view__main">
        <CollabDocProEditor v-if="doc.doc_kind === 'doc'" :doc-id="doc.id" :title="doc.title" :token="token" :display-name="displayName" />
        <CollabSheetEditor v-else-if="doc.doc_kind === 'sheet'" :doc-id="doc.id" :title="doc.title" :token="token" :display-name="displayName" />
        <CollabSlideKonvaEditor
          v-else-if="doc.doc_kind === 'slide'"
          :doc-id="doc.id"
          :title="doc.title"
          :token="token"
          :display-name="displayName"
          :tenant-id="tenantId"
        />
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
import CollabSlideKonvaEditor from '@/components/collab/CollabSlideKonvaEditor.vue'
import CollabAuditTimeline from '@/components/collab/CollabAuditTimeline.vue'
import CollabSharePasswordPanel from '@/components/collab/CollabSharePasswordPanel.vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const doc = ref<CollabDoc | null>(null)
const loading = ref(true)
const auditVisible = ref(false)
const shareVisible = ref(false)
const error = ref<string | null>(null)
const syncing = ref(false)
const authStore = useAuthStore()

const token = ref('')
const displayName = ref('匿名用户')
const tenantId = ref<number | string>(
  (authStore as any).user?.tenant_id ?? (authStore as any).user?.tenantId ?? 0,
)

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

const onShareUpdated = (payload: { share_token: string; protected: boolean; expires_at: string | null }) => {
  if (doc.value) {
    doc.value.share_token = payload.share_token
    doc.value.share_password_protected = payload.protected
    doc.value.share_expires_at = payload.expires_at
  }
  MessagePlugin.success('分享设置已更新')
}

const onShareDisabled = () => {
  if (doc.value) {
    doc.value.share_token = ''
    doc.value.share_password_protected = false
    doc.value.share_expires_at = null
  }
  MessagePlugin.success('分享链接已禁用')
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
.collab-editor-view__audit-toggle {
  margin-top: 8px;
  padding: 6px 12px;
  background: transparent;
  color: var(--td-brand-color-7);
  border: 1px solid var(--td-brand-color-7);
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
}
.collab-editor-view__audit-toggle:hover { background: var(--td-brand-color-1); }
.collab-editor-view__audit-wrap {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed var(--td-component-stroke);
}
.collab-editor-view__share-toggle {
  margin-top: 8px;
  padding: 6px 12px;
  background: transparent;
  color: var(--td-brand-color-7);
  border: 1px solid var(--td-brand-color-7);
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
}
.collab-editor-view__share-toggle:hover { background: var(--td-brand-color-1); }
.collab-editor-view__share-wrap {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed var(--td-component-stroke);
}
</style>
