<!--
  CollabDocRuler.vue — DOC 水平标尺（v0.7.78）

  仿 GenOffice apps/docs/src/renderer/components/Ruler.tsx：
  - 顶部刻度（cm/inch 切换）
  - 拖动调整左右页边距
  - 与 .collab-doc-pro__surface 宽度同步
  - 通过 rulerVisible ref 控制显隐
-->
<template>
  <div class="collab-doc-ruler" data-testid="doc-ruler" v-if="visible">
    <div class="collab-doc-ruler__track" :style="{ width: rulerWidth + 'px' }">
      <div class="collab-doc-ruler__ticks">
        <span
          v-for="tick in ticks"
          :key="tick.pos"
          class="collab-doc-ruler__tick"
          :class="{ 'is-major': tick.major }"
          :style="{ left: tick.pos + 'px' }"
        >
          <span v-if="tick.major" class="collab-doc-ruler__label">{{ tick.label }}</span>
        </span>
      </div>
      <div
        class="collab-doc-ruler__margin collab-doc-ruler__margin--left"
        :style="{ width: leftMargin + 'px' }"
        @mousedown="onStartDrag('left', $event)"
        data-testid="doc-ruler-margin-left"
      ></div>
      <div
        class="collab-doc-ruler__margin collab-doc-ruler__margin--right"
        :style="{ width: rightMargin + 'px' }"
        @mousedown="onStartDrag('right', $event)"
        data-testid="doc-ruler-margin-right"
      ></div>
    </div>
    <div class="collab-doc-ruler__hint">cm · 拖动蓝色条调整页边距</div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount } from 'vue'

const props = withDefaults(defineProps<{
  visible?: boolean
  rulerWidth?: number
  leftMargin?: number
  rightMargin?: number
  unit?: 'cm' | 'in'
}>(), {
  visible: true,
  rulerWidth: 816,
  leftMargin: 64,
  rightMargin: 64,
  unit: 'cm',
})

const emit = defineEmits<{
  (e: 'update:leftMargin', v: number): void
  (e: 'update:rightMargin', v: number): void
}>()

const ticks = computed(() => {
  const arr: { pos: number; major: boolean; label?: string }[] = []
  // 1 cm = 38px @ 96dpi; 1 inch = 96px
  const px = props.unit === 'cm' ? 38 : 96
  const max = props.rulerWidth
  for (let i = 0; i * px < max; i++) {
    const major = i % 1 === 0
    arr.push({
      pos: i * px,
      major,
      label: major ? String(i) : undefined,
    })
  }
  return arr
})

const onStartDrag = (which: 'left' | 'right', e: MouseEvent) => {
  e.preventDefault()
  const startX = e.clientX
  const startLeft = props.leftMargin
  const startRight = props.rightMargin
  const onMove = (ev: MouseEvent) => {
    const dx = ev.clientX - startX
    if (which === 'left') {
      const v = Math.max(0, Math.min(props.rulerWidth - props.rightMargin - 10, startLeft + dx))
      emit('update:leftMargin', Math.round(v))
    } else {
      const v = Math.max(0, Math.min(props.rulerWidth - props.leftMargin - 10, startRight - dx))
      emit('update:rightMargin', Math.round(v))
    }
  }
  const onUp = () => {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

onBeforeUnmount(() => {
  document.removeEventListener('mousemove', () => {})
})
</script>

<style scoped>
.collab-doc-ruler {
  display: flex;
  flex-direction: column;
  align-items: center;
  background: var(--app-surface, #161a22);
  border-bottom: 1px solid var(--app-border, #2c313b);
  padding: 4px 0;
  user-select: none;
}
.collab-doc-ruler__track {
  position: relative;
  height: 22px;
  background: var(--app-surface-raised, #1d212a);
  border: 1px solid var(--app-border, #2c313b);
  border-radius: 2px;
}
.collab-doc-ruler__ticks {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.collab-doc-ruler__tick {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1px;
  background: #5a6473;
  opacity: 0.6;
}
.collab-doc-ruler__tick.is-major {
  background: #8c98a8;
  opacity: 1;
}
.collab-doc-ruler__label {
  position: absolute;
  top: 1px;
  left: 2px;
  font-size: 9px;
  color: #9ca6b4;
  font-family: ui-monospace, monospace;
}
.collab-doc-ruler__margin {
  position: absolute;
  top: 0;
  bottom: 0;
  background: rgba(90, 168, 255, 0.18);
  border-top: 1px solid #5aa8ff;
  border-bottom: 1px solid #5aa8ff;
  cursor: ew-resize;
}
.collab-doc-ruler__margin--left { left: 0; }
.collab-doc-ruler__margin--right { right: 0; }
.collab-doc-ruler__margin:hover {
  background: rgba(90, 168, 255, 0.30);
}
.collab-doc-ruler__hint {
  font-size: 10px;
  color: #6c7686;
  margin-top: 2px;
}
</style>
