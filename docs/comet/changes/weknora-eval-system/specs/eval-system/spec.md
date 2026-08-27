# B31 — Eval System · Spec

> Comet-Native change · branch `lumos0826`
> spec.md · 跟随 brief.md;验收 / 改动清单 / 验证命令全在这

---

## 1. 范围一句话

在 WeKnora 现有 `/api/v1/evaluation` 入口基础上扩展成完整 eval 体系:**持久化 dataset 库** + **LLM-as-judge 自动评分** + **三维度(事实性 / 引用正确性 / 反思必要性)** + **badcase 库** + **审计联动** + **Admin 后台最小集**。

---

## 2. 改动清单(实际文件,按依赖序)

### 2.1 Backend

| # | 路径 | 操作 | 备注 |
| --- | --- | --- | --- |
| B01 | `migrations/000103_eval_system.up.sql` | NEW | 4 张表 eval_datasets / eval_dataset_qa / eval_runs / eval_run_results / eval_badcases(实际是 5 张) |
| B02 | `migrations/000103_eval_system.down.sql` | NEW | 对称 drop |
| B03 | `internal/types/eval_dataset.go` | NEW | `EvalDataset`,`EvalDatasetQA`,`EvalRun`,`EvalRunResult`,`EvalBadcase`,`EvalSeverity`,`EvalRunStatus`,`EvalBadcaseStatus`,`EvalBadcaseSource`,JSON tag 完整 |
| B04 | `internal/types/audit_log.go` | MOD | 新增 5 个 AuditAction:`eval.run_started / eval.run_completed / eval.badcase_flagged / eval.dataset_updated / eval.run_reviewed` |
| B05 | `internal/types/interfaces/eval.go` | NEW | `EvalDatasetService`,`EvalRunService`,`EvalBadcaseService` 三个接口 |
| B06 | `internal/application/service/eval_dataset.go` | NEW | Repo 实现 + CRUD + JSON 上传 + fixture 导入 |
| B07 | `internal/application/service/eval_runner.go` | NEW | 异步 runner;沿用 `errgroup + GOMAXPROCS-1`;扩展 `HookMetric` 读 CitationIndex + REFLECTION events |
| B08 | `internal/eval/judge/llm_judge.go` | NEW | `LLMJudge.Score` 调用 chat model;JSON 解析容错 |
| B09 | `internal/eval/judge/prompts.go` | NEW | 默认 prompt 模板(`v1`);可被 eval_runs.judge_prompt_version 锁定 |
| B10 | `internal/eval/judge/citation_fidelity.go` | NEW | 纯函数:`ComputeCitationFidelity(citations, top_k_passages) float64` |
| B11 | `internal/eval/judge/reflection_necessity.go` | NEW | 纯函数:`ComputeReflectionNecessity(top1_score float64, events []Event) float64` |
| B12 | `internal/application/service/eval_badcase.go` | NEW | auto flag + manual promote + jump_back link |
| B13 | `internal/application/service/eval_runner_metrics.go` | NEW | Prom:`eval_runs_total{dataset_id,status}` + `eval_judge_score_drift{judge_prompt_version}` + `eval_run_duration_seconds` |
| B14 | `internal/handler/eval_dataset.go` | NEW | CRUD handler |
| B15 | `internal/handler/eval_run.go` | NEW | run + result + cancel handler |
| B16 | `internal/handler/eval_badcase.go` | NEW | list + promote + resolve handler |
| B17 | `internal/router/routes_infra.go` | MOD | `RegisterEvalRoutes` 新增 `/api/v1/eval/*` 三个子路由;复用 `apiKeyRunEvaluations` capability |
| B18 | `internal/router/router_api_key_capabilities_test.go` | MOD | 新增 `/api/v1/eval/datasets` 等路由的 capability 断言 |
| B19 | `internal/container/container.go` | MOD | Provide `NewEvalDatasetService`,`NewEvalRunner`,`NewLLMJudge`,`NewEvalBadcaseService`,三个 handler |

### 2.2 Frontend

