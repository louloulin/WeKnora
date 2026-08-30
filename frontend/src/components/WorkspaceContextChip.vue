<template>
  <div v-if="visible" class="workspace-context-chip">
    <t-tooltip :content="tooltipContent" placement="bottom" :show-arrow="false">
      <button
        type="button"
        class="chip-row"
        :aria-label="tooltipContent"
        @click="$emit('switch')"
      >
        <span class="chip-tag chip-tag--tenant">
          <t-icon name="root-list" size="12px" />
          <span class="chip-name">{{ tenantName }}</span>
        </span>
        <span v-if="roleLabel" class="chip-tag" :class="`chip-tag--${roleVariant}`">
          <t-icon v-if="roleIconName" :name="roleIconName" size="12px" />
          <span>{{ roleLabel }}</span>
        </span>
      </button>
    </t-tooltip>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useRoleLabel } from '@/composables/useRoleLabel'

defineEmits<{
  (e: 'switch'): void
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const { formatRole, roleIcon } = useRoleLabel()

/**
 * Show the chip whenever we have a tenant context. The chip is the single
 * place that announces "you are acting as X in workspace Y" — without it
 * the rest of the UI silently assumes the user knows their scope, which
 * after a tenant switch or peer-tenant invite is rarely true.
 */
const visible = computed(() => Boolean(authStore.tenant?.name))

const tenantName = computed(() => authStore.tenant?.name || t('tenant.unknown'))

const currentRole = computed(() => {
  const activeId = authStore.selectedTenantId
  if (activeId != null) {
    const membership = (authStore.memberships || []).find(
      (m) => String(m.tenant_id) === String(activeId),
    )
    if (membership?.role) return membership.role
  }
  // Fall back to the first membership when the active tenant isn't
  // represented yet (e.g. immediately after login, before any switch).
  const first = (authStore.memberships || [])[0]
  return first?.role || null
})

const roleLabel = computed(() => formatRole(currentRole.value))
const roleIconName = computed(() => roleIcon(currentRole.value))

const roleVariant = computed(() => {
  switch (currentRole.value) {
    case 'owner':
      return 'danger'
    case 'admin':
      return 'warning'
    case 'contributor':
      return 'primary'
    default:
      return 'default'
  }
})

const tooltipContent = computed(() => {
  if (!roleLabel.value) {
    return t('workspaceContext.tooltipNoRole', { tenant: tenantName.value })
  }
  return t('workspaceContext.tooltipWithRole', {
    tenant: tenantName.value,
    role: roleLabel.value,
  })
})
</script>

<style scoped>
.workspace-context-chip {
  display: inline-flex;
  align-items: center;
}

.chip-row {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 999px;
  background: var(--td-bg-color-container);
  cursor: pointer;
  font-size: 12px;
  line-height: 18px;
  color: var(--td-text-color-primary);
  transition: background 0.15s ease, border-color 0.15s ease;
  outline: none;
}

.chip-row:hover {
  background: var(--td-bg-color-container-hover);
  border-color: var(--td-brand-color);
}

.chip-row:focus-visible {
  outline: 2px solid var(--td-brand-color);
  outline-offset: 2px;
}

.chip-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.chip-name {
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chip-tag--tenant {
  color: var(--td-text-color-primary);
}

.chip-tag--danger {
  color: var(--td-error-color);
}

.chip-tag--warning {
  color: var(--td-warning-color);
}

.chip-tag--primary {
  color: var(--td-brand-color);
}

.chip-tag--default {
  color: var(--td-text-color-secondary);
}
</style>
