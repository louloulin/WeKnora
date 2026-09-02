<!--
  CollabFormResponder.vue — public form-filling view (Tencent Docs 收集表 parity).

  Used by `/form/:token` route. Loads the form schema via the public
  `share/:token/download` route and submits responses via the public
  `POST /collaborative-docs/:id/responses?share_token=...` endpoint.

  Anonymous respondents land here without auth; the submitter_token
  pinned in localStorage de-dupes accidental re-submits.
-->
<template>
  <div class="collab-form-responder">
    <div v-if="loading" class="collab-form-responder__loading">
      加载表单中...
    </div>
    <div v-else-if="loadError" class="collab-form-responder__error">
      {{ loadError }}
    </div>
    <div v-else-if="questions.length > 0" class="collab-form-responder__shell">
      <header class="collab-form-responder__header">
        <h2>{{ title }}</h2>
        <p class="collab-form-responder__sub">{{ questions.length }} 个问题</p>
      </header>
      <div
        v-if="submitted"
        class="collab-form-responder__thanks"
        data-testid="responder-thanks"
      >
        <p>✅ 已提交，感谢您的填写！</p>
        <button type="button" @click="resetForm" data-testid="responder-reset">
          再填一次
        </button>
      </div>
      <form
        v-else
        class="collab-form-responder__form"
        @submit.prevent="onSubmit"
      >
        <div
          v-for="(q, idx) in questions"
          :key="q.id"
          class="collab-form-responder__item"
          :data-testid="`responder-q-${idx}`"
        >
          <label class="collab-form-responder__q-title">
            {{ q.title || `问题 ${idx + 1}` }}
            <span v-if="q.required" class="collab-form-responder__required"
              >*</span
            >
          </label>
          <textarea
            v-if="q.type === 'text' && q.multiline"
            v-model="answers[q.id]"
            class="collab-form-responder__textarea"
            :placeholder="q.placeholder || ''"
            :data-testid="`responder-q-${idx}-input`"
          />
          <input
            v-else-if="q.type === 'text'"
            v-model="answers[q.id]"
            type="text"
            class="collab-form-responder__text"
            :placeholder="q.placeholder || ''"
            :data-testid="`responder-q-${idx}-input`"
          />
          <div
            v-else-if="q.type === 'single'"
            class="collab-form-responder__single"
          >
            <label
              v-for="opt in q.options"
              :key="opt"
              class="collab-form-responder__radio"
            >
              <input
                type="radio"
                :name="`q-${q.id}`"
                :value="opt"
                v-model="answers[q.id]"
                :data-testid="`responder-q-${idx}-opt-${q.options.indexOf(opt)}`"
              />
              {{ opt }}
            </label>
          </div>
          <div
            v-else-if="q.type === 'multi'"
            class="collab-form-responder__multi"
          >
            <label
              v-for="opt in q.options"
              :key="opt"
              class="collab-form-responder__check"
            >
              <input
                type="checkbox"
                :value="opt"
                :checked="(answers[q.id] || []).includes(opt)"
                @change="
                  onMultiChange(
                    q.id,
                    opt,
                    ($event.target as HTMLInputElement).checked,
                  )
                "
                :data-testid="`responder-q-${idx}-opt-${q.options.indexOf(opt)}`"
              />
              {{ opt }}
            </label>
          </div>
          <div
            v-else-if="q.type === 'rating'"
            class="collab-form-responder__rating"
          >
            <button
              v-for="n in 5"
              :key="n"
              type="button"
              class="collab-form-responder__star"
              :class="{ active: Number(answers[q.id] || 0) >= n }"
              @click="answers[q.id] = n"
              :data-testid="`responder-q-${idx}-star-${n}`"
            >
              ★
            </button>
          </div>
          <input
            v-else-if="q.type === 'date'"
            v-model="answers[q.id]"
            type="date"
            class="collab-form-responder__date"
            :data-testid="`responder-q-${idx}-input`"
          />
        </div>
        <div class="collab-form-responder__submit-row">
          <button
            type="submit"
            class="collab-form-responder__submit"
            :disabled="submitting"
            data-testid="responder-submit"
          >
            {{ submitting ? "提交中..." : "提交" }}
          </button>
          <p
            v-if="submitError"
            class="collab-form-responder__error"
            data-testid="responder-error"
          >
            {{ submitError }}
          </p>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";

const props = defineProps<{
  token: string;
  apiBase?: string;
}>();

const loading = ref(true);
const loadError = ref<string | null>(null);
const submitError = ref<string | null>(null);
const submitting = ref(false);
const submitted = ref(false);
const docId = ref("");
const title = ref("表单");
const questions = ref<any[]>([]);
const answers = ref<any>({});

const apiBase = (props as any).apiBase || "/collaborative-docs";

interface Question {
  id: string;
  type: "text" | "single" | "multi" | "rating" | "date";
  title: string;
  required?: boolean;
  placeholder?: string;
  options?: string[];
  multiline?: boolean;
}

function loadToken(): string {
  let token = localStorage.getItem("wk_form_token");
  if (!token) {
    token =
      (crypto.randomUUID && crypto.randomUUID()) ||
      "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
        const r = (Math.random() * 16) | 0;
        const v = c === "x" ? r : (r & 0x3) | 0x8;
        return v.toString(16);
      });
    localStorage.setItem("wk_form_token", token);
  }
  return token;
}

