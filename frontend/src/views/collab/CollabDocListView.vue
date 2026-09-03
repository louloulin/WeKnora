<!--
  CollabDocListView — list + create collaborative docs.
  Mirrors the Feishu / Tencent document list UX: kind filter, owner
  indicator, presence dot, primary "新建文档" CTA.
-->
<template>
  <div class="collab-doc-list">
    <header class="collab-doc-list__header">
      <div>
        <div class="collab-doc-list__eyebrow">WORKSPACE / DOCUMENTS</div>
        <h2>协作文档</h2>
        <p class="collab-doc-list__sub">在一个工作区里创建、编辑并分享文档、表格、幻灯片和收集表。</p>
      </div>
      <div class="collab-doc-list__header-meta"><span class="collab-doc-list__status-dot"></span>实时协作已就绪</div>
    </header>
    <section class="collab-doc-list__create">
      <div class="collab-doc-list__create-heading"><span class="collab-doc-list__create-icon">＋</span><div><strong>新建内容</strong><small>从空白文件开始</small></div></div>
      <input v-model="newTitle" placeholder="输入文档标题" class="collab-doc-list__title-input" />
      <select v-model="newKind" class="collab-doc-list__kind-select" aria-label="内容类型">
        <option value="doc">文档</option><option value="sheet">表格</option><option value="slide">幻灯片</option><option value="form">收集表</option>
      </select>
      <button class="collab-doc-list__create-btn" :disabled="creating" @click="onCreate">{{ creating ? '创建中…' : '新建文档' }}</button>
    </section>
    <section class="collab-doc-list__toolbar">
      <div class="collab-doc-list__filters"><button v-for="k in kinds" :key="k.value" :class="{ active: filter.kind === k.value }" @click="filter.kind = k.value; reload()">{{ k.label }}</button></div>
      <label class="collab-doc-list__search"><span>⌕</span><input v-model="search" placeholder="搜索文档" aria-label="搜索文档" /></label>
    </section>
    <section class="collab-doc-list__table">
      <div class="collab-doc-list__table-head"><strong>我的文档</strong><span>{{ filteredItems.length }} 个文件</span></div>
      <table>
        <thead><tr><th>名称</th><th>类型</th><th>访问权限</th><th>最近编辑</th><th aria-label="操作"></th></tr></thead>
        <tbody>
          <tr v-for="d in filteredItems" :key="d.id">
            <td><router-link :to="{ name: 'collabDocEditor', params: { id: d.id } }"><span class="collab-doc-list__file-icon" :data-kind="d.doc_kind">{{ d.doc_kind === 'sheet' ? '▦' : d.doc_kind === 'slide' ? '▧' : d.doc_kind === 'form' ? '☷' : '▤' }}</span><span><b>{{ d.title }}</b><small>{{ d.id.slice(0, 8) }}</small></span></router-link></td>
            <td><span class="collab-doc-list__kind" :data-kind="d.doc_kind">{{ kindLabel(d.doc_kind) }}</span></td>
            <td><span class="collab-doc-list__visibility">● {{ d.visibility }}</span></td>
            <td>{{ formatTime(d.updated_at) }}</td>
            <td class="collab-doc-list__actions"><button class="collab-doc-list__share" @click="onShare(d.id, d.share_token)">分享</button><button class="collab-doc-list__del" @click="onDelete(d.id)" aria-label="删除文档">删除</button></td>
          </tr>
          <tr v-if="filteredItems.length === 0"><td colspan="5" class="collab-doc-list__empty"><span class="collab-doc-list__empty-icon">□</span><strong>{{ search ? '没有找到匹配的文档' : '还没有协作文档' }}</strong><small>{{ search ? '换个关键词再试试' : '创建一个文档，开始多人实时协作' }}</small></td></tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { createCollabDoc, deleteCollabDoc, listCollabDocs, type CollabDoc, type CollabDocKind } from '@/api/collabDoc'
import { listKnowledgeBases, createKnowledgeBase } from '@/api/knowledge-base'
import { MessagePlugin } from 'tdesign-vue-next'

const router = useRouter()
const items = ref<CollabDoc[]>([])
const newTitle = ref('')
const newKind = ref<CollabDocKind>('doc')
const creating = ref(false)
const search = ref('')

const kinds = [
  { value: '' as CollabDocKind | '', label: '全部' },
  { value: 'doc' as CollabDocKind, label: '文档' },
  { value: 'sheet' as CollabDocKind, label: '表格' },
  { value: 'slide' as CollabDocKind, label: '幻灯片' },
  { value: 'form' as CollabDocKind, label: '收集表' },
]

const filter = reactive<{ kind: CollabDocKind | '' }>({ kind: '' })
const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return items.value
  return items.value.filter((item) => item.title.toLowerCase().includes(query))
})

const reload = async () => {
  try {
    const r = await listCollabDocs({ doc_kind: (filter.kind || undefined) as CollabDocKind | undefined })
    items.value = r.items
  } catch (e: any) {
    MessagePlugin.error(`加载失败：${e?.message || e}`)
  }
}

