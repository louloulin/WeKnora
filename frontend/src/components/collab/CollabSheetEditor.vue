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
      <button class="collab-sheet-editor__add-col" @click="delLastColumn" type="button" :disabled="cols.length <= 1">- 列</button>
      <button class="collab-sheet-editor__add-row" @click="delLastRow" type="button" :disabled="rows.length <= 1">- 行</button>
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
    <!-- Sheet tabs (multi-sheet support) -->
    <div v-if="!loading && sheets.length > 1" class="collab-sheet-editor__tabs">
      <button
        v-for="(sh, si) in sheets"
        :key="si"
        class="collab-sheet-editor__tab"
        :class="{ active: si === activeSheet }"
        @click="switchSheet(si)"
      >
        {{ sh.name }}
      </button>
      <button class="collab-sheet-editor__tab collab-sheet-editor__add-col" @click="addSheet" type="button" title="新增 sheet">+ 新 sheet</button>
    </div>
    <div v-if="loading" class="collab-sheet-editor__loading">加载表格中...</div>
    <!-- Formula bar -->
    <div v-if="!loading" class="collab-sheet-editor__formula">
      <span class="collab-sheet-editor__cellref">{{ selectedRef || '选单元格' }}</span>
      <span class="collab-sheet-editor__fx">fx</span>
      <input
        class="collab-sheet-editor__formula-input"
        :value="formulaValue"
        @input="formulaValue = ($event.target as HTMLInputElement).value; formulaError = null"
        @keydown.enter="commitFormula"
        placeholder="输入公式 (例: =SUM(A1:A10))"
      />
      <span v-if="formulaError" class="collab-sheet-editor__formula-error">⚠ {{ formulaError }}</span>
      <span v-else-if="formulaValue" class="collab-sheet-editor__formula-hint">= {{ formulaResult }}</span>
    </div>
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
                :class="{
                  'is-formula': !!cellFormula(ri, ci),
                  'is-percent': cellPercent(ri, ci),
                  'collab-sheet-editor__cell--selected': selectedRi === ri && selectedCi === ci,
                  'collab-sheet-editor__cell--remote': remoteCellPeer(ri, ci),
                }"
                :style="remoteCellStyle(ri, ci)"
                :value="cellFormula(ri, ci) || cellText(ri, ci)"
                :title="cellFormula(ri, ci) || ''"
                @focus="onCellSelect(ri, ci)"
                @click="onCellSelect(ri, ci)"
                @input="setCell(ri, ci, ($event.target as HTMLInputElement).value)"
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="error || saveError" class="collab-sheet-editor__error">{{ saveError || error }}</p>
  



    <!-- v0.7.38 — sheet comment panel (cell-level anchor). -->
    <CollabCommentsPanel
      :doc-id="docId"
      :token="token"
      :anchor="commentAnchor"
      anchor-label="单元格"
      placeholder="对选中的单元格添加评论…"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import * as Y from 'yjs'
