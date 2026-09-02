<!--
  CollabFormResponsesPanel.vue — owner-side form response viewer.

  Three views: list of submissions, per-question summary, CSV export.
  Mounted as a modal from CollabFormEditor.vue.
-->
<template>
  <div class="collab-form-responses" data-testid="form-responses-panel">
    <div class="collab-form-responses__header">
      <h3>响应收集</h3>
      <div class="collab-form-responses__tabs">
        <button
          :class="{ active: tab === 'list' }"
          @click="onTab('list')"
          data-testid="responses-tab-list"
        >
          列表 ({{ list.length }})
        </button>
        <button
          :class="{ active: tab === 'summary' }"
          @click="onTab('summary')"
          data-testid="responses-tab-summary"
        >
          统计
        </button>
        <button
          :class="{ active: tab === 'export' }"
          @click="onTab('export')"
          data-testid="responses-tab-export"
        >
          导出
        </button>
      </div>
      <button class="collab-form-responses__close" @click="$emit('close')">
        ×
      </button>
    </div>
    <div v-if="loading" class="collab-form-responses__loading">加载中...</div>
    <div v-else-if="error" class="collab-form-responses__error">
      {{ error }}
    </div>
    <div
      v-else-if="tab === 'list'"
      class="collab-form-responses__list"
      data-testid="responses-list"
    >
      <p v-if="!list.length" class="collab-form-responses__empty">暂无响应</p>
      <table v-else>
        <thead>
          <tr>
            <th>#</th>
            <th>提交者</th>
            <th>答案预览</th>
            <th>时间</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="r in list"
            :key="r.id"
            :data-testid="`response-row-${r.id}`"
          >
            <td>{{ r.id }}</td>
            <td>{{ r.submitter_name || "匿名" }}</td>
            <td class="collab-form-responses__answers">
              {{ previewAnswers(r.answers) }}
            </td>
            <td>{{ formatTime(r.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div
      v-else-if="tab === 'summary'"
      class="collab-form-responses__summary"
      data-testid="responses-summary"
    >
      <p>
        共 <strong>{{ summary?.total || 0 }}</strong> 条响应
      </p>
      <div
        v-for="q in summary?.by_question || []"
        :key="q.question_id"
        class="collab-form-responses__qsummary"
      >
        <h4>
          {{ q.question_title || q.question_id }}
          <small>({{ q.question_type }})</small>
        </h4>
        <ul v-if="q.counts">
          <li v-for="(c, k) in q.counts" :key="k">{{ k }}: {{ c }}</li>
        </ul>
        <ul v-if="q.latest_sample?.length">
          <li v-for="(s, i) in q.latest_sample" :key="i">“{{ s }}”</li>
        </ul>
      </div>
    </div>
    <div
      v-else-if="tab === 'export'"
      class="collab-form-responses__export"
      data-testid="responses-export"
    >
      <p>导出全部响应为 CSV：</p>
      <button
        type="button"
        class="collab-form-responses__csv-btn"
        @click="downloadCsv"
        :disabled="exporting"
        data-testid="export-csv-link"
      >
        {{ exporting ? "导出中..." : "下载 CSV" }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import {
  getFormResponses,
  getFormResponseSummary,
  type FormResponse,
  type FormResponseSummary,
} from "@/api/collabDoc";

const props = defineProps<{
  docId: string;
  token: string;
}>();
defineEmits<{ (e: "close"): void }>();

const tab = ref<"list" | "summary" | "export">("list");
const loading = ref(true);
const error = ref<string | null>(null);
const list = ref<FormResponse[]>([]);
const summary = ref<FormResponseSummary | null>(null);

const exporting = ref(false);

async function loadList() {
  loading.value = true;
  error.value = null;
  try {
    const r = await getFormResponses(props.docId, { limit: 200 });
    list.value = r.items || [];
  } catch (e: any) {
    error.value = `加载响应失败：${e?.message || e}`;
  } finally {
    loading.value = false;
  }
}

async function loadSummary() {
  loading.value = true;
  error.value = null;
  try {
    summary.value = await getFormResponseSummary(props.docId);
  } catch (e: any) {
    error.value = `加载统计失败：${e?.message || e}`;
  } finally {
    loading.value = false;
  }
}

async function onTab(name: "list" | "summary" | "export") {
  tab.value = name;
  if (name === "list" && !list.value.length) await loadList();
  if (name === "summary" && !summary.value) await loadSummary();
}

async function downloadCsv() {
  exporting.value = true;
  try {
    const jwt = localStorage.getItem("weknora_token") || props.token;
    const response = await fetch(
      `/collaborative-docs/${props.docId}/responses/export.csv`,
      {
        headers: jwt ? { Authorization: `Bearer ${jwt}` } : {},
      },
    );
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `form-${props.docId}.csv`;
    anchor.click();
    URL.revokeObjectURL(url);
  } catch (e: any) {
    error.value = `导出失败：${e?.message || e}`;
  } finally {
    exporting.value = false;
  }
}

// Initial load
onMounted(async () => {
  await Promise.all([loadList(), loadSummary()]);
});

function previewAnswers(json: string | Record<string, unknown>): string {
  if (typeof json === "string") {
    try {
      json = JSON.parse(json);
    } catch {
      return json.slice(0, 50);
    }
  }
  const parts: string[] = [];
  for (const [k, v] of Object.entries(json)) {
    if (Array.isArray(v)) parts.push(`${k}: [${v.join(", ")}]`);
    else parts.push(`${k}: ${String(v).slice(0, 30)}`);
  }
  return parts.slice(0, 3).join(" | ").slice(0, 80);
}

function formatTime(s: string): string {
  try {
    const d = new Date(s);
    return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours()}:${String(d.getMinutes()).padStart(2, "0")}`;
  } catch {
    return s;
  }
}
</script>

<style scoped>
.collab-form-responses {
  background: var(--app-surface-bg);
  border: 1px solid var(--app-border);
  border-radius: 8px;
  padding: 16px;
  margin-top: 12px;
}
.collab-form-responses__header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--app-border);
  padding-bottom: 8px;
}
.collab-form-responses__header h3 {
  margin: 0;
  font-size: 16px;
  flex: 1;
}
.collab-form-responses__tabs {
  display: flex;
  gap: 4px;
}
.collab-form-responses__tabs button {
  background: none;
  border: 1px solid var(--app-border);
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
}
.collab-form-responses__tabs button.active {
  background: var(--app-brand);
  color: white;
  border-color: var(--app-brand);
}
.collab-form-responses__close {
  background: none;
  border: none;
  font-size: 20px;
  cursor: pointer;
}
.collab-form-responses__list table {
  width: 100%;
  border-collapse: collapse;
}
.collab-form-responses__list th,
.collab-form-responses__list td {
  text-align: left;
  padding: 6px 8px;
  border-bottom: 1px solid var(--app-border);
  font-size: 13px;
}
.collab-form-responses__answers {
  max-width: 360px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.collab-form-responses__qsummary {
  margin-bottom: 12px;
}
.collab-form-responses__qsummary h4 {
  margin: 0 0 4px;
  font-size: 14px;
}
.collab-form-responses__qsummary h4 small {
  color: var(--app-text-muted);
  font-weight: normal;
}
.collab-form-responses__csv-btn {
  background: var(--app-brand);
  color: white;
  border: none;
  padding: 8px 16px;
  border-radius: 6px;
  cursor: pointer;
}
.collab-form-responses__empty {
  color: var(--app-text-muted);
  text-align: center;
  padding: 24px;
}
</style>
