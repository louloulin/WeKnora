<!--
  CollabDocListView — list + create collaborative docs.
  Mirrors the Feishu / Tencent document list UX: kind filter, owner
  indicator, presence dot, primary "新建文档" CTA.
-->
<template>
  <div class="collab-doc-list">
    <header class="collab-doc-list__header">
      <h2>协作文档</h2>
      <p class="collab-doc-list__sub">类似飞书文档 / 腾讯文档：DOC、SHEET、SLIDE、FORM 四类多人实时协作。</p>
    </header>
    <section class="collab-doc-list__create">
      <input v-model="newTitle" placeholder="新文档标题" class="collab-doc-list__title-input" />
      <select v-model="newKind" class="collab-doc-list__kind-select">
        <option value="doc">文档 (DOC)</option>
        <option value="sheet">表格 (SHEET)</option>
        <option value="slide">幻灯片 (SLIDE)</option>
        <option value="form">收集表 (FORM)</option>
      </select>
      <input v-model="kbId" placeholder="知识库 ID" class="collab-doc-list__kb-input" />
      <button class="collab-doc-list__create-btn" :disabled="creating" @click="onCreate">新建</button>
    </section>
    <section class="collab-doc-list__filters">
      <button v-for="k in kinds" :key="k.value" :class="{ active: filter.kind === k.value }" @click="filter.kind = k.value; reload()">{{ k.label }}</button>
    </section>
    <section class="collab-doc-list__table">
      <table>
        <thead>
          <tr>
            <th>标题</th>
            <th>类型</th>
            <th>可见性</th>
            <th>更新时间</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="d in items" :key="d.id">
            <td><router-link :to="{ name: 'collabDocEditor', params: { id: d.id } }">{{ d.title }}</router-link></td>
            <td>{{ kindLabel(d.doc_kind) }}</td>
            <td>{{ d.visibility }}</td>
            <td>{{ formatTime(d.updated_at) }}</td>
            <td>
              <button class="collab-doc-list__share" @click="onShare(d.id, d.share_token)">分享</button>
              <button class="collab-doc-list__del" @click="onDelete(d.id)">删除</button>
            </td>
          </tr>
          <tr v-if="items.length === 0">
            <td colspan="5" class="collab-doc-list__empty">尚无文档，点击"新建"创建第一个协作文档。</td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { createCollabDoc, deleteCollabDoc, listCollabDocs, type CollabDoc, type CollabDocKind } from '@/api/collabDoc'
import { MessagePlugin } from 'tdesign-vue-next'

const router = useRouter()
const items = ref<CollabDoc[]>([])
const newTitle = ref('')
const newKind = ref<CollabDocKind>('doc')
const kbId = ref('')
const creating = ref(false)

const kinds = [
  { value: '' as CollabDocKind | '', label: '全部' },
  { value: 'doc' as CollabDocKind, label: '文档' },
  { value: 'sheet' as CollabDocKind, label: '表格' },
  { value: 'slide' as CollabDocKind, label: '幻灯片' },
  { value: 'form' as CollabDocKind, label: '收集表' },
]

const filter = reactive<{ kind: CollabDocKind | '' }>({ kind: '' })

const reload = async () => {
  try {
    const r = await listCollabDocs({ doc_kind: (filter.kind || undefined) as CollabDocKind | undefined })
    items.value = r.items
  } catch (e: any) {
    MessagePlugin.error(`加载失败：${e?.message || e}`)
  }
}

const onCreate = async () => {
  if (!newTitle.value || !kbId.value) {
    MessagePlugin.warning('请填写标题与知识库 ID')
    return
  }
  creating.value = true
  try {
    const d = await createCollabDoc({ kb_id: kbId.value, title: newTitle.value, doc_kind: newKind.value })
    router.push({ name: 'collabDocEditor', params: { id: d.id } })
  } catch (e: any) {
    MessagePlugin.error(`创建失败：${e?.message || e}`)
  } finally {
    creating.value = false
  }
}

const onShare = async (id: string, token: string) => {
  if (!token) {
    MessagePlugin.warning('该文档尚未生成分享令牌，请先保存后再分享')
    return
  }
  const url = `${window.location.origin}/collab-documents/share/${encodeURIComponent(token)}`
  try {
    await navigator.clipboard.writeText(url)
    MessagePlugin.success('分享链接已复制到剪贴板')
  } catch (e) {
    // Fallback for browsers without clipboard API: show the link.
    prompt('分享链接：', url)
  }
}

const onDelete = async (id: string) => {
  if (!confirm('确定删除该协作文档？该操作不可撤销。')) return
  try {
    await deleteCollabDoc(id)
    await reload()
  } catch (e: any) {
    MessagePlugin.error(`删除失败：${e?.message || e}`)
  }
}

