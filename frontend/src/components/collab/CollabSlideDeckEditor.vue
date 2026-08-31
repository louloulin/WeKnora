<!--
  CollabSlideDeckEditor — v0.7.37 Build #44 / v0.7.38 Build #46.x.

  Drives the higher-level Slides backend (deck + slides, theme, layout,
  auto-generate from doc markdown). Distinct from CollabSlideKonvaEditor
  which uses pptx-engine to render .pptx bytes; this editor talks to
  the deck/slides REST surface to produce structured slides.
-->
<template>
  <div class="collab-slide-deck">
    <header class="collab-slide-deck__header">
      <h2>演示文稿</h2>
      <p class="collab-slide-deck__hint">从文档 Markdown 自动生成 / 手工编辑 / 导出 Markdown / JSON / HTML</p>
      <div class="collab-slide-deck__actions">
        <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--primary" @click="onCreate" :disabled="creating">+ 新建 deck</button>
        <button type="button" class="collab-slide-deck__btn" @click="onAutoGenerate" :disabled="!hasSourceDoc">📄 从文档生成</button>
        <select v-model="theme" class="collab-slide-deck__theme" title="主题">
          <option v-for="t in themes" :key="t" :value="t">{{ t }}</option>
        </select>
      </div>
    </header>

    <section v-if="creating" class="collab-slide-deck__composer">
      <h3>新建演示文稿</h3>
      <label>标题 <input v-model="newTitle" placeholder="如: 2026 Q3 路线图" /></label>
      <div class="collab-slide-deck__composer-row">
        <label class="collab-slide-deck__composer-flex">来源 doc_id(可选)
          <input v-model="newSourceDoc" placeholder="collab-doc-uuid" />
        </label>
        <label class="collab-slide-deck__composer-flex">主题
          <select v-model="theme"><option v-for="t in themes" :key="t" :value="t">{{ t }}</option></select>
        </label>
      </div>
      <div class="collab-slide-deck__composer-actions">
        <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--primary" @click="submitCreate">创建</button>
        <button type="button" class="collab-slide-deck__btn" @click="creating = false">取消</button>
      </div>
    </section>

    <section v-if="loading" class="collab-slide-deck__loading">加载中…</section>
    <section v-else-if="error" class="collab-slide-deck__error">{{ error }}</section>

    <section v-else class="collab-slide-deck__list">
      <p v-if="decks.length === 0" class="collab-slide-deck__empty">
        还没有演示文稿。点击「+ 新建 deck」或「📄 从文档生成」开始。
      </p>
      <article v-for="d in decks" :key="d.id" class="collab-slide-deck__deck" :class="{ selected: d.id === selectedDeckID }" @click="selectDeck(d.id)">
        <header>
          <h4>{{ d.title }}</h4>
          <span class="collab-slide-deck__meta">{{ d.theme }} · {{ d.slide_count }} 页 · {{ formatTime(d.updated_at) }}</span>
        </header>
        <div class="collab-slide-deck__deck-actions" @click.stop>
          <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--ghost" @click="exportDeck(d.id, 'markdown')">导出 MD</button>
          <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--ghost" @click="exportDeck(d.id, 'json')">JSON</button>
          <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--danger" @click="deleteDeck(d.id)">删除</button>
        </div>
      </article>
    </section>

    <section v-if="selectedDeck" class="collab-slide-deck__detail">
      <header class="collab-slide-deck__detail-header">
        <h3>{{ selectedDeck.title }} 的幻灯片</h3>
        <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--primary" @click="onAddSlide">+ 新增幻灯片</button>
      </header>

      <ol class="collab-slide-deck__slide-list">
        <li v-for="(s, idx) in slides" :key="s.id" class="collab-slide-deck__slide">
          <span class="collab-slide-deck__slide-num">{{ idx + 1 }}</span>
          <div class="collab-slide-deck__slide-fields">
            <input v-model="s.title" class="collab-slide-deck__slide-title" @blur="commitSlide(s)" />
            <select v-model="s.layout" @change="commitSlide(s)" class="collab-slide-deck__slide-layout">
              <option v-for="l in layouts" :key="l" :value="l">{{ layoutLabel(l) }}</option>
            </select>
            <textarea v-model="s.body_md" rows="2" class="collab-slide-deck__slide-body" @blur="commitSlide(s)" placeholder="正文 Markdown…" />
            <input v-model="s.bulletsText" class="collab-slide-deck__slide-bullets" @blur="onBulletsBlur(s)" placeholder="要点(逗号或换行分隔)" />
          </div>
          <div class="collab-slide-deck__slide-actions" @click.stop>
            <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--ghost" @click="moveSlide(idx, -1)" :disabled="idx === 0">↑</button>
            <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--ghost" @click="moveSlide(idx, +1)" :disabled="idx === slides.length - 1">↓</button>
            <button type="button" class="collab-slide-deck__btn collab-slide-deck__btn--danger" @click="deleteSlide(s.id)">删除</button>
          </div>
        </li>
      </ol>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listSlideDecks,
  createSlideDeck,
  deleteSlideDeck,
  autoGenerateSlides,
  listSlides,
  createSlide,
  updateSlide,
  deleteSlide,
  exportSlides,
  type SlideDeck,
  type Slide,
  type SlideLayout,
  type SlideTheme,
} from '@/api/slides'