| # | 路径 | 操作 | 备注 |
| --- | --- | --- | --- |
| F01 | `frontend/src/api/eval.ts` | NEW | Axios 客户端封装:CRUD / runs / badcases |
| F02 | `frontend/src/stores/evalDataset.ts` | NEW | Pinia store:list + current + upsert |
| F03 | `frontend/src/stores/evalRun.ts` | NEW | Pinia store:runs + current run progress |
| F04 | `frontend/src/stores/evalBadcase.ts` | NEW | Pinia store:badcase list + promote |
| F05 | `frontend/src/components/admin/AdminEvalTab.vue` | NEW | 三 Tab:Datasets / Runs / Badcases;复用 AdminTabs.vue 的 Tab 槽位 |
| F06 | `frontend/src/components/eval/EvalDatasetForm.vue` | NEW | 创建 + JSON 导入 + 列表 |
| F07 | `frontend/src/components/eval/EvalRunProgress.vue` | NEW | 进度条 + 当前 run summary |
| F08 | `frontend/src/components/eval/EvalBadcaseList.vue` | NEW | badcase 列表 + severity badge + 跳转 chat turn |
| F09 | `frontend/src/locales/{zh-CN,en-US,ko-KR,ru-RU}/eval.json` | NEW/MOD | 8-12 keys × 4 locale |
| F10 | `frontend/src/components/admin/AdminTabs.vue` | MOD | 注册 "Eval" tab(如有 vault / system tab 同款注册路径) |

### 2.3 测试 + 脚本 + 文档

| # | 路径 | 操作 |
| --- | --- | --- |
| T01 | `internal/application/service/eval_runner_test.go` | NEW,≥ 8 用例:happy path / dataset 缺题 / judge 解析失败 / citation fidelity 缺 / reflection 错触发 / badcase 自动圈 / run cancel / concurrency limit |
| T02 | `internal/eval/judge/citation_fidelity_test.go` | NEW,≥ 4 用例:全覆盖 / 部分覆盖 / 全无 / 空 citations |
| T03 | `internal/eval/judge/reflection_necessity_test.go` | NEW,≥ 4 用例:应反思且反思 / 应反思未反思 / 不该反思却反思 / score 边界 |
| T04 | `internal/eval/judge/llm_judge_test.go` | NEW,≥ 4 用例:正常 JSON / 非法 JSON / 缺字段 / 模型调用失败 |
| T05 | `frontend/src/components/eval/EvalDatasetForm.test.ts` | NEW,≥ 3 用例 |
| T06 | `frontend/src/components/eval/EvalRunProgress.test.ts` | NEW,≥ 3 用例 |
| T07 | `frontend/src/components/eval/EvalBadcaseList.test.ts` | NEW,≥ 3 用例 |
| S01 | `scripts/smoke-eval.sh` | NEW,基于 `BASE_URL` / `TOKEN` / `DATASET_ID` env,dry-run safe |
| D01 | `docs/comet/changes/weknora-eval-system/{brief,spec}.md` | 已在 |

### 2.4 不改 / 锁定

| 路径 | 为什么不动 |
| --- | --- |
| `internal/types/evaluation.go` 旧 EvaluationTask / EvaluationDetail / MetricResult / RetrievalMetrics / GenerationMetrics | 旧入口(`POST /evaluation`)继续跑 fixture,不被 B31 替换 |
| `internal/application/service/evaluation.go` 的 Evaluation() 方法 | 保留旧入口,新增 `EvalRunner.Run()` |
| `internal/handler/evaluation.go` 旧 handler | 保留 |
| `internal/router/routes_infra.go` 旧 `RegisterEvaluationRoutes` | 保留 |
| `internal/application/service/metric_hook.go` 旧 HookMetric | 扩展而非替换 |
| B30 chat pipeline 三个文件(`reflection.go` / `tool_cache.go` / `citation_tokens.go`) | 不动 |

---

## 3. 数据库 schema(摘要)

