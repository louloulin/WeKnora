<!--
  CollabSheetEditor — v0.7.26 SHEET-kind collaborative editor with real
  .xlsx byte round-trip.

  Architecture:
   1. Open: GET /collaborative-docs/:id/download (latest .xlsx bytes)
            -> xlsxAdapter.openXlsx(bytes) seeds a structured sheet
            (rows[i][j] = { v: cell value }).
   2. Realtime: Y.Map<rowKey, Y.Map<colKey, Y.Text>>. Two clients editing
            different cells converge via Yjs CRDT.
   3. Save: xlsxAdapter.saveXlsxBytes(wb) -> POST .../upload
            (multipart/form-data with file field).
-->
<template>
  <div class="collab-sheet-editor">
    <div class="collab-sheet-editor__toolbar">
      <span class="collab-sheet-editor__title">{{ title }}</span>
      <span class="collab-sheet-editor__kind">{{ kindLabel }}</span>
      <span class="collab-sheet-editor__connection" :class="{ connected: connected && !saveError }">
        {{ connectionLabel }}
      </span>
      <span class="collab-sheet-editor__savetag" :class="savetagClass">
        {{ saveLabel }}
      </span>
      <button class="collab-sheet-editor__add-col" @click="addColumn" type="button">+ 列</button>
      <button class="collab-sheet-editor__add-row" @click="addRow" type="button">+ 行</button>
      <button class="collab-sheet-editor__upload" :disabled="uploading" @click="triggerUpload" type="button">
        {{ uploading ? '上传中...' : '上传 .xlsx' }}
      </button>
      <input
        ref="fileInput"
        type="file"
        accept=".xlsx"
        style="display:none"
        @change="onUploadFile"
      />
      <button class="collab-sheet-editor__export" :disabled="downloading" @click="exportXlsx" type="button">
        {{ downloading ? '下载中...' : '下载 .xlsx' }}
      </button>
      <span class="collab-sheet-editor__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-sheet-editor__peer"
          :style="{ backgroundColor: p.color }"
          :title="p.displayName"
        >{{ initialOf(p.displayName) }}</span>
      </span>
    </div>
    <div v-if="loading" class="collab-sheet-editor__loading">加载表格中...</div>
    <div v-else class="collab-sheet-editor__table-wrap">
      <table class="collab-sheet-editor__grid">
        <thead>
          <tr>
            <th class="collab-sheet-editor__rowhead"></th>
            <th v-for="(col, ci) in cols" :key="ci" class="collab-sheet-editor__colhead">
              <input
                class="collab-sheet-editor__header-input"
                :value="col"
                @input="renameColumn(ci, ($event.target as HTMLInputElement).value)"
              />
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, ri) in rows" :key="ri">
            <th class="collab-sheet-editor__rowhead">{{ ri + 1 }}</th>
            <td v-for="(col, ci) in cols" :key="ci">
              <input
                class="collab-sheet-editor__cell-input"
                :value="cellText(ri, ci)"
                @input="setCell(ri, ci, ($event.target as HTMLInputElement).value)"
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="error || saveError" class="collab-sheet-editor__error">{{ saveError || error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import * as Y from 'yjs'
import { MessagePlugin } from 'tdesign-vue-next'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'
import {
  openXlsx,
  saveXlsxBytes,
  newXlsxWorkbook,
  type XlsxAdapterWorkbook,
} from '@/editor/adapters/xlsxAdapter'
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

const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
const error = ref<string | null>(null)
const cols = ref<string[]>(['A', 'B', 'C'])
const rows = ref<string[][]>(
  Array.from({ length: 5 }, () => Array.from({ length: cols.value.length }, () => '')),
)
const loading = ref(false)
const downloading = ref(false)
const uploading = ref(false)
const saveLabel = ref('未修改')
const saveError = ref<string | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

let handle: ReturnType<typeof useYjsCollabDoc> | null = null
let ymap: Y.Map<Y.Map<string>> | null = null
let ycols: Y.Array<string> | null = null
let wb: XlsxAdapterWorkbook = newXlsxWorkbook()
let saveTimer: ReturnType<typeof setTimeout> | null = null

const connectionLabel = computed(() => {
  if (saveError.value) return `保存错误：${saveError.value}`
  if (connected.value) return '已连接'
  return '连接中...'
})

const kindLabel = computed(() => 'Excel 表格 (.xlsx)')

const savetagClass = computed(() => ({
  dirty: saveLabel.value === '有未保存的修改',
  saving: saveLabel.value === '保存中...',
}))

const initialOf = (name: string) => (name || '?').trim().slice(0, 1).toUpperCase()

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

  ycols = handle.ydoc.getArray<string>('sheet:cols')
  ymap = handle.ydoc.getMap<Y.Map<string>>('sheet:cells')

  loading.value = true
  try {
    let bytes: Uint8Array | null = null
    try {
      bytes = await downloadAsUint8Array()
    } catch (e) {
      bytes = null
    }
    if (bytes) {
      wb = await openXlsx(bytes)
      const first = wb.sheets[0]
      if (first) {
        const maxCol = first.rows.reduce((m, r) => Math.max(m, r.length), 0)
        cols.value = Array.from({ length: maxCol }, (_, i) => colName(i))
        rows.value = first.rows.map((r) => {
          const padded = r.map((c) => String(c.v ?? ''))
          while (padded.length < maxCol) padded.push('')
          return padded
        })
        if (rows.value.length === 0) {
          rows.value = [Array.from({ length: cols.value.length }, () => '')]
        }
      }
    }
  } catch (e: any) {
    error.value = `加载失败：${e?.message || e}`
  } finally {
    loading.value = false
  }

  // Sync from Yjs
  const remoteCols = ycols.toArray()
  if (remoteCols.length > 0) cols.value = remoteCols
  handle.ydoc.transact(() => {
    if (ycols && ycols.length === 0) ycols.insert(0, cols.value)
  })
  ycols.observe(() => {
    if (!ycols) return
    const next = ycols.toArray()
    if (JSON.stringify(next) !== JSON.stringify(cols.value)) {
      cols.value = next
      rows.value = rows.value.map((r) => {
        if (r.length < next.length) return [...r, ...Array.from({ length: next.length - r.length }, () => '')]
        if (r.length > next.length) return r.slice(0, next.length)
        return r
      })
    }
  })
  ymap.observe(() => syncFromY())
  syncFromY()
}

const colName = (i: number): string => {
  let n = i
  let s = ''
  while (n >= 0) {
    s = String.fromCharCode(65 + (n % 26)) + s
    n = Math.floor(n / 26) - 1
  }
  return s
}

const syncFromY = () => {
  if (!ymap) return
  const next: string[][] = []
  for (let i = 0; i < rows.value.length; i++) {
    const rowKey = String(i)
    const yrow = ymap.get(rowKey)
    const row: string[] = []
    for (let ci = 0; ci < cols.value.length; ci++) {
      row.push(yrow ? yrow.get(String(ci)) || '' : '')
    }
    next.push(row)
  }
  rows.value = next
}

const cellText = (ri: number, ci: number) => rows.value[ri]?.[ci] ?? ''
const setCell = (ri: number, ci: number, value: string) => {
  rows.value[ri][ci] = value
  if (!ymap || !handle) return
  handle!.ydoc.transact(() => {
    let yrow = ymap!.get(String(ri))
    if (!yrow) {
      yrow = new Y.Map<string>()
      ymap!.set(String(ri), yrow)
    }
    yrow.set(String(ci), value)
  })
  scheduleSave()
}

const addColumn = () => {
  if (!ycols || !handle) return
  handle!.ydoc.transact(() => ycols!.push([`列${cols.value.length + 1}`]))
  rows.value = rows.value.map((r) => [...r, ''])
  scheduleSave()
}

const addRow = () => {
  rows.value.push(Array.from({ length: cols.value.length }, () => ''))
  syncFromY()
  scheduleSave()
}

const renameColumn = (ci: number, name: string) => {
  if (!ycols || !handle) return
  handle!.ydoc.transact(() => ycols!.delete(ci, 1))
  handle!.ydoc.transact(() => ycols!.insert(ci, [name]))
  cols.value[ci] = name
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
    wb.sheets = [{
      name: 'Sheet1',
      rows: rows.value.map((r) => r.map((v) => ({ v }))),
    }]
    const bytes = await saveXlsxBytes(wb)
    await uploadCollabDocBytes(props.docId, bytes, `${props.title || 'collab-doc'}.xlsx`)
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

const exportXlsx = async () => {
  downloading.value = true
  try {
    wb.sheets = [{
      name: 'Sheet1',
      rows: rows.value.map((r) => r.map((v) => ({ v }))),
    }]
    const bytes = await saveXlsxBytes(wb)
    const ab = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
    const blob = new Blob([ab], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${props.title || 'collab-doc'}.xlsx`
    a.click()
    URL.revokeObjectURL(a.href)
    MessagePlugin.success('已下载 .xlsx')
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
    wb = await openXlsx(bytes)
    const first = wb.sheets[0]
    if (first) {
      const maxCol = first.rows.reduce((m, r) => Math.max(m, r.length), 0)
      cols.value = Array.from({ length: maxCol }, (_, i) => colName(i))
      rows.value = first.rows.map((r) => {
        const padded = r.map((c) => String(c.v ?? ''))
        while (padded.length < maxCol) padded.push('')
        return padded
      })
      if (rows.value.length === 0) {
        rows.value = [Array.from({ length: cols.value.length }, () => '')]
      }
      // reset Yjs cells
      if (ymap && handle) {
        handle.ydoc.transact(() => {
          for (const k of Array.from(ymap!.keys())) {
            ymap!.delete(k)
          }
          for (let i = 0; i < rows.value.length; i++) {
            const yrow = new Y.Map<string>()
            for (let ci = 0; ci < cols.value.length; ci++) {
              yrow.set(String(ci), rows.value[i][ci] || '')
            }
            ymap!.set(String(i), yrow)
          }
        })
      }
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
  ymap = null
  ycols = null
}

setup()
watch(() => props.docId, () => { teardown(); setup() })
onBeforeUnmount(teardown)
</script>

<style scoped>
.collab-sheet-editor { display: flex; flex-direction: column; height: 100%; }
.collab-sheet-editor__toolbar {
  display: flex; align-items: center; gap: 8px; padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  flex-wrap: wrap;
}
.collab-sheet-editor__title { font-weight: 600; }
.collab-sheet-editor__kind { font-size: 11px; padding: 2px 8px; border-radius: 999px; background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-sheet-editor__connection { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); }
.collab-sheet-editor__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-sheet-editor__savetag { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); }
.collab-sheet-editor__savetag.dirty { background: var(--td-warning-color-1); color: var(--td-warning-color-7); }
.collab-sheet-editor__savetag.saving { background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-sheet-editor__add-col, .collab-sheet-editor__add-row, .collab-sheet-editor__export, .collab-sheet-editor__upload {
  padding: 4px 10px; font-size: 12px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); cursor: pointer;
}
.collab-sheet-editor__add-col:disabled, .collab-sheet-editor__add-row:disabled, .collab-sheet-editor__export:disabled, .collab-sheet-editor__upload:disabled { opacity: 0.5; cursor: not-allowed; }
.collab-sheet-editor__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-sheet-editor__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-sheet-editor__loading { padding: 24px; }
.collab-sheet-editor__table-wrap { flex: 1; overflow: auto; padding: 16px; }
.collab-sheet-editor__grid { border-collapse: collapse; min-width: 100%; }
.collab-sheet-editor__colhead, .collab-sheet-editor__rowhead { background: var(--td-bg-color-secondarycontainer); padding: 6px 8px; font-weight: 500; min-width: 80px; border: 1px solid var(--td-component-stroke); }
.collab-sheet-editor__grid td { border: 1px solid var(--td-component-stroke); padding: 0; }
.collab-sheet-editor__cell-input, .collab-sheet-editor__header-input { width: 100%; padding: 6px 8px; border: none; outline: none; background: transparent; }
.collab-sheet-editor__cell-input:focus, .collab-sheet-editor__header-input:focus { background: var(--td-brand-color-1); }
.collab-sheet-editor__error { color: var(--td-error-color-7); padding: 8px 12px; }
</style>
