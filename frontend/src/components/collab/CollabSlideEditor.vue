<!--
  CollabSlideEditor — v0.7.26 SLIDE-kind collaborative editor with real
  .pptx byte round-trip.

  Architecture:
   1. Open: GET /collaborative-docs/:id/download (latest .pptx bytes)
            -> pptxAdapter.openPptx(bytes) seeds a structured deck
            ({ title, bullets } per slide).
   2. Realtime: Y.Array<Y.Map<{ title, bullets }>>. Two clients editing
            different slides converge via Yjs CRDT semantics.
   3. Save: pptxAdapter.savePptxBytes(deck) -> POST .../upload
            (multipart/form-data with file field).
-->
<template>
  <div class="collab-slide-editor">
    <div class="collab-slide-editor__toolbar">
      <span class="collab-slide-editor__title">{{ title }}</span>
      <span class="collab-slide-editor__kind">{{ kindLabel }}</span>
      <span class="collab-slide-editor__connection" :class="{ connected: connected && !saveError }">
        {{ connectionLabel }}
      </span>
      <span class="collab-slide-editor__savetag" :class="savetagClass">
        {{ saveLabel }}
      </span>
      <button class="collab-slide-editor__add-slide" @click="addSlide" type="button">+ 新建幻灯片</button>
      <button class="collab-slide-editor__upload" :disabled="uploading" @click="triggerUpload" type="button">
        {{ uploading ? '上传中...' : '上传 .pptx' }}
      </button>
      <input
        ref="fileInput"
        type="file"
        accept=".pptx"
        style="display:none"
        @change="onUploadFile"
      />
      <button class="collab-slide-editor__export" @click="exportPptx" type="button" :disabled="downloading">
        {{ downloading ? '下载中...' : '下载 .pptx' }}
      </button>
      <span class="collab-slide-editor__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-slide-editor__peer"
          :style="{ backgroundColor: p.color }"
          :title="p.displayName"
        >{{ initialOf(p.displayName) }}</span>
      </span>
    </div>
    <div v-if="loading" class="collab-slide-editor__loading">加载演示文稿中...</div>
    <div v-else class="collab-slide-editor__deck">
      <div
        v-for="(slide, idx) in slides"
        :key="idx"
        class="collab-slide-editor__slide"
        :class="{ active: idx === active }"
        @click="active = idx"
      >
        <div class="collab-slide-editor__slide-num">
          第 {{ idx + 1 }} 页
          <button class="collab-slide-editor__iconbtn" @click.stop="moveSlide(idx, idx - 1)" :disabled="idx === 0" title="上移">↑</button>
          <button class="collab-slide-editor__iconbtn" @click.stop="moveSlide(idx, idx + 1)" :disabled="idx === slides.length - 1" title="下移">↓</button>
          <button class="collab-slide-editor__iconbtn danger" @click.stop="deleteSlide(idx)" :disabled="slides.length <= 1" title="删除">×</button>
        </div>
        <input
          class="collab-slide-editor__slide-title"
          :value="slide.title"
          placeholder="幻灯片标题"
          @input="updateTitle(idx, ($event.target as HTMLInputElement).value)"
        />
        <ul class="collab-slide-editor__bullets">
          <li v-for="(b, bi) in slide.bullets" :key="bi">
            <input
              class="collab-slide-editor__bullet-input"
              :value="b"
              placeholder="要点"
              @input="updateBullet(idx, bi, ($event.target as HTMLInputElement).value)"
            />
            <button class="collab-slide-editor__bullet-del" @click.stop="removeBullet(idx, bi)" type="button">x</button>
          </li>
          <li>
            <button class="collab-slide-editor__bullet-add" @click.stop="addBullet(idx)" type="button">+ 添加要点</button>
          </li>
        </ul>
      </div>
    </div>
    <p v-if="error || saveError" class="collab-slide-editor__error">{{ saveError || error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import * as Y from 'yjs'
import { MessagePlugin } from 'tdesign-vue-next'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'
import {
  openPptx,
  savePptxBytes,
  newPptxDeck,
  type PptxAdapterDeck,
} from '@/editor/adapters/pptxAdapter'
import {
  downloadCollabDocBytes,
  uploadCollabDocBytes,
} from '@/api/collabDoc'

const props = defineProps<{
  docId: string
  title: string
  token: string
  displayName: string
}>()

interface Slide { title: string; bullets: string[] }

const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
const error = ref<string | null>(null)
const slides = ref<Slide[]>([{ title: '新幻灯片', bullets: [''] }])
const active = ref(0)
const downloading = ref(false)
const uploading = ref(false)
const loading = ref(false)
const saveLabel = ref('未修改')
const saveError = ref<string | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

let handle: ReturnType<typeof useYjsCollabDoc> | null = null
let ydeck: Y.Array<Y.Map<unknown>> | null = null
let deck: PptxAdapterDeck = newPptxDeck()
let saveTimer: ReturnType<typeof setTimeout> | null = null

const connectionLabel = computed(() => {
  if (saveError.value) return `保存错误：${saveError.value}`
  if (connected.value) return '已连接'
  return '连接中...'
})

const kindLabel = computed(() => 'PowerPoint (.pptx)')

const savetagClass = computed(() => ({
  dirty: saveLabel.value === '有未保存的修改',
  saving: saveLabel.value === '保存中...',
}))

const initialOf = (name: string) => (name || '?').trim().slice(0, 1).toUpperCase()

const slideToObj = (s: Slide): Record<string, unknown> => ({
  title: s.title,
  bullets: s.bullets,
})

const objToSlide = (raw: Record<string, unknown>): Slide => ({
  title: String(raw.title || ''),
  bullets: Array.isArray(raw.bullets) ? (raw.bullets as unknown[]).map((b) => String(b)) : [],
})

const downloadAsUint8Array = async (): Promise<Uint8Array> => {
  const blob = await downloadCollabDocBytes(props.docId)
  const buf = await blob.arrayBuffer()
  return new Uint8Array(buf)
}

const setup = async () => {
  if (!props.docId || !props.token) return
  handle = useYjsCollabDoc({ docId: props.docId, token: props.token, displayName: props.displayName })
  connected.value = Boolean(handle.connected.value)
  peers.value = (handle.peers.value ?? []) as Array<{ clientId: number; displayName: string; color: string }>
  error.value = (handle.error.value ?? null) as string | null
  watch(handle.connected, (v) => (connected.value = Boolean(v)))
  watch(handle.peers, (v) => (peers.value = (v ?? []) as Array<{ clientId: number; displayName: string; color: string }>))
  watch(handle.error, (v) => (error.value = (v ?? null) as string | null))

  ydeck = handle.ydoc.getArray<Y.Map<unknown>>('slide:deck')

  loading.value = true
  try {
    let bytes: Uint8Array | null = null
    try {
      bytes = await downloadAsUint8Array()
    } catch (e) {
      bytes = null
    }
    if (bytes) {
      deck = await openPptx(bytes)
      slides.value = deck.slides.map((s) => ({ title: s.title, bullets: s.bullets.slice() }))
    } else {
      deck = newPptxDeck()
      slides.value = deck.slides.map((s) => ({ title: s.title, bullets: s.bullets.slice() }))
    }
  } catch (e: any) {
    error.value = `加载失败：${e?.message || e}`
  } finally {
    loading.value = false
  }
  syncFromY()
  ydeck.observeDeep(() => syncFromY())
}

const syncFromY = () => {
  if (!ydeck) return
  const remote = ydeck.toArray().map((m: Y.Map<unknown>) => objToSlide(m.toJSON() as Record<string, unknown>))
  if (remote.length === 0 && handle && ydeck) {
    const localHandle = handle
    const localDeck = ydeck
    localHandle.ydoc.transact(() => {
      const seed = new Y.Map<unknown>()
      Object.entries(slideToObj(slides.value[0])).forEach(([k, v]) => seed.set(k, v))
      localDeck.push([seed])
    })
    return
  }
  slides.value = remote
}

const updateTitle = (idx: number, value: string) => {
  slides.value[idx].title = value
  if (!ydeck || !handle) return
  handle!.ydoc.transact(() => {
    const yslide = ydeck!.get(idx) as Y.Map<unknown>
    yslide.set('title', value)
  })
  scheduleSave()
}

const updateBullet = (idx: number, bi: number, value: string) => {
  slides.value[idx].bullets[bi] = value
  if (!ydeck || !handle) return
  handle!.ydoc.transact(() => {
    const yslide = ydeck!.get(idx) as Y.Map<unknown>
    yslide.set('bullets', slides.value[idx].bullets)
  })
  scheduleSave()
}

const addBullet = (idx: number) => {
  slides.value[idx].bullets.push('')
  if (!ydeck || !handle) return
  handle!.ydoc.transact(() => {
    const yslide = ydeck!.get(idx) as Y.Map<unknown>
    yslide.set('bullets', slides.value[idx].bullets)
  })
  scheduleSave()
}

const removeBullet = (idx: number, bi: number) => {
  slides.value[idx].bullets.splice(bi, 1)
  if (!ydeck || !handle) return
  handle!.ydoc.transact(() => {
    const yslide = ydeck!.get(idx) as Y.Map<unknown>
    yslide.set('bullets', slides.value[idx].bullets)
  })
  scheduleSave()
}

const moveSlide = (from: number, to: number) => {
  if (to < 0 || to >= slides.value.length) return
  if (!ydeck || !handle) return
  const slide = slides.value[from]
  if (!slide) return
  // Local mirror
  const next = slides.value.slice()
  next.splice(from, 1)
  next.splice(to, 0, slide)
  slides.value = next
  // Yjs mutation: delete + insert
  handle!.ydoc.transact(() => {
    const m = ydeck!.get(from) as Y.Map<unknown>
    ydeck!.delete(from, 1)
    const cloned = new Y.Map<unknown>()
    Object.entries(slideToObj(slide)).forEach(([k, v]) => cloned.set(k, v))
    ydeck!.insert(to, [cloned])
    // also propagate originalMap reference removal is implicit via delete above
    void m
  })
  active.value = to
  scheduleSave()
}

const deleteSlide = (idx: number) => {
  if (slides.value.length <= 1) return
  if (!ydeck || !handle) return
  slides.value.splice(idx, 1)
  handle!.ydoc.transact(() => ydeck!.delete(idx, 1))
  if (active.value >= slides.value.length) active.value = slides.value.length - 1
  scheduleSave()
}

const addSlide = () => {
  const fresh = { title: '新幻灯片', bullets: [''] }
  slides.value.push(fresh)
  if (!ydeck || !handle) return
  handle!.ydoc.transact(() => {
    const m = new Y.Map<unknown>()
    Object.entries(fresh).forEach(([k, v]) => m.set(k, v))
    ydeck!.push([m])
  })
  active.value = slides.value.length - 1
  scheduleSave()
}

const scheduleSave = () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => flushSave(), 1500)
}

