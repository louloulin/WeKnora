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
        <router-link :to="{ name: 'collabDocList' }" class="collab-editor-view__back">← <span>返回文档</span></router-link>
        <div class="collab-editor-view__identity">
          <span class="collab-editor-view__file-icon" :data-kind="doc.doc_kind">{{ doc.doc_kind === 'sheet' ? '▦' : doc.doc_kind === 'slide' ? '▧' : doc.doc_kind === 'form' ? '☷' : '▤' }}</span>
          <div><div class="collab-editor-view__title">{{ doc.title }}</div><div class="collab-editor-view__meta">{{ kindLabel(doc.doc_kind) }} · 私有文档</div></div>
        </div>
        <div class="collab-editor-view__collab-status"><span></span>实时保存 · 协作中</div>
        <div class="collab-editor-view__section-label">文档工具</div>
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
        <header class="collab-editor-view__topbar">
          <div class="collab-editor-view__topbar-title"><span class="collab-editor-view__topbar-kind">{{ kindLabel(doc.doc_kind) }}</span><strong>{{ doc.title }}</strong><span class="collab-editor-view__saved">已保存</span></div>
          <div class="collab-editor-view__topbar-actions"><span class="collab-editor-view__presence"><i></i>{{ displayName }}</span><button type="button" @click="shareVisible = !shareVisible">分享</button><button type="button" class="collab-editor-view__more" aria-label="更多操作">•••</button></div>
        </header>
        <CollabSlideThemePanel
          v-if="doc.doc_kind === 'slide'"
          class="collab-editor-view__slide-theme"
          @theme:apply="onSlideThemeApply"
        />
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
        <CollabFormEditor
          v-else-if="doc.doc_kind === 'form'"
          :doc-id="doc.id"
          :title="doc.title"
          :token="token"
          :display-name="displayName"
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
import CollabFormEditor from '@/components/collab/CollabFormEditor.vue'
import CollabAuditTimeline from '@/components/collab/CollabAuditTimeline.vue'
import CollabSharePasswordPanel from '@/components/collab/CollabSharePasswordPanel.vue'
import CollabSlideThemePanel from '@/components/collab/CollabSlideThemePanel.vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'
import type { SlideThemePreset } from '@/editor/slides/themes/genofficeThemes'

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

const kindLabel = (k: string) => ({ doc: '文档', sheet: '表格', slide: '幻灯片', form: '收集表' }[k] || k)

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

const onSlideThemeApply = (preset: SlideThemePreset) => {
  if (typeof window === 'undefined') return
  window.dispatchEvent(
    new CustomEvent('wk-slide-theme-apply', { detail: preset }),
  )
}

onMounted(load)
</script>

