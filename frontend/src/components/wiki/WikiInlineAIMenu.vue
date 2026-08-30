<template>
  <div
    v-if="visible"
    ref="menuRef"
    class="wiki-inline-ai-menu"
    role="toolbar"
    :aria-label="$t('knowledgeEditor.wikiInlineAI.regionLabel')"
    :style="positionStyle"
  >
    <button
      v-for="action in actions"
      :key="action.id"
      type="button"
      class="wiki-inline-ai-menu__btn"
      :title="$t(action.titleKey)"
      :aria-label="$t(action.titleKey)"
      @click="onClick(action)"
      @mousedown.prevent
    >
      <t-icon :name="action.icon" size="14px" />
      <span class="wiki-inline-ai-menu__label">{{ $t(action.labelKey) }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const MIN_SELECTION_LENGTH = 1
const MAX_SELECTION_LENGTH = 4000
const MENU_HEIGHT_ESTIMATE = 36
const MENU_WIDTH_ESTIMATE = 360
const VIEWPORT_PADDING = 8

interface InlineAIAction {
  id: 'summarize' | 'translate' | 'explain' | 'improve' | 'ask'
  icon: string
  labelKey: string
  titleKey: string
}

const props = defineProps<{
  /** The DOM element to monitor for text selection (e.g. reader body). */
  containerRef: HTMLElement | null
}>()

const emit = defineEmits<{
  (e: 'ai', payload: { action: InlineAIAction['id']; text: string }): void
}>()

const { t } = useI18n()

const visible = ref(false)
const menuRef = ref<HTMLElement | null>(null)
const selectionText = ref('')
const menuTop = ref(0)
const menuLeft = ref(0)
let hideTimer: number | null = null

const actions: InlineAIAction[] = [
  { id: 'summarize', icon: 'list', labelKey: 'knowledgeEditor.wikiInlineAI.summarize', titleKey: 'knowledgeEditor.wikiInlineAI.summarizeTitle' },
  { id: 'translate', icon: 'translate', labelKey: 'knowledgeEditor.wikiInlineAI.translate', titleKey: 'knowledgeEditor.wikiInlineAI.translateTitle' },
  { id: 'explain', icon: 'help', labelKey: 'knowledgeEditor.wikiInlineAI.explain', titleKey: 'knowledgeEditor.wikiInlineAI.explainTitle' },
  { id: 'improve', icon: 'edit', labelKey: 'knowledgeEditor.wikiInlineAI.improve', titleKey: 'knowledgeEditor.wikiInlineAI.improveTitle' },
  { id: 'ask', icon: 'chat', labelKey: 'knowledgeEditor.wikiInlineAI.ask', titleKey: 'knowledgeEditor.wikiInlineAI.askTitle' },
]

const positionStyle = computed(() => ({
  top: `${menuTop.value}px`,
  left: `${menuLeft.value}px`,
}))

function onSelectionChange() {
  if (!props.containerRef) return
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0 || sel.isCollapsed) {
    scheduleHide()
    return
  }
  const range = sel.getRangeAt(0)
  if (!props.containerRef.contains(range.commonAncestorContainer)) {
    scheduleHide()
    return
  }
  const text = sel.toString().trim()
  if (text.length < MIN_SELECTION_LENGTH) {
    scheduleHide()
    return
  }
  selectionText.value = text.slice(0, MAX_SELECTION_LENGTH)
  positionMenu(range.getBoundingClientRect())
  showNow()
}

function positionMenu(rect: DOMRect) {
  const viewportW = window.innerWidth
  // Prefer placing the menu above the selection; if not enough room, place below.
  let top = rect.top - MENU_HEIGHT_ESTIMATE - 6
  if (top < VIEWPORT_PADDING) {
    top = rect.bottom + 6
  }
  let left = rect.left + rect.width / 2 - MENU_WIDTH_ESTIMATE / 2
  if (left < VIEWPORT_PADDING) left = VIEWPORT_PADDING
  if (left + MENU_WIDTH_ESTIMATE > viewportW - VIEWPORT_PADDING) {
    left = viewportW - MENU_WIDTH_ESTIMATE - VIEWPORT_PADDING
  }
  menuTop.value = top + window.scrollY
  menuLeft.value = left + window.scrollX
}

function showNow() {
  if (hideTimer !== null) {
    window.clearTimeout(hideTimer)
    hideTimer = null
  }
  if (!visible.value) {
    visible.value = true
  }
}

function scheduleHide() {
  if (hideTimer !== null) window.clearTimeout(hideTimer)
  hideTimer = window.setTimeout(() => {
    visible.value = false
    selectionText.value = ''
    hideTimer = null
  }, 120)
}

function onClick(action: InlineAIAction) {
  emit('ai', { action: action.id, text: selectionText.value })
  visible.value = false
  selectionText.value = ''
  window.getSelection()?.removeAllRanges()
}

watch(
  () => props.containerRef,
  (el, prev) => {
    if (prev && prev !== el) {
      prev.removeEventListener('mouseup', onSelectionChange)
      prev.removeEventListener('keyup', onSelectionChange)
    }
    if (el) {
      el.addEventListener('mouseup', onSelectionChange)
      el.addEventListener('keyup', onSelectionChange)
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  if (props.containerRef) {
    props.containerRef.removeEventListener('mouseup', onSelectionChange)
    props.containerRef.removeEventListener('keyup', onSelectionChange)
  }
  if (hideTimer !== null) window.clearTimeout(hideTimer)
})
</script>

<style scoped>
.wiki-inline-ai-menu {
  position: absolute;
  z-index: 60;
  display: flex;
  gap: 2px;
  padding: 4px;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
  font-size: 12px;
  white-space: nowrap;
}
.wiki-inline-ai-menu__btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border: 0;
  background: transparent;
  border-radius: 6px;
  cursor: pointer;
  color: var(--td-text-color-primary, #1f1f1f);
  transition: background-color 120ms ease;
}
.wiki-inline-ai-menu__btn:hover {
  background: var(--td-bg-color-container-hover, #f5f5f5);
}
.wiki-inline-ai-menu__btn:focus-visible {
  outline: 2px solid var(--td-brand-color, #1677ff);
  outline-offset: 1px;
}
.wiki-inline-ai-menu__label {
  font-size: 12px;
}
</style>
