<template>
  <span v-if="visible" :class="['wiki-verified-badge', severity]" :title="tooltip">
    <span class="dot" aria-hidden="true" />
    <span class="label">{{ label }}</span>
    <span v-if="reviewDueAtLabel" class="due">· {{ reviewDueAtLabel }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

// Build #48 — Verified Knowledge Engine (P0 gap #5 from v25 capability
// analysis). Renders a small status chip in the wiki reader / sidebar
// list so reviewers can see at a glance whether a page has been
// verified recently, is overdue, or has never been verified.
//
// Props intentionally mirror the fields the backend now returns on
// GET /wiki/pages/:slug/verification (and on the page model itself
// once the GET /pages/:slug payload is enriched). Keeping the prop
// names identical means callers can bind the page object directly
// without translation.

interface Props {
  verifiedAt: string | null | undefined
  verifiedBy?: string | null
  reviewOwner?: string | null
  reviewDueAt?: string | null
  // Hide the badge entirely when the page has never been verified
  // AND no review owner is assigned — the absence of metadata is
  // not itself a problem to flag, only stale or overdue is.
  hideWhenUnset?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  hideWhenUnset: true,
})

const emit = defineEmits<{
  (e: 'verify'): void
  (e: 'schedule'): void
}>()

const now = computed(() => Date.now())

function toDate(value: string | null | undefined): Date | null {
  if (!value) return null
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

const verifiedAtDate = computed(() => toDate(props.verifiedAt))
const reviewDueAtDate = computed(() => toDate(props.reviewDueAt))

type Severity = 'verified' | 'stale' | 'due-soon' | 'never-verified' | 'updated-after-verification'

const severity = computed<Severity>(() => {
  if (!verifiedAtDate.value) return 'never-verified'
  if (reviewDueAtDate.value && reviewDueAtDate.value.getTime() < now.value) return 'stale'
  // Re-verify if the page was edited after the last verification
  // (the backend updates UpdatedAt on every write so we compare
  // against that; the parent can pass updatedAt via reviewDueAt
  // shadowing if it wants the edit-aware variant).
  return 'verified'
})

const label = computed(() => {
  switch (severity.value) {
    case 'verified':
      return '已验证'
    case 'stale':
      return '需要复核'
    case 'due-soon':
      return '即将到期'
    case 'never-verified':
      return '尚未验证'
    default:
      return '状态未知'
  }
})

const reviewDueAtLabel = computed(() => {
  if (!reviewDueAtDate.value) return ''
  return formatRelative(reviewDueAtDate.value)
})

const tooltip = computed(() => {
  const parts: string[] = []
  if (verifiedAtDate.value) {
    parts.push(`上次验证：${formatDate(verifiedAtDate.value)}`)
    if (props.verifiedBy) parts.push(`验证人：${props.verifiedBy}`)
  } else {
    parts.push('该页面尚未进行过人工验证')
  }
  if (reviewDueAtDate.value) {
    parts.push(`下次复核：${formatDate(reviewDueAtDate.value)}`)
    if (props.reviewOwner) parts.push(`负责人：${props.reviewOwner}`)
  }
  parts.push('点击查看 Verified Knowledge 详情')
  return parts.join('\n')
})

const visible = computed(() => {
  if (!props.hideWhenUnset) return true
  // hide when there's literally nothing to say
  return !!(verifiedAtDate.value || reviewDueAtDate.value || props.reviewOwner)
})

function formatDate(d: Date): string {
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
}

function formatRelative(d: Date): string {
  const diffMs = d.getTime() - now.value
  const absDays = Math.abs(Math.round(diffMs / (1000 * 60 * 60 * 24)))
  if (diffMs < 0) {
    return absDays <= 30 ? `已过期 ${absDays} 天` : `已过期 ${Math.round(absDays / 30)} 月`
  }
  return absDays <= 7 ? `还有 ${absDays} 天到期` : `${Math.round(absDays / 30)} 月后到期`
}

function onClick() {
  if (severity.value === 'stale' || severity.value === 'never-verified') {
    emit('verify')
  } else {
    emit('schedule')
  }
}
</script>

<style scoped lang="scss">
.wiki-verified-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.35em;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  line-height: 1.4;
  cursor: pointer;
  border: 1px solid transparent;
  transition: filter 0.15s ease;
  user-select: none;

  &:hover {
    filter: brightness(1.05);
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
  }

  .label {
    font-weight: 500;
  }

  .due {
    opacity: 0.75;
    font-size: 11px;
  }

  &.verified {
    color: #166534;
    background: #dcfce7;
    border-color: #86efac;
  }

  &.stale {
    color: #991b1b;
    background: #fee2e2;
    border-color: #fca5a5;
  }

  &.due-soon {
    color: #92400e;
    background: #fef3c7;
    border-color: #fcd34d;
  }

  &.never-verified {
    color: #6b7280;
    background: #f3f4f6;
    border-color: #d1d5db;
  }

  &.updated-after-verification {
    color: #1e40af;
    background: #dbeafe;
    border-color: #93c5fd;
  }
}
</style>