import CollabCommentsPanel from '@/components/collab/CollabCommentsPanel.vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'
import {
  openXlsx,
  saveXlsxBytes,
  newXlsxWorkbook,
  type XlsxAdapterWorkbook,
  type XlsxAdapterCell,
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
const sheets = ref<Array<{ name: string; rows: string[][] }>>([])
const activeSheet = ref(0)
// Selected cell tracking for the formula bar
const selectedRi = ref(-1)
const selectedCi = ref(-1)
const formulaValue = ref('')
const formulaError = ref<string | null>(null)
const selectedRef = computed(() =>
  selectedRi.value >= 0 && selectedCi.value >= 0
    ? `${colName(selectedCi.value)}${selectedRi.value + 1}`
    : '',
)
const formulaResult = computed(() => {
  if (!formulaValue.value.startsWith('=')) return ''
  try {
    return evaluateFormula(formulaValue.value, rows.value)
  } catch (e: any) {
    return ''
  }
})
// v0.7.38 — remote cell-selection highlighter (peer awareness)
const remoteCellPeer = (ri: number, ci: number) => {
  return remoteSelections.value.find((p) => p.cell && p.cell.ri === ri && p.cell.ci === ci) || null
}
const remoteCellStyle = (ri: number, ci: number) => {
  const p = remoteCellPeer(ri, ci)
  if (!p) return {}
  return { outline: `2px solid ${p.color}`, outlineOffset: '-1px' }
}

// v0.7.38 — sheet comment anchor (cell-level)
const commentAnchor = ref<{ type: 'sheet'; ref: string } | null>(null)

const cellLabel = (ri: number, ci: number) => {
  // Spreadsheet column letter (A, B, ..., Z, AA, ...)
  let n = ci
  let s = ''
  do {
    s = String.fromCharCode(65 + (n % 26)) + s
    n = Math.floor(n / 26) - 1
  } while (n >= 0)
  return `${s}${ri + 1}`
}

const onCellSelect = (ri: number, ci: number) => {
  selectedRi.value = ri
  selectedCi.value = ci
  const raw = rows.value[ri]?.[ci]
  formulaValue.value = raw && raw.startsWith('=') ? raw : ''
  formulaError.value = null
  commentAnchor.value = { type: 'sheet', ref: cellLabel(ri, ci) }
  // v0.7.38 — broadcast cell selection on the Yjs awareness layer
  // so collaborators see the active cell outline in real time.
  try {
    handle?.publishCellSelection?.(ri, ci)
  } catch (e) {
    // eslint-disable-next-line no-console
    console.warn('[CollabSheetEditor] publishCellSelection failed', e)
  }
}
const commitFormula = () => {
  if (selectedRi.value < 0 || selectedCi.value < 0) return
  setCell(selectedRi.value, selectedCi.value, formulaValue.value)
  formulaError.value = null
}
const colNameToIndex = (name: string): number => {
  let n = 0
  for (const ch of name.toUpperCase()) n = n * 26 + (ch.charCodeAt(0) - 64)
  return n - 1
}
const resolveCellRef = (ref: string, grid: string[][]): number => {
  const m = /^([A-Z]+)(d+)$/i.exec(ref.trim())
  if (!m) return NaN
  const ci = colNameToIndex(m[1])
  const ri = Number(m[2]) - 1
  const v = Number(grid[ri]?.[ci])
  return Number.isNaN(v) ? 0 : v
}
const parseRangeArgs = (args: string, grid: string[][]): number[][] => {
  const parts = args.split(/[,;]/).map((s) => s.trim()).filter(Boolean)
  const out: number[][] = []
  for (const p of parts) {
    const range = /^([A-Z]+d+):([A-Z]+d+)$/i.exec(p)
    if (range) {
      const ca = colNameToIndex(range[1].replace(/d+/g, ''))
      const cb = colNameToIndex(range[2].replace(/d+/g, ''))
      const ra = Number(range[1].replace(/[A-Z]+/g, '')) - 1
      const rb = Number(range[2].replace(/[A-Z]+/g, '')) - 1
      for (let r = ra; r <= rb; r++) {
        const row: number[] = []
        for (let c = ca; c <= cb; c++) row.push(Number(grid[r]?.[c]) || 0)
        out.push(row)
      }
    } else {
      const v = resolveCellRef(p, grid)
      if (!Number.isNaN(v)) out.push([v])
    }
  }
  return out
}
const evaluateFormula = (expr: string, grid: string[][]): string => {
  const cleaned = expr.replace(/^=/, '').trim()
  if (!cleaned) return ''
  const fnMatch = /^([A-Z]+)((.*))$/i.exec(cleaned)
  if (fnMatch) {
    const fn = fnMatch[1].toUpperCase()
    const args = parseRangeArgs(fnMatch[2], grid)
    const flat = args.flat()
    if (!flat.length) throw new Error('empty range')
    switch (fn) {
      case 'SUM': return String(flat.reduce((a, b) => a + b, 0))
      case 'AVERAGE': return String(flat.reduce((a, b) => a + b, 0) / flat.length)
      case 'COUNT': return String(flat.length)
      case 'MIN': return String(Math.min(...flat))
      case 'MAX': return String(Math.max(...flat))
      default: throw new Error('unknown function: ' + fn)
    }
  }
  const tokens = cleaned.split(/([+*\-])/).map((s) => s.trim()).filter(Boolean)
  let acc: number | null = null
  let op = '+'
  for (const tok of tokens) {
    if (tok === '+' || tok === '-' || tok === '*' || tok === '/') { op = tok; continue }
    const v = Number(tok)
    const refV = Number.isNaN(v) ? resolveCellRef(tok, grid) : v
    if (Number.isNaN(refV)) throw new Error('bad token: ' + tok)
    acc = acc == null ? refV : op === '+' ? acc + refV : op === '-' ? acc - refV : op === '*' ? acc * refV : acc / refV
  }
  return acc == null ? '' : String(acc)
}
const switchSheet = (i: number) => {
  if (i < 0 || i >= sheets.value.length) return
  // Persist current edits into sheets[]
  if (sheets.value[activeSheet.value]) {
    sheets.value[activeSheet.value] = {
      name: sheets.value[activeSheet.value].name,
      rows: rows.value.map((r) => r.slice()),
    }
  }
  activeSheet.value = i
  rows.value = sheets.value[i].rows.map((r) => r.slice())
  cols.value = Array.from({ length: Math.max(rows.value[0]?.length || 0, 1) }, (_, k) => colName(k))
  selectedRi.value = -1
  selectedCi.value = -1
  formulaValue.value = ''
}
const addSheet = () => {
  if (sheets.value[activeSheet.value]) {
    sheets.value[activeSheet.value] = {
      name: sheets.value[activeSheet.value].name,
      rows: rows.value.map((r) => r.slice()),
    }
  }
  const name = 'Sheet' + (sheets.value.length + 1)
  sheets.value.push({ name, rows: [Array.from({ length: cols.value.length }, () => '')] })
  activeSheet.value = sheets.value.length - 1
  rows.value = sheets.value[activeSheet.value].rows.map((r) => r.slice())
  selectedRi.value = -1
  selectedCi.value = -1
  formulaValue.value = ''
  scheduleSave()
}
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

const remoteSelections = ref<Array<{
  clientId: number
  displayName: string
  color: string
  cell?: { ri: number; ci: number } | null
}>>([])

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
  // v0.7.38 — remote cell-selection awareness
  if (handle.remoteSelections) {
    remoteSelections.value = handle.remoteSelections.value as any
    watch(handle.remoteSelections, (v) => (remoteSelections.value = (v ?? []) as any))
  }
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
      sheets.value = wb.sheets.map((sh) => ({ name: sh.name, rows: sh.rows.map((r) => r.map((c) => String(c.v ?? ''))) }))
      activeSheet.value = 0
      const first = sheets.value[0]
      if (first) {
        const maxCol = first.rows.reduce((m, r) => Math.max(m, r.length), 0)
        cols.value = Array.from({ length: Math.max(maxCol, 1) }, (_, i) => colName(i))
        rows.value = first.rows.map((r) => {
          const padded = r.slice()
          while (padded.length < cols.value.length) padded.push('')
          return padded
        })
        if (rows.value.length === 0) {
          rows.value = [Array.from({ length: cols.value.length }, () => '')]
        }
      } else {
        sheets.value = [{ name: 'Sheet1', rows: [Array.from({ length: cols.value.length }, () => '')] }]
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

/** A cell may be a raw string (from Yjs) or an XlsxAdapterCell (after
 *  openXlsx). Normalize to one shape so the helpers below stay symmetric. */
const normalizeCell = (raw: unknown): XlsxAdapterCell => {
  if (raw == null || raw === '') return { v: '' }
  if (typeof raw === 'object') return raw as XlsxAdapterCell
  return { v: String(raw) }
}
const cellFormula = (ri: number, ci: number) => {
  const c = normalizeCell(rows.value[ri]?.[ci])
  if (c.f) return c.f
  const v = c.v
  return typeof v === 'string' && v.startsWith('=') ? v.slice(1) : ''
}
const cellPercent = (ri: number, ci: number) => {
  const c = normalizeCell(rows.value[ri]?.[ci])
  return (c.z ?? '').includes('%')
}

/** Build an XlsxAdapterCell from a raw string cell. Strings starting with
 *  '=' become formulas (cell.f = the right-hand side); pure numerics become
 *  number cells with no format. Anything else becomes a text cell. */
const buildCell = (raw: string | undefined): XlsxAdapterCell => {
  const v = raw ?? ''
  if (typeof v === 'string' && v.startsWith('=') && v.length > 1) {
    return { v: '', f: v.slice(1) }
  }
  if (typeof v === 'string' && /^-?\d+(\.\d+)?$/.test(v)) {
    return { v: Number(v) }
  }
  return { v }
}

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

const delLastColumn = () => {
  if (cols.value.length <= 1) return
  if (!ycols || !handle) return
  handle!.ydoc.transact(() => {
    const localCols = ycols!
    localCols.delete(localCols.length - 1, 1)
  })
  rows.value = rows.value.map((r) => r.slice(0, -1))
  if (ymap) {
    handle!.ydoc.transact(() => {
      ymap!.forEach((yrow, key) => {
        const lastKey = String(cols.value.length)
        yrow.delete(lastKey)
        if (yrow.size === 0) ymap!.delete(key)
      })
    })
  }
  scheduleSave()
}

const delLastRow = () => {
  if (rows.value.length <= 1) return
  rows.value = rows.value.slice(0, -1)
  if (ymap) ymap!.delete(String(rows.value.length))
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
    sheets.value[activeSheet.value] = {
      name: sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1)),
      rows: rows.value.map((r) => r.slice()),
    }
    wb.sheets = sheets.value.map((sh) => ({
      name: sh.name,
      rows: sh.rows.map((r) => r.map((v) => buildCell(v))),
    }))
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
    sheets.value[activeSheet.value] = {
      name: sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1)),
      rows: rows.value.map((r) => r.slice()),
    }
    wb.sheets = sheets.value.map((sh) => ({
      name: sh.name,
      rows: sh.rows.map((r) => r.map((v) => buildCell(v))),
    }))
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