const kindLabel = (k: CollabDocKind) => ({ doc: '文档', sheet: '表格', slide: '幻灯片', form: '收集表' }[k] || k)
const formatTime = (s: string) => new Date(s).toLocaleString()

onMounted(reload)
</script>

<style scoped>
.collab-doc-list { min-height: 100%; box-sizing: border-box; padding: 32px 36px; background: var(--app-page-bg); color: var(--app-text); }
.collab-doc-list__header { max-width: 1180px; margin: 0 auto 24px; }
.collab-doc-list__header h2 { margin: 0 0 8px; font-size: 24px; letter-spacing: -0.02em; }
.collab-doc-list__sub { margin: 0; color: var(--app-text-muted); font-size: 13px; }
.collab-doc-list__create { max-width: 1180px; box-sizing: border-box; display: flex; gap: 10px; margin: 0 auto 18px; padding: 16px; background: var(--app-surface-raised); border: 1px solid var(--app-border); border-radius: 10px; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12); }
.collab-doc-list__title-input { flex: 2; min-width: 180px; }
.collab-doc-list__kind-select, .collab-doc-list__kb-input, .collab-doc-list__title-input { box-sizing: border-box; min-height: 36px; padding: 8px 11px; border: 1px solid var(--app-border-strong); border-radius: 6px; background: var(--app-control-bg); color: var(--app-text); }
.collab-doc-list__kind-select, .collab-doc-list__kb-input { flex: 1; min-width: 150px; }
.collab-doc-list__create-btn { min-width: 72px; padding: 0 16px; background: var(--td-brand-color-6); color: var(--td-text-color-anti); border: 1px solid var(--td-brand-color-6); border-radius: 6px; cursor: pointer; font-weight: 600; }
.collab-doc-list__create-btn:hover { background: var(--td-brand-color-5); }
.collab-doc-list__create-btn:disabled { opacity: .55; cursor: not-allowed; }
.collab-doc-list__filters { max-width: 1180px; display: flex; gap: 6px; margin: 0 auto 14px; }
.collab-doc-list__filters button { padding: 7px 14px; background: transparent; color: var(--app-text-muted); border: 1px solid transparent; border-radius: 6px; cursor: pointer; font-size: 13px; }
.collab-doc-list__filters button:hover { color: var(--app-text); background: var(--app-surface-raised); }
.collab-doc-list__filters button.active { background: color-mix(in srgb, var(--td-brand-color) 16%, var(--app-surface-raised)); border-color: color-mix(in srgb, var(--td-brand-color) 45%, var(--app-border)); color: var(--td-brand-color); }
.collab-doc-list__table { max-width: 1180px; margin: 0 auto; background: var(--app-surface-bg); border: 1px solid var(--app-border); border-radius: 10px; overflow: hidden; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.1); }
.collab-doc-list__table table { width: 100%; border-collapse: collapse; }
.collab-doc-list__table th { padding: 12px 16px; text-align: left; background: var(--app-surface-raised); color: var(--app-text-muted); font-size: 12px; font-weight: 600; }
.collab-doc-list__table td { padding: 14px 16px; text-align: left; border-top: 1px solid var(--app-border); color: var(--app-text-muted); font-size: 13px; }
.collab-doc-list__table tr:hover td { background: color-mix(in srgb, var(--td-brand-color) 4%, var(--app-surface-bg)); }
.collab-doc-list__table a { color: var(--app-text); font-weight: 600; text-decoration: none; }
.collab-doc-list__table a:hover { color: var(--td-brand-color); }
.collab-doc-list__share, .collab-doc-list__del { padding: 6px 10px; margin-right: 6px; border-radius: 5px; cursor: pointer; font-size: 12px; }
.collab-doc-list__share { background: transparent; color: var(--td-brand-color); border: 1px solid color-mix(in srgb, var(--td-brand-color) 55%, var(--app-border)); }
.collab-doc-list__share:hover { background: color-mix(in srgb, var(--td-brand-color) 12%, transparent); }
.collab-doc-list__del { background: transparent; color: var(--td-error-color-7); border: 1px solid color-mix(in srgb, var(--td-error-color) 45%, var(--app-border)); }
.collab-doc-list__del:hover { background: color-mix(in srgb, var(--td-error-color) 12%, transparent); }
.collab-doc-list__empty { text-align: center; padding: 52px 24px !important; color: var(--app-text-muted) !important; }
@media (max-width: 760px) {
  .collab-doc-list { padding: 20px 14px; }
  .collab-doc-list__create { flex-wrap: wrap; }
  .collab-doc-list__title-input, .collab-doc-list__kind-select, .collab-doc-list__kb-input { flex: 1 1 100%; }
  .collab-doc-list__create-btn { min-height: 36px; }
  .collab-doc-list__table { overflow-x: auto; }
  .collab-doc-list__table table { min-width: 680px; }
}
</style>
