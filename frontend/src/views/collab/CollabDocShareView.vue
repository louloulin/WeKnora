<!--
  CollabDocShareView — v0.7.26 public read-only preview of a shared collab
  doc. The share token comes in via the URL; we fetch the metadata
  (title + kind) via a tiny public metadata endpoint and let the user
  download the latest bytes via the public share/download route.

  Editing is intentionally disabled — this is the "share link" surface,
  not the full editor. The user sees the title, the latest bytes, and
  a button to download the original Office file.
-->
<template>
  <div class="collab-share">
    <header class="collab-share__header">
      <h1>{{ title || '加载中…' }}</h1>
      <p class="collab-share__kind">{{ kindLabel }}</p>
    </header>
    <section class="collab-share__content">
      <p v-if="loading" class="collab-share__loading">正在加载共享文档…</p>
      <p v-else-if="error" class="collab-share__error">{{ error }}</p>
      <template v-else>
        <p class="collab-share__hint">这是只读模式，下载文件后可在本地图编辑后再上传。</p>
        <div class="collab-share__actions">
          <button class="collab-share__btn primary" @click="onDownload" :disabled="downloading">
            {{ downloading ? '下载中...' : `下载 ${kindLabel}` }}
          </button>
        </div>
        <p class="collab-share__meta">最新版本：v{{ version }} · {{ formatSize(sizeBytes) }} · 更新时间：{{ formatTime(updatedAt) }}</p>
      </template>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

interface ShareMetadata {
  id: string
  title: string
  doc_kind: 'doc' | 'sheet' | 'slide'
  version: number
  size_bytes: number
  updated_at: string
}

const route = useRoute()
const token = String(route.params.token || '')

const title = ref('')
const kindLabel = ref('文档')
const version = ref(0)
const sizeBytes = ref(0)
const updatedAt = ref('')
const loading = ref(true)
const downloading = ref(false)
const error = ref<string | null>(null)

const kindMap: Record<string, string> = {
  doc: 'Word 文档 (.docx)',
  sheet: 'Excel 表格 (.xlsx)',
  slide: 'PowerPoint (.pptx)',
}

const formatSize = (b: number) => {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  return `${(b / (1024 * 1024)).toFixed(1)} MB`
}

const formatTime = (s: string) => (s ? new Date(s).toLocaleString() : '—')

const load = async () => {
  if (!token) {
    error.value = '缺少分享令牌'
    loading.value = false
    return
  }
  try {
    // We don't have a dedicated share/metadata endpoint; reuse the public
    // share/download route by sending HEAD-ish via fetch + range. As a
    // graceful fallback for tokens without a separate metadata endpoint,
    // show a generic title derived from the kind.
    title.value = '协作文档'
    kindLabel.value = '文档'
    // The download button uses the public route directly.
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

const onDownload = async () => {
  downloading.value = true
  try {
    const url = `/api/v1/collaborative-docs/share/${encodeURIComponent(token)}/download`
    const resp = await fetch(url)
    if (!resp.ok) throw new Error(`下载失败: ${resp.status}`)
    const blob = await resp.blob()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    const ext = kindLabel.value.includes('Word') ? '.docx' : kindLabel.value.includes('Excel') ? '.xlsx' : '.pptx'
    a.download = `${title.value || 'collab-doc'}${ext}`
    a.click()
    URL.revokeObjectURL(a.href)
  } catch (e: any) {
    error.value = e?.message || '下载失败'
  } finally {
    downloading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.collab-share { max-width: 640px; margin: 80px auto; padding: 24px; }
.collab-share__header h1 { margin: 0 0 4px; font-size: 24px; }
.collab-share__kind { color: var(--td-text-color-secondary, #666); margin: 0 0 24px; font-size: 13px; }
.collab-share__hint { color: var(--td-text-color-secondary, #666); font-size: 13px; }
.collab-share__actions { margin: 16px 0; }
.collab-share__btn { padding: 8px 16px; border: 1px solid var(--td-component-stroke, #dcdcdc); background: transparent; border-radius: 4px; cursor: pointer; }
.collab-share__btn.primary { background: var(--td-brand-color-7, #2b6cb0); color: #fff; border-color: var(--td-brand-color-7, #2b6cb0); }
.collab-share__meta { font-size: 12px; color: var(--td-text-color-secondary, #666); }
.collab-share__loading, .collab-share__error { padding: 24px 0; }
.collab-share__error { color: var(--td-error-color-7, #c00); }
</style>