<style scoped>
.collab-editor-view__slide-theme { margin: 8px 12px 0; padding: 8px; border: 1px solid var(--td-component-stroke); border-radius: 6px; background: var(--app-surface-raised); }
.collab-editor-view { display: flex; height: 100%; min-height: 0; background: var(--app-page-bg); color: var(--app-text); }
.collab-editor-view__sidebar { width: 220px; flex: 0 0 220px; box-sizing: border-box; padding: 18px 14px; border-right: 1px solid var(--app-border); display: flex; flex-direction: column; gap: 10px; background: var(--app-surface-bg); overflow-y: auto; }
.collab-editor-view__back { display: inline-flex; align-items: center; gap: 5px; width: fit-content; margin-bottom: 10px; font-size: 12px; color: var(--app-text-muted); text-decoration: none; }
.collab-editor-view__back:hover { color: var(--td-brand-color); }
.collab-editor-view__identity { display:flex; align-items:center; gap:9px; }
.collab-editor-view__file-icon { width:30px; height:34px; display:grid; place-items:center; flex:none; border-radius:7px; background:color-mix(in srgb, var(--td-brand-color) 14%, var(--app-surface-raised)); color:var(--td-brand-color); font-size:17px; }
.collab-editor-view__file-icon[data-kind=sheet] { color:#46a6ff; background:rgba(70,166,255,.12); } .collab-editor-view__file-icon[data-kind=slide] { color:#f7a94a; background:rgba(247,169,74,.12); } .collab-editor-view__file-icon[data-kind=form] { color:#b78cff; background:rgba(183,140,255,.12); }
.collab-editor-view__title { font-size: 15px; font-weight: 650; line-height: 1.35; overflow-wrap: anywhere; }
.collab-editor-view__meta { font-size: 12px; color: var(--app-text-muted); }
.collab-editor-view__collab-status { display:flex; align-items:center; gap:7px; margin:8px 0 16px; color:var(--app-text-muted); font-size:11px; } .collab-editor-view__collab-status span { width:6px; height:6px; border-radius:50%; background:var(--td-brand-color); }
.collab-editor-view__section-label { color:var(--app-text-muted); font-size:10px; font-weight:700; letter-spacing:.08em; text-transform:uppercase; }
.collab-editor-view__sync-kb { margin-top: auto; padding: 9px 12px; background: var(--td-brand-color-6); color: var(--td-text-color-anti); border: 1px solid var(--td-brand-color-6); border-radius: 6px; cursor: pointer; font-weight: 600; }
.collab-editor-view__sync-kb:hover { background: var(--td-brand-color-5); }
.collab-editor-view__sync-kb:disabled { opacity: .55; cursor: not-allowed; }
.collab-editor-view__main { flex: 1; display: flex; flex-direction: column; min-width: 0; min-height: 0; overflow: hidden; }
.collab-editor-view__topbar { display:flex; align-items:center; justify-content:space-between; gap:16px; min-height:50px; padding:0 18px; border-bottom:1px solid var(--app-border); background:var(--app-surface-bg); }
.collab-editor-view__topbar-title, .collab-editor-view__topbar-actions { display:flex; align-items:center; gap:10px; min-width:0; } .collab-editor-view__topbar-title strong { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:13px; } .collab-editor-view__topbar-kind { padding:3px 6px; border-radius:4px; background:var(--app-surface-raised); color:var(--app-text-muted); font-size:10px; } .collab-editor-view__saved { color:var(--app-text-muted); font-size:11px; } .collab-editor-view__presence { display:flex; align-items:center; gap:5px; color:var(--app-text-muted); font-size:11px; } .collab-editor-view__presence i { width:7px; height:7px; border-radius:50%; background:var(--td-brand-color); }
.collab-editor-view__topbar-actions button { padding:6px 11px; border:1px solid var(--app-border); border-radius:6px; background:var(--app-surface-raised); color:var(--app-text); cursor:pointer; font-size:12px; } .collab-editor-view__topbar-actions button:hover { border-color:var(--td-brand-color); color:var(--td-brand-color); } .collab-editor-view__topbar-actions .collab-editor-view__more { padding:6px 8px; border-color:transparent; background:transparent; color:var(--app-text-muted); letter-spacing:2px; }
.collab-editor-view__loading, .collab-editor-view__error { padding: 24px; color: var(--app-text-muted); }
.collab-editor-view__error { color: var(--td-error-color-7); }
.collab-editor-view__audit-toggle {
  margin-top: 8px;
  padding: 6px 12px;
  background: transparent;
  color: var(--td-brand-color);
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 55%, var(--app-border));
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
}
.collab-editor-view__audit-toggle:hover { background: color-mix(in srgb, var(--td-brand-color) 12%, transparent); }
.collab-editor-view__audit-wrap {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed var(--td-component-stroke);
}
.collab-editor-view__share-toggle {
  margin-top: 8px;
  padding: 6px 12px;
  background: transparent;
  color: var(--td-brand-color);
  border: 1px solid color-mix(in srgb, var(--td-brand-color) 55%, var(--app-border));
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
}
.collab-editor-view__share-toggle:hover { background: color-mix(in srgb, var(--td-brand-color) 12%, transparent); }
.collab-editor-view__share-wrap {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed var(--td-component-stroke);
}
@media (max-width: 860px) {
  .collab-editor-view__sidebar { width: 190px; flex-basis: 190px; padding: 14px 12px; }
  .collab-editor-view__topbar { padding:0 12px; }
  .collab-editor-view__presence { display:none; }
}
</style>