async function loadSchema() {
  loading.value = true;
  loadError.value = null;
  try {
    const r = await fetch(
      `${apiBase}/share/${encodeURIComponent(props.token)}/form-schema`,
    );
    if (!r.ok) {
      loadError.value = `加载失败：HTTP ${r.status}`;
      return;
    }
    const envelope = await r.json();
    const parsed = envelope.data || envelope;
    questions.value = (parsed.questions || []).map((q: any) => ({
      ...q,
      id: q.id || `q${Math.random().toString(36).slice(2, 8)}`,
      options: Array.isArray(q.options) ? q.options : [],
    }));
    title.value = parsed.title || "表单";
    docId.value = parsed.doc_id || "";
    answers.value = {};
    if (!docId.value) loadError.value = "表单缺少文档 ID";
  } catch (e: any) {
    loadError.value = `解析失败：${e?.message || e}`;
  } finally {
    loading.value = false;
  }
}

async function onSubmit() {
  submitError.value = null;
  // Required-field validation
  for (const q of questions.value) {
    if (!q.required) continue;
    const v = answers.value[q.id];
    if (
      v === undefined ||
      v === null ||
      v === "" ||
      (Array.isArray(v) && v.length === 0)
    ) {
      submitError.value = `「${q.title || q.id}」是必填项`;
      return;
    }
  }
  submitting.value = true;
  try {
    if (!docId.value) {
      submitError.value = "表单缺少文档 ID，请刷新重试";
      submitting.value = false;
      return;
    }
    const token = loadToken();
    const r = await fetch(
      `${apiBase}/${docId.value}/responses?share_token=${encodeURIComponent(props.token)}`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          submitter_token: token,
          submitter_name: "匿名用户",
          answers: answers.value,
        }),
      },
    );
    if (!r.ok) {
      const errBody = await r.text().catch(() => "");
      submitError.value = `提交失败：HTTP ${r.status} ${errBody}`;
      return;
    }
    submitted.value = true;
  } catch (e: any) {
    submitError.value = `提交失败：${e?.message || e}`;
  } finally {
    submitting.value = false;
  }
}

function resetForm() {
  answers.value = {};
  submitted.value = false;
  submitError.value = null;
}

function onMultiChange(qid: string, opt: string, checked: boolean) {
  const current: string[] = (answers.value[qid] as string[]) || [];
  if (checked) {
    answers.value[qid] = [...current, opt];
  } else {
    answers.value[qid] = current.filter((x) => x !== opt);
  }
}

onMounted(loadSchema);
</script>

<style scoped>
.collab-form-responder {
  max-width: 720px;
  margin: 0 auto;
  padding: 24px;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
.collab-form-responder__loading,
.collab-form-responder__error {
  padding: 24px;
  border-radius: 8px;
  background: var(--app-surface-raised, #f6f8fa);
  color: var(--app-text-muted, #57606a);
  text-align: center;
}
.collab-form-responder__error {
  background: var(--app-error-bg, #fff1f0);
  color: var(--td-error-color-7, #cf222e);
}
.collab-form-responder__header {
  margin-bottom: 24px;
  border-bottom: 1px solid var(--app-border, #d0d7de);
  padding-bottom: 12px;
}
.collab-form-responder__header h2 {
  margin: 0 0 4px;
  font-size: 22px;
}
.collab-form-responder__sub {
  margin: 0;
  color: var(--app-text-muted, #57606a);
  font-size: 13px;
}
.collab-form-responder__item {
  margin-bottom: 20px;
}
.collab-form-responder__q-title {
  display: block;
  font-weight: 600;
  margin-bottom: 8px;
}
.collab-form-responder__required {
  color: #cf222e;
  margin-left: 4px;
}
.collab-form-responder__text,
.collab-form-responder__textarea,
.collab-form-responder__date {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid #d0d7de;
  border-radius: 6px;
  font-size: 14px;
  box-sizing: border-box;
}
.collab-form-responder__textarea {
  min-height: 80px;
}
.collab-form-responder__radio,
.collab-form-responder__check {
  display: block;
  margin-bottom: 6px;
}
.collab-form-responder__star {
  background: none;
  border: none;
  font-size: 28px;
  color: #d0d7de;
  cursor: pointer;
  padding: 0 4px;
}
.collab-form-responder__star.active {
  color: #f59e0b;
}
.collab-form-responder__submit-row {
  margin-top: 24px;
}
.collab-form-responder__submit {
  background: #0969da;
  color: white;
  border: none;
  padding: 10px 20px;
  border-radius: 6px;
  font-size: 14px;
  cursor: pointer;
}
.collab-form-responder__submit:disabled {
  background: #8c959f;
  cursor: not-allowed;
}
.collab-form-responder__thanks {
  background: #dafbe1;
  color: #1a7f37;
  padding: 24px;
  border-radius: 8px;
  text-align: center;
}
.collab-form-responder__thanks button {
  margin-top: 12px;
  background: var(--app-surface-raised, white);
  border: 1px solid #1a7f37;
  color: #1a7f37;
  padding: 8px 16px;
  border-radius: 6px;
  cursor: pointer;
}
</style>