```sql
-- 000103_eval_system.up.sql
CREATE TABLE eval_datasets (
  id              VARCHAR(64) PRIMARY KEY,
  tenant_id       BIGINT NOT NULL,
  name            VARCHAR(200) NOT NULL,
  description     TEXT,
  qa_count        INT NOT NULL DEFAULT 0,
  schema_version  INT NOT NULL DEFAULT 1,
  created_by      VARCHAR(64) NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_eval_datasets_tenant ON eval_datasets(tenant_id, created_at DESC);

CREATE TABLE eval_dataset_qa (
  dataset_id       VARCHAR(64) NOT NULL REFERENCES eval_datasets(id) ON DELETE CASCADE,
  qid              INT NOT NULL,
  question         TEXT NOT NULL,
  expected_answer  TEXT NOT NULL,
  expected_passages JSONB NOT NULL DEFAULT '[]',
  tags             TEXT[] NOT NULL DEFAULT '{}',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (dataset_id, qid)
);
CREATE INDEX idx_eval_dataset_qa_dataset ON eval_dataset_qa(dataset_id);

CREATE TABLE eval_runs (
  id                  VARCHAR(64) PRIMARY KEY,
  tenant_id            BIGINT NOT NULL,
  dataset_id           VARCHAR(64) NOT NULL,
  chat_model_id        VARCHAR(64) NOT NULL,
  rerank_model_id      VARCHAR(64),
  reflection_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
  judge_model_id       VARCHAR(64) NOT NULL,
  judge_prompt_version VARCHAR(32) NOT NULL DEFAULT 'v1',
  status               VARCHAR(32) NOT NULL DEFAULT 'pending',
  started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at          TIMESTAMPTZ,
  canceled_at          TIMESTAMPTZ,
  error                TEXT,
  summary              JSONB,
  correlation_id       VARCHAR(64),
  git_sha              VARCHAR(40),
  created_by           VARCHAR(64) NOT NULL
);
CREATE INDEX idx_eval_runs_tenant ON eval_runs(tenant_id, started_at DESC);
CREATE INDEX idx_eval_runs_dataset ON eval_runs(dataset_id, started_at DESC);

CREATE TABLE eval_run_results (
  run_id                       VARCHAR(64) NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
  qid                          INT NOT NULL,
  question                     TEXT NOT NULL,
  model_answer                 TEXT NOT NULL,
  expected_answer              TEXT NOT NULL,
  search_top_k                 JSONB NOT NULL DEFAULT '[]',
  citation_index               JSONB NOT NULL DEFAULT '[]',
  reflection_events            JSONB NOT NULL DEFAULT '[]',
  factuality_score             FLOAT,
  citation_fidelity_score      FLOAT,
  reflection_necessity_score   FLOAT,
  passed                       BOOLEAN NOT NULL,
  badcase_flag_reason          VARCHAR(64),
  created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, qid)
);
CREATE INDEX idx_eval_run_results_run ON eval_run_results(run_id);
CREATE INDEX idx_eval_run_results_passed ON eval_run_results(run_id) WHERE passed = FALSE;

CREATE TABLE eval_badcases (
  id                     VARCHAR(64) PRIMARY KEY,
  tenant_id              BIGINT NOT NULL,
  run_id                 VARCHAR(64) NOT NULL,
  qid                    INT NOT NULL,
  flag_source            VARCHAR(32) NOT NULL,  -- auto | human_promote
  severity               VARCHAR(16) NOT NULL,  -- low | medium | high | critical
  status                 VARCHAR(32) NOT NULL DEFAULT 'open',
  notes                  TEXT,
  jump_chat_message_id   VARCHAR(64),
  promoted_by            VARCHAR(64),
  promoted_at            TIMESTAMPTZ,
  resolved_at            TIMESTAMPTZ,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_eval_badcases_tenant ON eval_badcases(tenant_id, status, created_at DESC);
CREATE INDEX idx_eval_badcases_run ON eval_badcases(run_id);
```

---

## 4. API 表面(摘要)

```
# Dataset CRUD
POST   /api/v1/eval/datasets                      # 创建 + 上传 JSON
GET    /api/v1/eval/datasets                      # 列表
GET    /api/v1/eval/datasets/:id                  # 详情 + QA
PUT    /api/v1/eval/datasets/:id                  # 改名 / 改描述
DELETE /api/v1/eval/datasets/:id                  # 级联删 QA + 历史 runs

# Run
POST   /api/v1/eval/runs                          # body: {dataset_id, chat_model_id, rerank_model_id?, reflection_enabled?, judge_model_id?}
GET    /api/v1/eval/runs/:id                      # 含 summary + 所有 run_results
GET    /api/v1/eval/runs                          # 列表 + 分页 + filter dataset_id/status
POST   /api/v1/eval/runs/:id/cancel               # 取消正在跑的

# Badcase
GET    /api/v1/eval/badcases                      # filter status/severity/source
POST   /api/v1/eval/badcases/:id/promote          # body: {severity, notes}
POST   /api/v1/eval/badcases/:id/resolve          # body: {notes}
```

权限:
- 全部 `apiKeyRunEvaluations(apiKeyFullAccess())` 现有 capability 复用
- Admin+ 在 RBAC 链路上

---

## 5. 验证命令(checklist)