const flushSave = async () => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  saveLabel.value = '保存中...'
  try {
    deck.slides = slides.value.map((s) => ({ title: s.title, bullets: s.bullets.slice() }))
    const bytes = await savePptxBytes(deck)
    await uploadCollabDocBytes(props.docId, bytes, `${props.title || 'collab-doc'}.pptx`)
    saveLabel.value = '已保存'
    saveError.value = null
    setTimeout(() => {
      if (saveLabel.value === '已保存') saveLabel.value = '未修改'
    }, 1500)
  } catch (e: any) {
    saveError.value = e?.message || String(e)
    saveLabel.value = '保存失败'
  }
}

const exportPptx = async () => {
  downloading.value = true
  try {
    deck.slides = slides.value.map((s) => ({ title: s.title, bullets: s.bullets.slice() }))
    const bytes = await savePptxBytes(deck)
    const ab = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
    const blob = new Blob([ab], {
      type: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
    })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${props.title || 'collab-doc'}.pptx`
    a.click()
    URL.revokeObjectURL(a.href)
    MessagePlugin.success('已下载 .pptx')
  } catch (e: any) {
    MessagePlugin.error(`下载失败：${e?.message || e}`)
  } finally {
    downloading.value = false
  }
}

const triggerUpload = () => fileInput.value?.click()

const onUploadFile = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploading.value = true
  try {
    const buf = await file.arrayBuffer()
    const bytes = new Uint8Array(buf)
    deck = await openPptx(bytes)
    slides.value = deck.slides.map((s) => ({ title: s.title, bullets: s.bullets.slice() }))
    if (ydeck && handle) {
      handle.ydoc.transact(() => {
        ydeck!.delete(0, ydeck!.length)
        for (const slide of slides.value) {
          const m = new Y.Map<unknown>()
          Object.entries(slideToObj(slide)).forEach(([k, v]) => m.set(k, v))
          ydeck!.push([m])
        }
      })
    }
    await uploadCollabDocBytes(props.docId, bytes, file.name)
    saveLabel.value = '已上传'
    MessagePlugin.success(`已上传 ${file.name}`)
  } catch (err: any) {
    MessagePlugin.error(`上传失败：${err?.message || err}`)
  } finally {
    uploading.value = false
    if (input) input.value = ''
  }
}

const teardown = () => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  if (handle) {
    handle.destroy()
    handle = null
  }
  ydeck = null
}

setup()
watch(() => props.docId, () => { teardown(); setup() })
onBeforeUnmount(teardown)
</script>

<style scoped>
.collab-slide-editor { display: flex; flex-direction: column; height: 100%; }
.collab-slide-editor__toolbar {
  display: flex; align-items: center; gap: 8px; padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  flex-wrap: wrap;
}
.collab-slide-editor__title { font-weight: 600; }
.collab-slide-editor__kind { font-size: 11px; padding: 2px 8px; border-radius: 999px; background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-slide-editor__connection { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); }
.collab-slide-editor__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-slide-editor__savetag { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); }
.collab-slide-editor__savetag.dirty { background: var(--td-warning-color-1); color: var(--td-warning-color-7); }
.collab-slide-editor__savetag.saving { background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-slide-editor__add-slide, .collab-slide-editor__export, .collab-slide-editor__upload {
  padding: 4px 10px; font-size: 12px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); cursor: pointer;
}
.collab-slide-editor__add-slide:disabled, .collab-slide-editor__export:disabled, .collab-slide-editor__upload:disabled { opacity: 0.5; cursor: not-allowed; }
.collab-slide-editor__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-slide-editor__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-slide-editor__loading { padding: 24px; }
.collab-slide-editor__deck { display: flex; flex-wrap: wrap; gap: 16px; padding: 16px; overflow: auto; }
.collab-slide-editor__slide { width: 320px; min-height: 200px; border: 1px solid var(--td-component-stroke); border-radius: 8px; padding: 12px; background: var(--td-bg-color-container); }
.collab-slide-editor__slide.active { border-color: var(--td-brand-color-7); box-shadow: 0 0 0 2px var(--td-brand-color-1); }
.collab-slide-editor__slide-num { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 8px; }
.collab-slide-editor__slide-title { width: 100%; font-size: 18px; font-weight: 600; padding: 6px 8px; border: 1px solid var(--td-component-stroke); border-radius: 4px; }
.collab-slide-editor__bullets { list-style: none; padding: 0; margin-top: 12px; }
.collab-slide-editor__bullets li { display: flex; align-items: center; gap: 4px; margin-bottom: 4px; }
.collab-slide-editor__bullet-input { flex: 1; padding: 4px 6px; border: 1px solid var(--td-component-stroke); border-radius: 4px; }
.collab-slide-editor__bullet-del, .collab-slide-editor__bullet-add { background: transparent; border: none; cursor: pointer; color: var(--td-text-color-secondary); font-size: 14px; }
.collab-slide-editor__iconbtn { background: transparent; border: 1px solid var(--td-component-stroke); border-radius: 4px; padding: 1px 6px; margin-left: 6px; cursor: pointer; font-size: 11px; }
.collab-slide-editor__iconbtn:disabled { opacity: 0.4; cursor: not-allowed; }
.collab-slide-editor__iconbtn.danger { color: var(--td-error-color-7); border-color: var(--td-error-color-3); }
.collab-slide-editor__error { color: var(--td-error-color-7); padding: 8px 12px; }
</style>
