<template>
  <div class="connector-settings">
    <header class="connector-settings-header">
      <h3>{{ t('datasource.connector.title') }}</h3>
      <p class="connector-settings-desc">{{ descText }}</p>
    </header>
    <ConnectorManager />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ConnectorManager from '@/components/connector/ConnectorManager.vue'

const { t, locale } = useI18n()

// Static description so we don't need a new i18n key namespace.
// Localizes the headline via the existing datasource.connector.title.
const descText = computed(() => {
  // Pick the user's locale; fall back to English copy.
  if (locale.value === 'zh-CN') {
    return '连接外部数据源（Slack、邮箱、RSS、Confluence、Webhook），将内容直接摄入知识库。'
  }
  if (locale.value === 'ko-KR') {
    return '외부 소스(Slack, 이메일, RSS, Confluence, Webhook)를 연결해 지식베이스로 직접 가져옵니다.'
  }
  if (locale.value === 'ru-RU') {
    return 'Подключайте внешние источники (Slack, Email, RSS, Confluence, Webhook) и импортируйте содержимое напрямую в базу знаний.'
  }
  return 'Connect external sources (Slack, Email, RSS, Confluence, Webhooks) to ingest content directly into knowledge bases.'
})
</script>

<style scoped>
.connector-settings {
  padding: 24px;
}
.connector-settings-header h3 {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
}
.connector-settings-desc {
  margin: 0 0 24px;
  color: var(--text-secondary, #666);
  font-size: 14px;
  line-height: 1.5;
}
</style>