### 5.1 编译 + 单测
```bash
# backend
go test -short -run 'TestEval|TestJudge|TestCitationFidelity|TestReflectionNecessity|TestLLMJudge' ./...
# 期望:exit 0,TestEval ≥ 8 / TestJudge 系列 ≥ 12

# frontend
cd frontend && npx vue-tsc --noEmit && npx vitest run --reporter=verbose Eval
# 期望:vue-tsc 0 error,Vitest ≥ 9 用例

# i18n
cd frontend && node -e "Object.keys(require('./src/locales')).forEach(l=>{const o=require('./src/locales/'+l+'/eval.json');const ref=require('./src/locales/zh-CN/eval.json');Object.keys(ref).forEach(k=>{if(!(k in o))throw new Error(l+' missing '+k)})})"
# 期望:exit 0(4 locale 同步)
```

### 5.2 静态 grep
```bash
# Schema 改动存在
grep -c 'eval_datasets\|eval_runs\|eval_run_results\|eval_badcases' migrations/000103_eval_system.up.sql
# 期望:≥ 8(每张表 ≥2)

# 新增 AuditAction 存在
grep -c 'eval\.\(run_started\|run_completed\|badcase_flagged\|dataset_updated\|run_reviewed\)' internal/types/audit_log.go
# 期望:≥ 5

# LLM-as-judge 三个评分函数名
grep -c 'func.*\(CitationFidelity\|ReflectionNecessity\|LLMJudge\)' internal/eval/judge/*.go
# 期望:≥ 3

# 路由命名空间使用 /api/v1/eval/
grep -c '"/api/v1/eval/' internal/router/routes_infra.go
# 期望:≥ 6(注册 3 个子路径 + GET 详情/列表)

# correlation_id 透传
grep -c 'CorrelationIDFromContext\|WithBackgroundCorrelationID' internal/application/service/eval_runner.go
# 期望:≥ 2
```

### 5.3 Smoke
```bash
EVAL_SMOKE_BASE_URL=http://localhost:8080 \
EVAL_SMOKE_TOKEN=$JWT \
EVAL_SMOKE_DATASET_ID=ds_smoke \
bash scripts/smoke-eval.sh
# 期望:三步 curl 各返回 200 + JSON
```

---

## 6. 强制 checklist(commit 前自查)

- [ ] B01-B19 全部落地,commit 单拆 / 合并皆可,build/commit 前 `go build ./...` exit 0
- [ ] T01-T07 全跑通,harness + vitest 0 failure
- [ ] S01 dry-run safe,本地有 WeKnora 实例时实跑 200
- [ ] F09 i18n 4 locale 同步,`tsc` 0 error
- [ ] 旧入口(`POST /api/v1/evaluation`)不破:`internal/handler/evaluation_test.go` 仍通过
- [ ] B30 三个文件无 diff
- [ ] audit `eval.*` 五个 action 通过 `WikiAuditSourceActivity` 投影(B24 B2 复用)
- [ ] correlation_id 透传,且 B25 WikiAuditDrawer chip 可跳回 eval 详情
- [ ] badcase 库 `jump_chat_message_id` 字段填非空(走 B30 B4 的 source_message_id 链路)
- [ ] 不引入新模型类型 / 不动 ModelService 接口
- [ ] D5 dataset 上限(100 dataset / 10000 题)实测返回 422
- [ ] D6 badcase 仅 Admin+ 可见,Viewer 403
- [ ] 评估种子数据走 `./dataset/samples/*.parquet` 导入,但允许跳过(空 dataset 也允许跑 → 报 `dataset_empty`)

---

## 7. 与 brief.md 的对应关系

| brief 章节 | spec 章节 |
| --- | --- |
| §4.1 IN(A-J) | §2 改动清单 + §3 schema + §4 API |
| §6 决策 D1-D8 | 全部入 spec,D7 直接落到 §2.1 B17,D8 直接落到 §2.2 F05 |
| §7 风险与对策 | 体现在 T01-T04 测试用例 + §6 checklist |
| §8 验收 A1-A12 | §5 验证命令 + §6 checklist 合并 |
| §9 排期 | 内部参考,不入 spec |

---

**等你点头进 Build**(默认:全部按 spec 实施,无 [blocking] 决定悬而未决)。

如果你想:
- 调整某条 D* → 告诉我直接改 brief + spec
- 拆 B31a(后端)/ B31b(前端) → 拆,分别给 change 名
- 加 human review 后台 → 改成 B31 + B31.x,本次只做自动 + badcase
- 砍 LLM-as-judge → 改 B31 = 纯数据集 + BLEU/ROUGE + 持久化,无 judge