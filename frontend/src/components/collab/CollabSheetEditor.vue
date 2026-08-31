<!--
  CollabSheetEditor — v0.7.25 SHEET-kind collaborative editor (MVP).

  This is a deliberately thin sheet canvas backed by the shared Yjs doc
  (a Y.Map<rowKey, Y.Map<colKey, Y.Text>>). It demonstrates the realtime
  fan-out over the same Yjs WebSocket the DOC editor uses, so end-to-end
  wiring (presence + fan-out + snapshot persistence) is proven before the
  full Univer integration lands.

  The full Univer (Apache 2.0) preset is the v0.7.26 target — until then
  this MVP lets two clients type into the same cell and see each other.
-->
<template>
  <div class="collab-sheet-editor">
    <div class="collab-sheet-editor__toolbar">
      <span class="collab-sheet-editor__title">{{ title }}</span>
      <span class="collab-sheet-editor__connection" :class="{ connected }">
        {{ connected ? '已连接' : '连接中...' }}
      </span>
      <button class="collab-sheet-editor__add-col" @click="addColumn" type="button">+ 列</button>
      <button class="collab-sheet-editor__add-row" @click="addRow" type="button">+ 行</button>
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
    <p v-if="error" class="collab-sheet-editor__error">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import * as Y from 'yjs'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'

const props = defineProps<{
  docId: string
  title: string
  token: string
  displayName: string
  initialCols?: string[]
  initialRows?: number
}>()

const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
const error = ref<string | null>(null)
const cols = ref<string[]>(props.initialCols && props.initialCols.length > 0 ? props.initialCols : ['A', 'B', 'C'])
const rows = ref<string[][]>(
  Array.from({ length: props.initialRows && props.initialRows > 0 ? props.initialRows : 5 }, () =>
    Array.from({ length: cols.value.length }, () => ''),
  ),
)

let handle: ReturnType<typeof useYjsCollabDoc> | null = null
let ymap: Y.Map<Y.Map<string>> | null = null
let ycols: Y.Array<string> | null = null

const setup = () => {
  if (!props.docId || !props.token) return
  handle = useYjsCollabDoc({ docId: props.docId, token: props.token, displayName: props.displayName })
  connected.value = handle.connected.value
  peers.value = handle.peers.value
  error.value = handle.error.value
  watch(handle.connected, (v) => (connected.value = !!v as boolean))
  watch(handle.peers, (v) => (peers.value = (v ?? []) as Array<{ clientId: number; displayName: string; color: string }>))
  watch(handle.error, (v) => (error.value = (v ?? null) as string | null))

  ycols = handle.ydoc.getArray<string>('sheet:cols')
  ymap = handle.ydoc.getMap<Y.Map<string>>('sheet:cells')
  // Initial sync from remote state.
  const remoteCols = ycols.toArray()
  if (remoteCols.length > 0) cols.value = remoteCols
  handle.ydoc.transact(() => {
    if (ycols && ycols.length === 0) ycols.insert(0, cols.value)
  })
  // Observe future changes.
  ycols.observe(() => {
    if (!ycols) return
    const next = ycols.toArray()
    if (JSON.stringify(next) !== JSON.stringify(cols.value)) {
      cols.value = next
      // pad rows to match
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
  handle.ydoc.transact(() => {
    let yrow = ymap.get(String(ri))
    if (!yrow) {
      yrow = new Y.Map<string>()
      ymap.set(String(ri), yrow)
    }
    yrow.set(String(ci), value)
  })
}

const addColumn = () => {
  if (!ycols || !handle) return
  handle.ydoc.transact(() => ycols.push([`列${cols.value.length + 1}`]))
  rows.value = rows.value.map((r) => [...r, ''])
}

const addRow = () => {
  rows.value.push(Array.from({ length: cols.value.length }, () => ''))
  syncFromY()
}

const renameColumn = (ci: number, name: string) => {
  if (!ycols || !handle) return
  handle.ydoc.transact(() => ycols.delete(ci, 1))
  handle.ydoc.transact(() => ycols.insert(ci, [name]))
  cols.value[ci] = name
}

const teardown = () => {
  if (handle) {
    handle.destroy()
    handle = null
  }
}

setup()
watch(() => props.docId, () => { teardown(); setup() })
onBeforeUnmount(teardown)

const initialOf = (name: string) => (name || '?').trim().slice(0, 1).toUpperCase()
</script>

<style scoped>
.collab-sheet-editor { display: flex; flex-direction: column; height: 100%; }
.collab-sheet-editor__toolbar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--td-component-stroke); }
.collab-sheet-editor__title { font-weight: 600; }
.collab-sheet-editor__connection { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); }
.collab-sheet-editor__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-sheet-editor__add-col, .collab-sheet-editor__add-row { padding: 4px 10px; font-size: 12px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); cursor: pointer; }
.collab-sheet-editor__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-sheet-editor__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-sheet-editor__grid { border-collapse: collapse; width: 100%; }
.collab-sheet-editor__colhead, .collab-sheet-editor__rowhead { background: var(--td-bg-color-secondarycontainer); padding: 6px 8px; font-weight: 500; min-width: 80px; border: 1px solid var(--td-component-stroke); }
.collab-sheet-editor__grid td { border: 1px solid var(--td-component-stroke); padding: 0; }
.collab-sheet-editor__cell-input, .collab-sheet-editor__header-input { width: 100%; padding: 6px 8px; border: none; outline: none; background: transparent; }
.collab-sheet-editor__cell-input:focus, .collab-sheet-editor__header-input:focus { background: var(--td-brand-color-1); }
.collab-sheet-editor__error { color: var(--td-error-color-7); padding: 8px 12px; }
</style>
