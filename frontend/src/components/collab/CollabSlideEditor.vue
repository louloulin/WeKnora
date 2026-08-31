<!--
  CollabSlideEditor — v0.7.25 SLIDE-kind collaborative editor (MVP).

  v0.7.25 ships a structured-but-minimal slide editor backed by a Yjs
  Y.Array<slide> where each slide carries { title, bullets }. Edits to
  titles and bullets fan out over the same Yjs WebSocket the DOC/SHEET
  editors use, so the realtime plumbing is shared.

  The full PPTX-grade WYSIWYG (drag-and-drop shapes, themes, animations,
  inline image upload) is the v0.7.26+ target. For this MVP we render the
  slides with a simple list view + a download endpoint that the backend
  turns into a .pptx via PptxGenJS. This already lets users build a deck
  collaboratively and export it as a real .pptx file.
-->
<template>
  <div class="collab-slide-editor">
    <div class="collab-slide-editor__toolbar">
      <span class="collab-slide-editor__title">{{ title }}</span>
      <span class="collab-slide-editor__connection" :class="{ connected }">
        {{ connected ? '已连接' : '连接中...' }}
      </span>
      <button class="collab-slide-editor__add-slide" @click="addSlide" type="button">+ 新建幻灯片</button>
      <button class="collab-slide-editor__export" @click="exportPptx" type="button" :disabled="exporting">
        {{ exporting ? '导出中...' : '导出 .pptx' }}
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
    <div class="collab-slide-editor__deck">
      <div
        v-for="(slide, idx) in slides"
        :key="idx"
        class="collab-slide-editor__slide"
        :class="{ active: idx === active }"
        @click="active = idx"
      >
        <div class="collab-slide-editor__slide-num">第 {{ idx + 1 }} 页</div>
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
            <button class="collab-slide-editor__bullet-del" @click.stop="removeBullet(idx, bi)" type="button">×</button>
          </li>
          <li>
            <button class="collab-slide-editor__bullet-add" @click.stop="addBullet(idx)" type="button">+ 添加要点</button>
          </li>
        </ul>
      </div>
    </div>
    <p v-if="error" class="collab-slide-editor__error">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import * as Y from 'yjs'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'

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
const exporting = ref(false)

let handle: ReturnType<typeof useYjsCollabDoc> | null = null
let ydeck: Y.Array<Y.Map<unknown>> | null = null

const slideToObj = (s: Slide): Record<string, unknown> => ({
  title: s.title,
  bullets: s.bullets,
})

const objToSlide = (raw: Record<string, unknown>): Slide => ({
  title: String(raw.title || ''),
  bullets: Array.isArray(raw.bullets) ? (raw.bullets as unknown[]).map((b) => String(b)) : [],
})

const setup = () => {
  if (!props.docId || !props.token) return
  handle = useYjsCollabDoc({ docId: props.docId, token: props.token, displayName: props.displayName })
  connected.value = handle.connected.value
  peers.value = handle.peers.value
  error.value = handle.error.value
  watch(handle.connected, (v) => (connected.value = v))
  watch(handle.peers, (v) => (peers.value = v))
  watch(handle.error, (v) => (error.value = v))

  ydeck = handle.ydoc.getArray<Y.Map<unknown>>('slide:deck')
  syncFromY()
  ydeck.observeDeep(() => syncFromY())
}

const syncFromY = () => {
  if (!ydeck) return
  const remote = ydeck.toArray().map((m) => objToSlide(m.toJSON() as Record<string, unknown>))
  if (remote.length === 0) {
    handle!.ydoc.transact(() => {
      const seed = new Y.Map<unknown>()
      Object.entries(slideToObj(slides.value[0])).forEach(([k, v]) => seed.set(k, v))
      ydeck!.push([seed])
    })
    return
  }
  slides.value = remote
}

const updateTitle = (idx: number, value: string) => {
  slides.value[idx].title = value
  if (!ydeck || !handle) return
  handle.ydoc.transact(() => {
    const yslide = ydeck.get(idx) as Y.Map<unknown>
    yslide.set('title', value)
  })
}

