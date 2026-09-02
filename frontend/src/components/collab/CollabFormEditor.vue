<!--
  CollabFormEditor — v0.7.73 FORM-kind collaborative editor.

  Architecture:
   1. Open: GET /collaborative-docs/:id/download (latest .form.json bytes)
            -> adapter parses JSON, seeds local questions[].
   2. Realtime: Y.Array<Y.Map<{ id, type, title, required, options }>>
            Two clients editing different questions converge via Yjs CRDT.
   3. Save: JSON.stringify(questions) -> POST .../upload (Blob file field).
-->
<template>
  <div class="collab-form-editor">
    <div class="collab-form-editor__toolbar">
      <span class="collab-form-editor__title">{{ title }}</span>
      <span class="collab-form-editor__kind">{{ kindLabel }}</span>
      <span
        class="collab-form-editor__connection"
        :class="{ connected: connected && !saveError }"
      >
        {{ connectionLabel }}
      </span>
      <span class="collab-form-editor__savetag" :class="savetagClass">
        {{ saveLabel }}
      </span>
      <button
        class="collab-form-editor__add-question"
        @click="addQuestion('text')"
        type="button"
      >
        + 文本题
      </button>
      <button
        class="collab-form-editor__add-question"
        @click="addQuestion('single')"
        type="button"
      >
        + 单选题
      </button>
      <button
        class="collab-form-editor__add-question"
        @click="addQuestion('multi')"
        type="button"
      >
        + 多选题
      </button>
      <button
        class="collab-form-editor__add-question"
        @click="addQuestion('rating')"
        type="button"
      >
        + 评分题
      </button>
      <button
        class="collab-form-editor__add-question"
        @click="addQuestion('date')"
        type="button"
      >
        + 日期题
      </button>
      <button
        class="collab-form-editor__export"
        @click="exportForm"
        type="button"
        :disabled="downloading"
      >
        {{ downloading ? "下载中..." : "下载 .form.json" }}
      </button>
      <button
        class="collab-form-editor__responses-btn"
        @click="responsesVisible = !responsesVisible"
        type="button"
        data-testid="form-responses-btn"
      >
        {{ responsesVisible ? "隐藏响应" : "查看响应" }}
      </button>
      <span class="collab-form-editor__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-form-editor__peer"
          :style="{ backgroundColor: p.color }"
          :title="p.displayName"
          >{{ initialOf(p.displayName) }}</span
        >
      </span>
    </div>

    <div v-if="loading" class="collab-form-editor__loading">
      加载收集表中...
    </div>
    <div v-else class="collab-form-editor__layout">
      <div class="collab-form-editor__panel collab-form-editor__panel--editor">
        <h3 class="collab-form-editor__panel-title">问题列表</h3>
        <ol class="collab-form-editor__list">
          <li
            v-for="(q, idx) in questions"
            :key="q.id"
            class="collab-form-editor__item"
          >
            <div class="collab-form-editor__item-head">
              <span class="collab-form-editor__num">第 {{ idx + 1 }} 题</span>
              <span class="collab-form-editor__type">{{
                typeLabel(q.type)
              }}</span>
              <span class="collab-form-editor__required">
                <label>
                  <input
                    type="checkbox"
                    :checked="q.required"
                    @change="
                      setRequired(
                        idx,
                        ($event.target as HTMLInputElement).checked,
                      )
                    "
                  />
                  必填
                </label>
              </span>
              <span class="collab-form-editor__actions">
                <button
                  class="collab-form-editor__iconbtn"
                  @click="moveQuestion(idx, idx - 1)"
                  :disabled="idx === 0"
                  title="上移"
                >
                  ↑
                </button>
                <button
                  class="collab-form-editor__iconbtn"
                  @click="moveQuestion(idx, idx + 1)"
                  :disabled="idx === questions.length - 1"
                  title="下移"
                >
                  ↓
                </button>
                <button
                  class="collab-form-editor__iconbtn danger"
                  @click="deleteQuestion(idx)"
                  :disabled="questions.length <= 1"
                  title="删除"
                >
                  ×
                </button>
              </span>
            </div>
            <input
              class="collab-form-editor__question-title"
              :value="q.title"
              placeholder="题干（必填）"
              @input="
                updateTitle(idx, ($event.target as HTMLInputElement).value)
              "
            />
            <!-- Options for single / multi -->
            <div
              v-if="q.type === 'single' || q.type === 'multi'"
              class="collab-form-editor__options"
            >
              <ol class="collab-form-editor__options-list">
                <li v-for="(opt, oi) in q.options || []" :key="oi">
                  <span class="collab-form-editor__opt-marker">
                    <template v-if="q.type === 'single'">○</template>
                    <template v-else>☐</template>
                  </span>
                  <input
                    class="collab-form-editor__option-input"
                    :value="opt"
                    :placeholder="`选项 ${oi + 1}`"
                    @input="
                      updateOption(
                        idx,
                        oi,
                        ($event.target as HTMLInputElement).value,
                      )
                    "
                  />
                  <button
                    class="collab-form-editor__iconbtn"
                    @click="removeOption(idx, oi)"
                    type="button"
                    title="删除选项"
                  >
                    ×
                  </button>
                </li>
              </ol>
              <button
                class="collab-form-editor__opt-add"
                @click="addOption(idx)"
                type="button"
              >
                + 添加选项
              </button>
            </div>
            <!-- Preview for text -->
            <div
              v-else-if="q.type === 'text'"
              class="collab-form-editor__preview"
            >
              <textarea
                readonly
                placeholder="（用户填写区预览）"
                rows="2"
              ></textarea>
            </div>
            <!-- Preview for rating -->
            <div
              v-else-if="q.type === 'rating'"
              class="collab-form-editor__preview"
            >
              <span class="collab-form-editor__star">★</span>
              <span class="collab-form-editor__star">★</span>
              <span class="collab-form-editor__star">★</span>
              <span class="collab-form-editor__star">★</span>
              <span class="collab-form-editor__star">★</span>
            </div>
            <!-- Preview for date -->
            <div
              v-else-if="q.type === 'date'"
              class="collab-form-editor__preview"
            >
              <input type="date" readonly />
            </div>
          </li>
        </ol>
      </div>

      <div class="collab-form-editor__panel collab-form-editor__panel--preview">
        <h3 class="collab-form-editor__panel-title">实时预览</h3>
        <div class="collab-form-editor__preview-form">
          <div class="collab-form-editor__preview-header">
            <h2 class="collab-form-editor__preview-title">
              {{ title || "（无标题）" }}
            </h2>
          </div>
          <div
            v-for="(q, idx) in questions"
            :key="q.id"
            class="collab-form-editor__preview-item"
          >
            <label class="collab-form-editor__preview-label">
              {{ idx + 1 }}. {{ q.title || "（未命名问题）" }}
              <span v-if="q.required" class="collab-form-editor__required-mark"
                >*</span
              >
            </label>
            <div v-if="q.type === 'text'">
              <textarea readonly placeholder="用户输入..." rows="2"></textarea>
            </div>
            <div
              v-else-if="q.type === 'single'"
              class="collab-form-editor__preview-options"
            >
              <div
                v-for="(opt, oi) in q.options || []"
                :key="oi"
                class="collab-form-editor__preview-option"
              >
                <input type="radio" :name="`pv-${q.id}`" disabled />
                {{ opt || `选项 ${oi + 1}` }}
              </div>
            </div>
            <div
              v-else-if="q.type === 'multi'"
              class="collab-form-editor__preview-options"
            >
              <div
                v-for="(opt, oi) in q.options || []"
                :key="oi"
                class="collab-form-editor__preview-option"
              >
                <input type="checkbox" disabled />
                {{ opt || `选项 ${oi + 1}` }}
              </div>
            </div>
            <div
              v-else-if="q.type === 'rating'"
              class="collab-form-editor__preview-rating"
            >
              <span class="collab-form-editor__star">★</span>
              <span class="collab-form-editor__star">★</span>
              <span class="collab-form-editor__star">★</span>
              <span class="collab-form-editor__star">★</span>
              <span class="collab-form-editor__star">★</span>
            </div>
            <div v-else-if="q.type === 'date'">
              <input type="date" readonly />
            </div>
          </div>
        </div>
      </div>
    </div>

    <CollabFormResponsesPanel
      v-if="responsesVisible"
      :doc-id="props.docId"
      :token="props.token"
      @close="responsesVisible = false"
    />

    <p v-if="error || saveError" class="collab-form-editor__error">
      {{ saveError || error }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import * as Y from "yjs";
import CollabFormResponsesPanel from "@/components/collab/CollabFormResponsesPanel.vue";
import { MessagePlugin } from "tdesign-vue-next";
import { useYjsCollabDoc } from "@/composables/useYjsCollabDoc";
import { downloadCollabDocBytes, uploadCollabDocBytes } from "@/api/collabDoc";

type QuestionType = "text" | "single" | "multi" | "rating" | "date";

interface Question {
  id: string;
  type: QuestionType;
  title: string;
  required: boolean;
  options: string[];
}

const props = defineProps<{
  docId: string;
  title: string;
  token: string;
  displayName: string;
}>();

const responsesVisible = ref(false);

const connected = ref(false);
const peers = ref<
  Array<{ clientId: number; displayName: string; color: string }>
>([]);
const error = ref<string | null>(null);
const questions = ref<Question[]>([
  {
    id: "q-default-1",
    type: "text",
    title: "请输入你的反馈",
    required: false,
    options: [],
  },
]);
const loading = ref(false);
const downloading = ref(false);
const saveLabel = ref("未修改");
const saveError = ref<string | null>(null);

let handle: ReturnType<typeof useYjsCollabDoc> | null = null;
let yarr: Y.Array<Y.Map<unknown>> | null = null;
let saveTimer: ReturnType<typeof setTimeout> | null = null;

const connectionLabel = computed(() => {
  if (saveError.value) return `保存错误：${saveError.value}`;
  if (connected.value) return "已连接";
  return "连接中...";
});

const kindLabel = computed(() => "收集表 (.form.json)");

const savetagClass = computed(() => ({
  dirty: saveLabel.value === "有未保存的修改",
  saving: saveLabel.value === "保存中...",
}));

const initialOf = (name: string) =>
  (name || "?").trim().slice(0, 1).toUpperCase();

const typeLabel = (t: QuestionType): string => {
  switch (t) {
    case "text":
      return "文本";
    case "single":
      return "单选";
    case "multi":
      return "多选";
    case "rating":
      return "评分";
    case "date":
      return "日期";
  }
};

const questionToObj = (q: Question): Record<string, unknown> => ({
  id: q.id,
  type: q.type,
  title: q.title,
  required: q.required,
  options: q.options || [],
});

const objToQuestion = (raw: Record<string, unknown>): Question => ({
  id: String(
    raw.id || `q-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
  ),
  type: (["text", "single", "multi", "rating", "date"].includes(
    String(raw.type),
  )
    ? String(raw.type)
    : "text") as QuestionType,
  title: String(raw.title || ""),
  required: Boolean(raw.required),
  options: Array.isArray(raw.options)
    ? (raw.options as unknown[]).map((o) => String(o))
    : [],
});

const genId = () => `q-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

const downloadAsBytes = async (): Promise<Uint8Array | null> => {
  try {
    const blob = await downloadCollabDocBytes(props.docId);
    const buf = await blob.arrayBuffer();
    return new Uint8Array(buf);
  } catch {
    return null;
  }
};

const setup = async () => {
  if (!props.docId || !props.token) return;
  handle = useYjsCollabDoc({
    docId: props.docId,
    token: props.token,
    displayName: props.displayName,
  });
  connected.value = Boolean(handle.connected.value);
  peers.value = (handle.peers.value ?? []) as Array<{
    clientId: number;
    displayName: string;
    color: string;
  }>;
  error.value = (handle.error.value ?? null) as string | null;
  watch(handle.connected, (v) => (connected.value = Boolean(v)));
  watch(
    handle.peers,
    (v) =>
      (peers.value = (v ?? []) as Array<{
        clientId: number;
        displayName: string;
        color: string;
      }>),
  );
  watch(handle.error, (v) => (error.value = (v ?? null) as string | null));

  yarr = handle.ydoc.getArray<Y.Map<unknown>>("form:questions");

  loading.value = true;
  try {
    const bytes = await downloadAsBytes();
    if (bytes) {
      try {
        const text = new TextDecoder("utf-8").decode(bytes);
        const parsed = JSON.parse(text);
        if (parsed && Array.isArray(parsed.questions)) {
          questions.value = parsed.questions.map((q: any) => objToQuestion(q));
        }
      } catch (e: any) {
        error.value = `解析失败：${e?.message || e}`;
      }
    }
  } catch (e: any) {
    error.value = `加载失败：${e?.message || e}`;
  } finally {
    loading.value = false;
  }
  syncFromY();
  yarr.observeDeep(() => syncFromY());
};

const syncFromY = () => {
  if (!yarr) return;
  const remote = yarr
    .toArray()
    .map((m) => objToQuestion(m.toJSON() as Record<string, unknown>));
  if (remote.length === 0 && handle && yarr) {
    const localHandle = handle;
    const localArr = yarr;
    localHandle.ydoc.transact(() => {
      const seed = new Y.Map<unknown>();
      Object.entries(questionToObj(questions.value[0])).forEach(([k, v]) =>
        seed.set(k, v),
      );
      localArr.push([seed]);
    });
    return;
  }
  questions.value = remote;
};

const updateTitle = (idx: number, value: string) => {
  questions.value[idx].title = value;
  applyQuestionPatch(idx);
};

const setRequired = (idx: number, value: boolean) => {
  questions.value[idx].required = value;
  applyQuestionPatch(idx);
};

const updateOption = (idx: number, oi: number, value: string) => {
  questions.value[idx].options[oi] = value;
  applyQuestionPatch(idx);
};

const addOption = (idx: number) => {
  questions.value[idx].options.push("");
  applyQuestionPatch(idx);
};

const removeOption = (idx: number, oi: number) => {
  questions.value[idx].options.splice(oi, 1);
  applyQuestionPatch(idx);
};

const applyQuestionPatch = (idx: number) => {
  if (!yarr || !handle) return;
  handle.ydoc.transact(() => {
    const yq = yarr!.get(idx) as Y.Map<unknown>;
    const obj = questionToObj(questions.value[idx]);
    Object.entries(obj).forEach(([k, v]) => yq.set(k, v));
  });
  scheduleSave();
};

const addQuestion = (type: QuestionType) => {
  const fresh: Question = {
    id: genId(),
    type,
    title: "",
    required: false,
    options: type === "single" || type === "multi" ? ["", ""] : [],
  };
  questions.value.push(fresh);
  if (!yarr || !handle) return;
  handle.ydoc.transact(() => {
    const m = new Y.Map<unknown>();
    Object.entries(questionToObj(fresh)).forEach(([k, v]) => m.set(k, v));
    yarr!.push([m]);
  });
  scheduleSave();
};

const moveQuestion = (from: number, to: number) => {
  if (to < 0 || to >= questions.value.length) return;
  if (!yarr || !handle) return;
  const q = questions.value[from];
  if (!q) return;
  const next = questions.value.slice();
  next.splice(from, 1);
  next.splice(to, 0, q);
  questions.value = next;
  handle.ydoc.transact(() => {
    yarr!.delete(from, 1);
    const cloned = new Y.Map<unknown>();
    Object.entries(questionToObj(q)).forEach(([k, v]) => cloned.set(k, v));
    yarr!.insert(to, [cloned]);
  });
  scheduleSave();
};

const deleteQuestion = (idx: number) => {
  if (questions.value.length <= 1) return;
  if (!yarr || !handle) return;
  questions.value.splice(idx, 1);
  handle.ydoc.transact(() => yarr!.delete(idx, 1));
  scheduleSave();
};

const scheduleSave = () => {
  saveLabel.value = "有未保存的修改";
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(() => flushSave(), 1500);
};

const flushSave = async () => {
  if (saveTimer) {
    clearTimeout(saveTimer);
    saveTimer = null;
  }
  saveLabel.value = "保存中...";
  try {
    const payload = JSON.stringify(
      { doc_id: props.docId, questions: questions.value },
      null,
      2,
    );
    const bytes = new TextEncoder().encode(payload);
    await uploadCollabDocBytes(
      props.docId,
      bytes,
      `${props.title || "collab-form"}.form.json`,
    );
    saveLabel.value = "已保存";
    saveError.value = null;
    setTimeout(() => {
      if (saveLabel.value === "已保存") saveLabel.value = "未修改";
    }, 1500);
  } catch (e: any) {
    saveError.value = e?.message || String(e);
    saveLabel.value = "保存失败";
  }
};

const exportForm = async () => {
  downloading.value = true;
  try {
    const payload = JSON.stringify(
      { doc_id: props.docId, questions: questions.value },
      null,
      2,
    );
    const bytes = new TextEncoder().encode(payload);
    const ab = bytes.buffer.slice(
      bytes.byteOffset,
      bytes.byteOffset + bytes.byteLength,
    ) as ArrayBuffer;
    const blob = new Blob([ab], { type: "application/json" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${props.title || "collab-form"}.form.json`;
    a.click();
    URL.revokeObjectURL(a.href);
    MessagePlugin.success("已下载 .form.json");
  } catch (e: any) {
    MessagePlugin.error(`下载失败：${e?.message || e}`);
  } finally {
    downloading.value = false;
  }
};

setup();
watch(
  () => props.docId,
  () => {
    // Reset and reconnect
    if (handle) handle.destroy();
    handle = null;
    yarr = null;
    questions.value = [
      { id: genId(), type: "text", title: "", required: false, options: [] },
    ];
    setup();
  },
);
onBeforeUnmount(() => {
  if (saveTimer) {
    clearTimeout(saveTimer);
    saveTimer = null;
    void flushSave();
  }
  if (handle) handle.destroy();
  handle = null;
});
</script>

<style scoped>
.collab-form-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 600px;
  background: var(--app-page-bg);
  color: var(--app-text);
}
.collab-form-editor__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
  background: var(--app-surface-raised);
  flex-wrap: wrap;
}
.collab-form-editor__title {
  font-weight: 600;
  font-size: 15px;
}
.collab-form-editor__kind {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--td-brand-color-light, #f0f7ff);
  color: var(--td-brand-color, #0052d9);
  font-size: 12px;
}
.collab-form-editor__connection {
  font-size: 12px;
  padding: 2px 6px;
  border-radius: 4px;
  background: #fff7e6;
  color: #d48806;
}
.collab-form-editor__connection.connected {
  background: #f6ffed;
  color: #389e0d;
}
.collab-form-editor__savetag {
  font-size: 12px;
  padding: 2px 6px;
  border-radius: 4px;
  background: #fafafa;
  color: #666;
}
.collab-form-editor__savetag.dirty {
  background: #fff7e6;
  color: #d48806;
}
.collab-form-editor__savetag.saving {
  background: #e6f4ff;
  color: #1677ff;
}
.collab-form-editor__add-question {
  padding: 4px 8px;
  border: 1px solid var(--td-component-stroke, #d9d9d9);
  border-radius: 4px;
  background: var(--app-control-bg);
  color: var(--app-text);
  cursor: pointer;
  font-size: 12px;
}
.collab-form-editor__add-question:hover {
  background: var(--td-brand-color-light, #f0f7ff);
}
.collab-form-editor__export {
  padding: 4px 8px;
  border: 1px solid var(--td-component-stroke, #d9d9d9);
  border-radius: 4px;
  background: var(--app-control-bg);
  color: var(--app-text);
  cursor: pointer;
  font-size: 12px;
}
.collab-form-editor__peers {
  display: flex;
  gap: 2px;
  margin-left: auto;
}
.collab-form-editor__peer {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  color: white;
  font-size: 12px;
  font-weight: 600;
}
.collab-form-editor__loading {
  padding: 24px;
  text-align: center;
  color: #999;
}
.collab-form-editor__layout {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  flex: 1;
  padding: 16px;
  overflow: hidden;
  background: var(--app-page-bg);
}
.collab-form-editor__panel {
  background: var(--app-surface-bg);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  padding: 16px;
  overflow-y: auto;
}
.collab-form-editor__panel-title {
  margin: 0 0 12px 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text);
}
.collab-form-editor__list {
  padding: 0;
  margin: 0;
  list-style: none;
}
.collab-form-editor__item {
  padding: 12px;
  border: 1px solid var(--app-border);
  border-radius: 6px;
  margin-bottom: 12px;
  background: var(--app-surface-raised);
}
.collab-form-editor__item-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.collab-form-editor__num {
  font-weight: 600;
  font-size: 13px;
  color: var(--app-text-muted);
}
.collab-form-editor__type {
  padding: 1px 6px;
  background: var(--td-brand-color-light, #e6f4ff);
  color: var(--td-brand-color, #1677ff);
  border-radius: 3px;
  font-size: 11px;
}
.collab-form-editor__required label {
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
}
.collab-form-editor__actions {
  margin-left: auto;
  display: flex;
  gap: 4px;
}
.collab-form-editor__iconbtn {
  padding: 2px 6px;
  border: 1px solid var(--app-border-strong);
  border-radius: 3px;
  background: var(--app-control-bg);
  color: var(--app-text);
  cursor: pointer;
  font-size: 11px;
}
.collab-form-editor__iconbtn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.collab-form-editor__iconbtn.danger {
  color: #ff4d4f;
}
.collab-form-editor__question-title {
  width: 100%;
  padding: 6px 8px;
  border: 1px solid var(--app-border-strong);
  border-radius: 4px;
  font-size: 14px;
  box-sizing: border-box;
}
.collab-form-editor__options {
  margin-top: 8px;
}
.collab-form-editor__options-list {
  padding: 0;
  margin: 0 0 6px 0;
  list-style: none;
}
.collab-form-editor__options-list li {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}
.collab-form-editor__opt-marker {
  color: #999;
}
.collab-form-editor__option-input {
  flex: 1;
  padding: 4px 6px;
  border: 1px solid var(--app-border-strong);
  border-radius: 3px;
  font-size: 13px;
}
.collab-form-editor__opt-add {
  font-size: 12px;
  color: var(--td-brand-color, #1677ff);
  background: none;
  border: 1px dashed var(--app-border-strong);
  border-radius: 3px;
  padding: 2px 8px;
  cursor: pointer;
}
.collab-form-editor__preview {
  margin-top: 8px;
  opacity: 0.7;
}
.collab-form-editor__preview textarea {
  width: 100%;
  padding: 4px 6px;
  border: 1px dashed var(--app-border-strong);
  border-radius: 3px;
  font-size: 13px;
  box-sizing: border-box;
  resize: vertical;
}
.collab-form-editor__star {
  color: #faad14;
  font-size: 18px;
  margin-right: 2px;
}
.collab-form-editor__preview-form {
  padding: 12px;
  background: var(--app-surface-raised);
  border-radius: 4px;
}
.collab-form-editor__preview-header {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--app-border);
}
.collab-form-editor__preview-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--app-text);
}
.collab-form-editor__preview-item {
  margin-bottom: 16px;
}
.collab-form-editor__preview-label {
  display: block;
  margin-bottom: 6px;
  font-weight: 500;
  color: var(--app-text);
  font-size: 14px;
}
.collab-form-editor__required-mark {
  color: #ff4d4f;
  margin-left: 2px;
}
.collab-form-editor__preview-options {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.collab-form-editor__preview-option {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}
.collab-form-editor__preview-rating {
  font-size: 18px;
}
.collab-form-editor__error {
  padding: 8px 12px;
  background: #fff2f0;
  color: #ff4d4f;
  border-radius: 4px;
  margin: 8px 16px 0;
}
</style>