const themes: SlideTheme[] = ['notion', 'confluence', 'coda', 'lark', 'apple', 'google', 'academic', 'dark']
const layouts: SlideLayout[] = ['title', 'section', 'bullet', 'two_col', 'image', 'quote', 'end']

const layoutLabel = (l: SlideLayout) => ({
  title: '标题', section: '节扉', bullet: '要点', two_col: '双列', image: '图片', quote: '引文', end: '结尾',
}[l] || l)

const decks = ref<SlideDeck[]>([])
const slides = ref<Array<Slide & { bulletsText: string }>>([])
const loading = ref(true)
const error = ref<string | null>(null)
const theme = ref<SlideTheme>('notion')
const selectedDeckID = ref<string | null>(null)
const creating = ref(false)
const newTitle = ref('')
const newSourceDoc = ref('')

const selectedDeck = computed(() => decks.value.find((d) => d.id === selectedDeckID.value) || null)
const hasSourceDoc = computed(() => newSourceDoc.value.trim().length > 0)

const formatTime = (iso: string) => {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

async function refresh() {
  loading.value = true
  error.value = null
  try {
    const r = await listSlideDecks()
    decks.value = r.items || []
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

async function selectDeck(id: string) {
  selectedDeckID.value = id
  await refreshSlides()
}

async function refreshSlides() {
  if (!selectedDeckID.value) {
    slides.value = []
    return
  }
  try {
    const r = await listSlides(selectedDeckID.value)
    slides.value = (r.items || []).map((s) => ({
      ...s,
      bulletsText: (s.bullets || []).join('\n'),
    }))
  } catch (e: any) {
    error.value = e?.message || '加载幻灯片失败'
  }
}

function onCreate() {
  creating.value = true
  newTitle.value = ''
  newSourceDoc.value = ''
}

async function submitCreate() {
  if (!newTitle.value.trim()) {
    MessagePlugin.warning('请填写标题')
    return
  }
  try {
    const deck = await createSlideDeck({
      title: newTitle.value.trim(),
      theme: theme.value,
      source_doc_id: newSourceDoc.value.trim() || undefined,
    })
    creating.value = false
    await refresh()
    selectDeck(deck.id)
    MessagePlugin.success('已创建 deck')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '创建失败')
  }
}

async function onAutoGenerate() {
  if (!hasSourceDoc.value) {
    MessagePlugin.warning('请先填写源 doc_id')
    return
  }
  try {
    const deck = await autoGenerateSlides({
      source_doc_id: newSourceDoc.value.trim(),
      title: newTitle.value.trim() || '自动生成的演示文稿',
      theme: theme.value,
      max_slides: 12,
    })
    await refresh()
    selectDeck(deck.id)
    MessagePlugin.success(`已生成 ${deck.slide_count} 张幻灯片`)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '生成失败')
  }
}

async function deleteDeck(id: string) {
  try {
    await deleteSlideDeck(id)
    if (selectedDeckID.value === id) {
      selectedDeckID.value = null
      slides.value = []
    }
    await refresh()
    MessagePlugin.success('已删除')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

async function exportDeck(id: string, format: 'markdown' | 'json') {
  try {
    const r = await exportSlides(id, format)
    const blob = new Blob([r.content], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `slides-${id}.${format}`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '导出失败')
  }
}

function onAddSlide() {
  if (!selectedDeckID.value) return
  slides.value.push({
    id: '',
    deck_id: selectedDeckID.value,
    index: slides.value.length,
    layout: 'bullet',
    title: '新幻灯片',
    body_md: '',
    bullets: [],
    bulletsText: '',
    image_url: '',
    notes: '',
    created_at: '',
    updated_at: '',
  })
}

async function commitSlide(s: Slide & { bulletsText: string }) {
  if (!s.id) {
    if (!selectedDeckID.value) return
    try {
      const created = await createSlide(selectedDeckID.value, {
        layout: s.layout,
        title: s.title,
        body_md: s.body_md,
        bullets: s.bullets,
      })
      Object.assign(s, created, { bulletsText: s.bulletsText })
      MessagePlugin.success('已添加幻灯片')
    } catch (e: any) {
      MessagePlugin.error(e?.message || '新增失败')
    }
    return
  }
  try {
    await updateSlide(s.deck_id, s.id, {
      layout: s.layout,
      title: s.title,
      body_md: s.body_md,
      bullets: s.bullets,
    })
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}

function onBulletsBlur(s: Slide & { bulletsText: string }) {
  s.bullets = s.bulletsText.split(/[\n,]/).map((x) => x.trim()).filter(Boolean)
  commitSlide(s)
}

async function moveSlide(idx: number, delta: number) {
  const target = idx + delta
  if (target < 0 || target >= slides.value.length) return
  const a = slides.value[idx]
  const b = slides.value[target]
  if (!a.id || !b.id) return
  const ai = a.index
  const bi = b.index
  try {
    await updateSlide(a.deck_id, a.id, { index: bi })
    await updateSlide(b.deck_id, b.id, { index: ai })
    a.index = bi
    b.index = ai
    const arr = slides.value.slice()
    ;[arr[idx], arr[target]] = [arr[target], arr[idx]]
    slides.value = arr
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重排失败')
  }
}

async function deleteSlide(id: string) {
  if (!selectedDeckID.value) return
  try {
    await deleteSlide(selectedDeckID.value, id)
    await refreshSlides()
    MessagePlugin.success('已删除幻灯片')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

watch(selectedDeckID, refreshSlides)
onMounted(refresh)
</script>

<style scoped>
.collab-slide-deck { padding: 16px; max-width: 1100px; margin: 0 auto; }
.collab-slide-deck__header h2 { margin: 0 0 4px; }
.collab-slide-deck__hint { color: #6b7280; font-size: 13px; margin: 0 0 12px; }
.collab-slide-deck__actions { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 16px; }
.collab-slide-deck__theme { padding: 4px 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px; }
.collab-slide-deck__btn { padding: 6px 12px; border: 1px solid #d1d5db; background: #fff; color: #1f2937; border-radius: 4px; cursor: pointer; font-size: 13px; }
.collab-slide-deck__btn:hover:not(:disabled) { background: #f3f4f6; }
.collab-slide-deck__btn:disabled { opacity: 0.5; cursor: not-allowed; }
.collab-slide-deck__btn--primary { background: #2563eb; border-color: #2563eb; color: #fff; }
.collab-slide-deck__btn--primary:hover:not(:disabled) { background: #1d4ed8; }
.collab-slide-deck__btn--ghost { padding: 4px 8px; font-size: 12px; }
.collab-slide-deck__btn--danger { background: #fef2f2; border-color: #ef4444; color: #b91c1c; }
.collab-slide-deck__btn--danger:hover:not(:disabled) { background: #fee2e2; }
.collab-slide-deck__composer { background: #fff; border: 1px solid #e5e7eb; border-radius: 6px; padding: 12px; margin-bottom: 16px; }
.collab-slide-deck__composer label { display: block; margin: 8px 0; font-size: 13px; }
.collab-slide-deck__composer input, .collab-slide-deck__composer select { display: block; width: 100%; padding: 6px 10px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 14px; }
.collab-slide-deck__composer-row { display: flex; gap: 12px; }
.collab-slide-deck__composer-flex { flex: 1; }
.collab-slide-deck__composer-actions { display: flex; gap: 8px; margin-top: 8px; }
.collab-slide-deck__loading, .collab-slide-deck__error { padding: 12px; color: #6b7280; }
.collab-slide-deck__error { color: #b91c1c; }
.collab-slide-deck__empty { color: #6b7280; text-align: center; padding: 32px 0; }
.collab-slide-deck__list { display: flex; flex-direction: column; gap: 8px; }
.collab-slide-deck__deck { background: #fff; border: 1px solid #e5e7eb; border-radius: 6px; padding: 12px; cursor: pointer; transition: border-color 120ms ease; }
.collab-slide-deck__deck:hover { border-color: #93c5fd; }
.collab-slide-deck__deck.selected { border-color: #2563eb; background: #eff6ff; }
.collab-slide-deck__deck header { display: flex; justify-content: space-between; align-items: center; }
.collab-slide-deck__deck h4 { margin: 0; font-size: 14px; }
.collab-slide-deck__meta { color: #6b7280; font-size: 12px; }
.collab-slide-deck__deck-actions { display: flex; gap: 6px; margin-top: 8px; }
.collab-slide-deck__detail { margin-top: 24px; background: #fff; border: 1px solid #e5e7eb; border-radius: 6px; padding: 16px; }
.collab-slide-deck__detail-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.collab-slide-deck__detail-header h3 { margin: 0; }
.collab-slide-deck__slide-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 8px; }
.collab-slide-deck__slide { display: grid; grid-template-columns: 28px 1fr auto; gap: 8px; padding: 8px; background: #f9fafb; border-radius: 4px; align-items: start; }
.collab-slide-deck__slide-num { font-weight: 700; color: #6b7280; padding-top: 6px; }
.collab-slide-deck__slide-fields { display: flex; flex-direction: column; gap: 4px; }
.collab-slide-deck__slide-title { padding: 4px 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 14px; font-weight: 600; }
.collab-slide-deck__slide-layout { padding: 2px 6px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 12px; align-self: flex-start; }
.collab-slide-deck__slide-body { padding: 4px 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px; resize: vertical; }
.collab-slide-deck__slide-bullets { padding: 4px 8px; border: 1px solid #d1d5db; border-radius: 4px; font-size: 13px; }
.collab-slide-deck__slide-actions { display: flex; gap: 4px; align-items: center; }
</style>