const updateBullet = (idx: number, bi: number, value: string) => {
  slides.value[idx].bullets[bi] = value
  if (!ydeck || !handle) return
  handle.ydoc.transact(() => {
    const yslide = ydeck.get(idx) as Y.Map<unknown>
    yslide.set('bullets', slides.value[idx].bullets)
  })
}

const addBullet = (idx: number) => {
  slides.value[idx].bullets.push('')
  if (!ydeck || !handle) return
  handle.ydoc.transact(() => {
    const yslide = ydeck.get(idx) as Y.Map<unknown>
    yslide.set('bullets', slides.value[idx].bullets)
  })
}

const removeBullet = (idx: number, bi: number) => {
  slides.value[idx].bullets.splice(bi, 1)
  if (!ydeck || !handle) return
  handle.ydoc.transact(() => {
    const yslide = ydeck.get(idx) as Y.Map<unknown>
    yslide.set('bullets', slides.value[idx].bullets)
  })
}

const addSlide = () => {
  const fresh = { title: '新幻灯片', bullets: [''] }
  slides.value.push(fresh)
  if (!ydeck || !handle) return
  handle.ydoc.transact(() => {
    const m = new Y.Map<unknown>()
    Object.entries(fresh).forEach(([k, v]) => m.set(k, v))
    ydeck.push([m])
  })
  active.value = slides.value.length - 1
}

const exportPptx = async () => {
  exporting.value = true
  try {
    const res = await fetch(`/api/v1/collaborative-docs/${encodeURIComponent(props.docId)}/export?format=pptx`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${props.token}` },
      body: JSON.stringify({ slides: slides.value, title: props.title }),
    })
    if (!res.ok) throw new Error(`export failed: ${res.status}`)
    const blob = await res.blob()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${props.title}.pptx`
    a.click()
    URL.revokeObjectURL(a.href)
  } catch (e: any) {
    error.value = `export: ${e?.message || e}`
  } finally {
    exporting.value = false
  }
}

const teardown = () => { if (handle) { handle.destroy(); handle = null } }

setup()
watch(() => props.docId, () => { teardown(); setup() })
onBeforeUnmount(teardown)

const initialOf = (name: string) => (name || '?').trim().slice(0, 1).toUpperCase()
</script>

<style scoped>
.collab-slide-editor { display: flex; flex-direction: column; height: 100%; }
.collab-slide-editor__toolbar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--td-component-stroke); }
.collab-slide-editor__title { font-weight: 600; }
.collab-slide-editor__connection { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); }
.collab-slide-editor__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-slide-editor__add-slide, .collab-slide-editor__export { padding: 4px 10px; font-size: 12px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); cursor: pointer; }
.collab-slide-editor__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-slide-editor__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-slide-editor__deck { display: flex; flex-wrap: wrap; gap: 16px; padding: 16px; overflow: auto; }
.collab-slide-editor__slide { width: 320px; min-height: 200px; border: 1px solid var(--td-component-stroke); border-radius: 8px; padding: 12px; background: var(--td-bg-color-container); }
.collab-slide-editor__slide.active { border-color: var(--td-brand-color-7); box-shadow: 0 0 0 2px var(--td-brand-color-1); }
.collab-slide-editor__slide-num { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 8px; }
.collab-slide-editor__slide-title { width: 100%; font-size: 18px; font-weight: 600; padding: 6px 8px; border: 1px solid var(--td-component-stroke); border-radius: 4px; }
.collab-slide-editor__bullets { list-style: none; padding: 0; margin-top: 12px; }
.collab-slide-editor__bullets li { display: flex; align-items: center; gap: 4px; margin-bottom: 4px; }
.collab-slide-editor__bullet-input { flex: 1; padding: 4px 6px; border: 1px solid var(--td-component-stroke); border-radius: 4px; }
.collab-slide-editor__bullet-del, .collab-slide-editor__bullet-add { background: transparent; border: none; cursor: pointer; color: var(--td-text-color-secondary); font-size: 14px; }
.collab-slide-editor__error { color: var(--td-error-color-7); padding: 8px 12px; }
</style>