const resolveDefaultKBId = async (): Promise<string> => {
  // Tencent-Document style "blank from any workspace": pick the tenant's
  // first available KB; if none exists, auto-create one named "工作区".
  // No internal UUID ever surfaces to the user.
  try {
    const res: any = await listKnowledgeBases()
    const items = Array.isArray(res?.data) ? res.data : []
    if (items.length > 0 && items[0]?.id) return String(items[0].id)
  } catch (_) {
    // fall through to auto-create below
  }
  try {
    const res: any = await createKnowledgeBase({ name: '工作区', description: '默认协作文档空间', type: 'document' })
    if (res?.data?.id) return String(res.data.id)
  } catch (_) {
    // surfaced by createCollabDoc below
  }
  return ''
}

const onCreate = async () => {
  if (!newTitle.value.trim()) {
    MessagePlugin.warning('请填写文档标题')
    return
  }
  creating.value = true
  try {
    const kbId = await resolveDefaultKBId()
    if (!kbId) {
      MessagePlugin.error('当前租户暂无可用知识库，请先在「知识库」页创建一个知识库再试。')
      return
    }
    const d = await createCollabDoc({ kb_id: kbId, title: newTitle.value.trim(), doc_kind: newKind.value })
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
.collab-doc-list { min-height: 100%; box-sizing: border-box; padding: 38px clamp(20px, 5vw, 72px) 56px; background: var(--app-page-bg); color: var(--app-text); }
.collab-doc-list__header, .collab-doc-list__create, .collab-doc-list__toolbar, .collab-doc-list__table { max-width: 1180px; margin-left: auto; margin-right: auto; }
.collab-doc-list__header { display:flex; align-items:flex-end; justify-content:space-between; gap:24px; margin-bottom:28px; }
.collab-doc-list__eyebrow { color: var(--td-brand-color); font-size: 10px; font-weight:700; letter-spacing:.16em; margin-bottom:10px; }
.collab-doc-list__header h2 { margin:0 0 8px; font-size:28px; letter-spacing:-.03em; }
.collab-doc-list__sub { margin:0; color:var(--app-text-muted); font-size:13px; }
.collab-doc-list__header-meta { display:flex; align-items:center; gap:8px; color:var(--app-text-muted); font-size:12px; white-space:nowrap; }
.collab-doc-list__status-dot { width:7px; height:7px; border-radius:50%; background:var(--td-brand-color); box-shadow:0 0 0 4px color-mix(in srgb, var(--td-brand-color) 15%, transparent); }
.collab-doc-list__create { display:flex; align-items:center; gap:10px; padding:14px; box-sizing:border-box; background:linear-gradient(100deg, color-mix(in srgb, var(--td-brand-color) 8%, var(--app-surface-raised)), var(--app-surface-raised)); border:1px solid var(--app-border); border-radius:12px; box-shadow:0 12px 30px rgba(0,0,0,.16); }
.collab-doc-list__create-heading { display:flex; align-items:center; gap:9px; min-width:135px; padding:0 10px 0 2px; }
.collab-doc-list__create-heading strong, .collab-doc-list__create-heading small { display:block; }
.collab-doc-list__create-heading strong { font-size:13px; }
.collab-doc-list__create-heading small { color:var(--app-text-muted); font-size:11px; margin-top:3px; }
.collab-doc-list__create-icon { width:28px; height:28px; display:grid; place-items:center; border-radius:8px; color:var(--td-text-color-anti); background:var(--td-brand-color); font-size:20px; line-height:1; }
.collab-doc-list__title-input, .collab-doc-list__kind-select { min-height:38px; box-sizing:border-box; padding:8px 11px; border:1px solid var(--app-border-strong); border-radius:7px; background:var(--app-control-bg); color:var(--app-text); outline:none; }
.collab-doc-list__title-input { flex:2; min-width:160px; } .collab-doc-list__kind-select { flex:1; min-width:130px; }
.collab-doc-list__title-input:focus, .collab-doc-list__kind-select:focus, .collab-doc-list__search:focus-within { border-color:var(--td-brand-color); box-shadow:0 0 0 3px color-mix(in srgb, var(--td-brand-color) 14%, transparent); }
.collab-doc-list__create-btn { min-height:38px; padding:0 16px; background:var(--td-brand-color); color:var(--td-text-color-anti); border:1px solid var(--td-brand-color); border-radius:7px; cursor:pointer; font-weight:600; white-space:nowrap; }
.collab-doc-list__create-btn:hover { background:var(--td-brand-color-active); } .collab-doc-list__create-btn:disabled { opacity:.55; cursor:not-allowed; }
.collab-doc-list__toolbar { display:flex; align-items:center; justify-content:space-between; gap:16px; margin-top:28px; margin-bottom:12px; }
.collab-doc-list__filters { display:flex; gap:4px; } .collab-doc-list__filters button { padding:7px 13px; border:1px solid transparent; border-radius:6px; background:transparent; color:var(--app-text-muted); cursor:pointer; font-size:13px; } .collab-doc-list__filters button:hover { background:var(--app-surface-raised); color:var(--app-text); } .collab-doc-list__filters button.active { background:color-mix(in srgb, var(--td-brand-color) 14%, var(--app-surface-raised)); color:var(--td-brand-color); border-color:color-mix(in srgb, var(--td-brand-color) 35%, var(--app-border)); }
.collab-doc-list__search { display:flex; align-items:center; gap:7px; min-width:190px; padding:0 10px; border:1px solid var(--app-border); border-radius:7px; background:var(--app-surface-raised); color:var(--app-text-muted); } .collab-doc-list__search input { width:100%; min-height:32px; border:0; outline:0; background:transparent; color:var(--app-text); font-size:12px; }
.collab-doc-list__table { overflow:hidden; background:var(--app-surface-bg); border:1px solid var(--app-border); border-radius:12px; box-shadow:0 10px 28px rgba(0,0,0,.12); } .collab-doc-list__table-head { display:flex; align-items:center; justify-content:space-between; padding:18px 20px 14px; } .collab-doc-list__table-head strong { font-size:15px; } .collab-doc-list__table-head span { color:var(--app-text-muted); font-size:12px; }
.collab-doc-list__table table { width:100%; border-collapse:collapse; } .collab-doc-list__table th { padding:10px 20px; background:var(--app-surface-raised); color:var(--app-text-muted); text-align:left; font-size:11px; font-weight:600; } .collab-doc-list__table td { padding:13px 20px; border-top:1px solid var(--app-border); color:var(--app-text-muted); font-size:12px; } .collab-doc-list__table tr:hover td { background:color-mix(in srgb, var(--td-brand-color) 4%, var(--app-surface-bg)); }
.collab-doc-list__table a { display:flex; align-items:center; gap:10px; color:var(--app-text); text-decoration:none; } .collab-doc-list__table a:hover b { color:var(--td-brand-color); } .collab-doc-list__table a b, .collab-doc-list__table a small { display:block; } .collab-doc-list__table a b { font-size:13px; font-weight:600; } .collab-doc-list__table a small { color:var(--app-text-muted); font-size:10px; margin-top:3px; opacity:.7; }
.collab-doc-list__file-icon { width:28px; height:32px; display:grid; place-items:center; border-radius:6px; background:color-mix(in srgb, var(--td-brand-color) 14%, var(--app-surface-raised)); color:var(--td-brand-color); font-size:17px; } .collab-doc-list__file-icon[data-kind=sheet] { color:#46a6ff; background:rgba(70,166,255,.12); } .collab-doc-list__file-icon[data-kind=slide] { color:#f7a94a; background:rgba(247,169,74,.12); } .collab-doc-list__file-icon[data-kind=form] { color:#b78cff; background:rgba(183,140,255,.12); }
.collab-doc-list__kind { padding:4px 8px; border-radius:5px; background:var(--app-surface-raised); color:var(--app-text-muted); } .collab-doc-list__visibility { color:var(--app-text-muted); } .collab-doc-list__visibility::first-letter { color:var(--td-brand-color); } .collab-doc-list__actions { white-space:nowrap; text-align:right; } .collab-doc-list__share, .collab-doc-list__del { padding:5px 9px; margin-left:6px; border-radius:5px; cursor:pointer; font-size:11px; } .collab-doc-list__share { background:transparent; color:var(--td-brand-color); border:1px solid color-mix(in srgb, var(--td-brand-color) 45%, var(--app-border)); } .collab-doc-list__share:hover { background:color-mix(in srgb, var(--td-brand-color) 12%, transparent); } .collab-doc-list__del { background:transparent; color:var(--td-error-color-7); border:1px solid transparent; } .collab-doc-list__del:hover { border-color:color-mix(in srgb, var(--td-error-color) 40%, var(--app-border)); }
.collab-doc-list__empty { text-align:center; padding:64px 24px !important; } .collab-doc-list__empty-icon, .collab-doc-list__empty strong, .collab-doc-list__empty small { display:block; } .collab-doc-list__empty-icon { color:var(--app-text-muted); font-size:30px; margin-bottom:10px; } .collab-doc-list__empty strong { color:var(--app-text); font-size:14px; } .collab-doc-list__empty small { margin-top:6px; color:var(--app-text-muted); }
@media (max-width:760px) { .collab-doc-list { padding:24px 14px 40px; } .collab-doc-list__header, .collab-doc-list__toolbar { align-items:flex-start; flex-direction:column; } .collab-doc-list__header-meta { display:none; } .collab-doc-list__create { flex-wrap:wrap; } .collab-doc-list__create-heading, .collab-doc-list__title-input, .collab-doc-list__kind-select, .collab-doc-list__create-btn { flex:1 1 100%; } .collab-doc-list__table { overflow-x:auto; } .collab-doc-list__table table { min-width:700px; } }
</style>