/* v0.7.28 — sheet tabs + formula bar */
.collab-sheet-editor__tabs {
  display: flex;
  gap: 2px;
  padding: 4px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  overflow-x: auto;
}
.collab-sheet-editor__tab {
  padding: 4px 12px;
  font-size: 12px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 4px 4px 0 0;
  cursor: pointer;
  color: var(--td-text-color-secondary);
}
.collab-sheet-editor__tab.active {
  background: var(--td-bg-color-container);
  border-color: var(--td-component-stroke);
  border-bottom-color: var(--td-bg-color-container);
  color: var(--td-text-color-primary);
  font-weight: 600;
  margin-bottom: -1px;
}
.collab-sheet-editor__formula {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}
.collab-sheet-editor__cellref {
  min-width: 56px;
  font-weight: 600;
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.collab-sheet-editor__fx {
  font-style: italic;
  font-weight: 700;
  color: var(--td-brand-color-7);
}
.collab-sheet-editor__formula-input {
  flex: 1;
  padding: 4px 8px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 13px;
}
.collab-sheet-editor__formula-error {
  color: var(--td-error-color-7);
  font-size: 12px;
}
.collab-sheet-editor__formula-hint {
  color: var(--td-brand-color-7);
  font-size: 12px;
  font-family: 'JetBrains Mono', ui-monospace, monospace;
}
.collab-sheet-editor__cell--selected {
  outline: 2px solid var(--td-brand-color-7);
  outline-offset: -2px;
}
</style>
.collab-sheet-editor__cell-input.is-formula { color: var(--td-brand-color-7); font-family: 'JetBrains Mono', ui-monospace, monospace; }
.collab-sheet-editor__cell-input.is-percent { text-align: right; }
