<template>
  <div class="db-settings">
    <header class="db-settings-header">
      <h3>{{ t('databaseView.title') }}</h3>
      <p>{{ t('databaseView.desc') }}</p>
    </header>
    <div v-if="kbId" class="db-settings-body">
      <DatabaseView :knowledge-base-id="kbId" :can-write="true" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import DatabaseView from '@/components/database/DatabaseView.vue'

const route = useRoute()
const { t } = useI18n()

const kbId = computed<string>(() => {
  return (route.params.id as string) || (route.query.kb_id as string) || ''
})
</script>

<style scoped>
.db-settings { padding: 0; }
.db-settings-header { padding: 24px 24px 0; }
.db-settings-header h3 { margin: 0 0 8px; }
.db-settings-header p { color: var(--text-secondary, #666); font-size: 13px; margin: 0; }
.db-settings-body { padding: 16px 24px; }
</style>
