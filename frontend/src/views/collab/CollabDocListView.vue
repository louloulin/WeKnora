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
.collab-doc-list { padding: 24px; }
.collab-doc-list__header h2 { margin: 0 0 4px; font-size: 22px; }
.collab-doc-list__sub { margin: 0 0 24px; color: var(--td-text-color-secondary); font-size: 13px; }
.collab-doc-list__create { display: flex; gap: 8px; margin-bottom: 16px; }
.collab-doc-list__title-input { flex: 2; padding: 6px 10px; border: 1px solid var(--td-component-stroke); border-radius: 4px; }
.collab-doc-list__kind-select, .collab-doc-list__kb-input { padding: 6px 10px; border: 1px solid var(--td-component-stroke); border-radius: 4px; }
.collab-doc-list__create-btn { padding: 6px 16px; background: var(--td-brand-color-7); color: white; border: none; border-radius: 4px; cursor: pointer; }
.collab-doc-list__filters { display: flex; gap: 4px; margin-bottom: 12px; }
.collab-doc-list__filters button { padding: 4px 12px; background: var(--td-bg-color-secondarycontainer); border: 1px solid var(--td-component-stroke); border-radius: 999px; cursor: pointer; font-size: 12px; }
.collab-doc-list__filters button.active { background: var(--td-brand-color-1); border-color: var(--td-brand-color-7); color: var(--td-brand-color-7); }
.collab-doc-list__table { background: var(--td-bg-color-container); border: 1px solid var(--td-component-stroke); border-radius: 6px; overflow: hidden; }
.collab-doc-list__table table { width: 100%; border-collapse: collapse; }
.collab-doc-list__table th, .collab-doc-list__table td { padding: 10px 16px; text-align: left; border-bottom: 1px solid var(--td-component-stroke); }
.collab-doc-list__del { padding: 4px 10px; background: var(--td-error-color-1); color: var(--td-error-color-7); border: 1px solid var(--td-error-color-3); border-radius: 4px; cursor: pointer; font-size: 12px; }
.collab-doc-list__empty { text-align: center; padding: 24px; color: var(--td-text-color-secondary); }
</style>
