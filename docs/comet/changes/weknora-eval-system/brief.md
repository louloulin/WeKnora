# B31 — Eval System (Build #31)

> Comet-Native change · branch `lumos0826` · triggered by `01a0433c-7e56-780e-83d1-b0444e966e3c`
> brief.md 调研版本 · 在 Build 前需要你确认 §5 范围与 §6 决策

---

## 1. 背景

P1 上半(Build #30)产出了 chat 主路径上三类能力:
- 多轮反思(`PluginReflection` 事件 + `event.REFLECTION`)
- 工具缓存(`ToolCache` + `chat_tool_cache_*` Prom)
- 证据渲染(`[[cite:N]]` token + `chat_manage.CitationIndex`)

但**今天没有任何东西持续监测这些能力的质量**——只有在 PR 上线后靠人工 demo 试错,或等客户报障才知道某个 chat 答案出错了。

P1 下半(Build #31)的目标是把"chat 答案质量"变成可自动评估、可追溯到具体样本、可人工复核的体系。

---

## 2. 现状盘点(2026-08-27)

### 2.1 `internal/application/service/evaluation.go` + `metric_hook.go` (476+194 LOC)

| 维度 | 现状 |
| --- | --- |
| 入口 | `POST /api/v1/evaluation` 跑全量;`GET /api/v1/evaluation?task_id=…` 拿结果 |
| 数据集 | `./dataset/samples/*.parquet` 五个本地文件(`queries / corpus / answers / qrels / qas`),**datasetID 实际被忽略**,永远返回同一份 fixture |
| 执行模型 | 自动建临时 KB + 灌 passages + 跑 `KnowledgeQAByEvent`(B30 之前就是这套) |
| 评分 | `HookMetric`:retrieval 维度(Precision / Recall / NDCG@3,10 / MRR / MAP)+ generation 维度(BLEU-1/2/4 / ROUGE-1/2/L) |
| 并发 | errgroup,worker 数 = `GOMAXPROCS - 1` |
| 任务存储 | `evaluationMemoryStorage` 进程内 map + RWMutex,**无持久化**,重启即失 |
| 路由 | `RegisterEvaluationRoutes` 注册 `POST/GET`,Admin / Viewer 守卫,API-key gated(`apiKeyRunEvaluations`) |
| DI | `service.NewEvaluationService` + `service.NewDatasetService` + `handler.NewEvaluationHandler` 已在 container.go 接线 |
| 清理 | `defer` 删除临时 KB + 临时 knowledge |

### 2.2 关键类型

```go
// internal/types/evaluation.go
type EvaluationTask  { ID, TenantID, DatasetID, Status, StartTime, ErrMsg, Total, Finished }
type EvaluationDetail{ Task, Params(*ChatManage), Metric(*MetricResult) }
type MetricResult    { RetrievalMetrics, GenerationMetrics }
type RetrievalMetrics{ Precision, Recall, NDCG3, NDCG10, MRR, MAP }
type GenerationMetrics{ BLEU1, BLEU2, BLEU4, ROUGE1, ROUGE2, ROUGEL }

// internal/types/interfaces/evaluation.go
type EvaluationService { Evaluation(...), EvaluationResult(...) }
type Metrics           { Compute(*MetricInput) float64 }  // 接口已定义但 BLEU/ROUGE 是直接函数
type EvalHook          { Handle(ctx, state, index, data) error }  // 接口已定义但只有 HookMetric 一个用
type DatasetService    { GetDatasetByID(ctx, datasetID) ([]*QAPair, error) }
```

### 2.3 B30 产出的可复用 telemetry

| 信号 | 来源 | 怎么被 B31 消费 |
| --- | --- | --- |
| `chat_tool_cache_{hits,misses}_total{kb_id, tool_name}` | `internal/application/service/tool_cache_metrics.go` | eval 完成时取 scrap 值算 hit_ratio |
| `chat_tool_cache_evictions_total{kb_id, tool_name}` | 同上 | 评估 cache 容量是否够用 |
| `event.REFLECTION` 在 `chat_manage.events` 落盘 | `internal/types/chat_manage.go:337` | 评测里判"反思触发是否合理" |
| `chat_manage.CitationIndex` (B30 B3 写入) | `internal/types/chat_manage.go` | 评测 citation fidelity 时跟 search_result 比对 |
| `audit_logs` 行(`action=chat.citation_accessed`,correlation_id) | B30 B4 | badcase 面板里 pivot 回 chat turn |

### 2.4 审计联动

- `audit_logs.action` 已有 `wiki.* / faq.* / chat.citation_accessed` 一批值,B31 新增 `eval.*` 系列:`eval.run_started / eval.run_completed / eval.badcase_flagged / eval.dataset_updated / eval.run_reviewed`。
- 复用 B25 `CorrelationIDFromContext` + B24 `WikiAuditSourceActivity` 投影,运维侧在同一个 audit feed 看到 eval 事件。
- badcase 库作为 `eval_badcases` 表落地,**不** 走 audit_logs(它是数据,不是事件)。

---

## 3. 用户可见目标

| 视角 | 看得到什么 |
| --- | --- |
| **Admin(运维/产品)** | 后台"评估"页:选数据集 → 选 chat 模型 + rerank 模型 + reflection 开关 → 点"开始评估" → 看总分 + 每题分数 + 自动圈出来的 badcase(可一键标"重要 / 已修") |
| **算法工程师** | 每次跑完 eval 都有:总数 / 通过率 / 各维度分 / top-5 badcase / hit_ratio / 反思触发率 / citation fidelity 折线 |
| **支持/客服** | 客户报"chat 答错了"时,查 badcase 库,定位到 chat message_id,跳回原 turn(B30 的 `source_message_id` chip 同款) |
| **CI/CD** | 改 chat prompt / reflection 阈值后跑 eval,超过阈值回滚(后续在 B33+ 接) |

---

## 4. 范围

### 4.1 IN(本次落地)

| 块 | 内容 | 用户可见价值 |
| --- | --- | --- |
| **(A) Eval dataset CRUD** | `eval_datasets` 表 + `eval_dataset_qa` 表 + REST CRUD;种子数据走 `./dataset/samples/*.parquet` 导入;后续支持 JSON 上传 | "我有一个新的 200 条 QA 集,跑一遍" |
| **(B) LLM-as-judge** | 新建 `internal/eval/judge/` 包:`Judge.Score(ctx, prompt, response, references) (Score, error)`;复用 chat model 配置通道;prompt template 走内嵌常量 + 可 override | 自动评分,不依赖 ground-truth 字面匹配 |
| **(C) Citation fidelity 维度** | 把 B30 的 `CitationIndex` 跟 `chatManage.SearchResult` 比对:每个 `[[cite:N]]` token 必须能映射回 top-K 检索结果中的某个 chunk_id | 判"模型给的脚注能不能对得上 chunk" |
| **(D) Reflection necessity 维度** | 判 `event.REFLECTION` 是否被触发,以及 top-1 score 是否真的低于阈值(D1: `top1_score < 0.5`) | 判"反思是不是乱开 / 漏开" |
| **(E) Eval run history** | `eval_runs` + `eval_run_results` 表,持久化每次跑的总分 / 各维度分 / 单题分 / 时间戳 / dataset_id / model 版本 | 历史趋势 + 跨 PR 对比 |
| **(F) Badcase 库** | `eval_badcases` 表:每次跑自动圈 score < 阈值的样本,落入 badcase;支持人工 promote 到"重要 badcase"(不参与自动计算,只展示);带跳转回原 chat turn 的 link | "上周 N 条错例" + "客户报的那条是不是已知问题" |
| **(G) Audit 联动** | `eval.run_started / run_completed / badcase_flagged / dataset_updated / run_reviewed` 五个新 AuditAction;correlation_id 透传;通过 WikiAuditSourceActivity 投影 | 运维侧同一 feed 看 eval 事件 |
| **(H) Telemetry 关联** | eval_run_results 关联 `chat_manage.events` 提取的 `REFLECTION` 计数;跑完时落一行 Prom counter `eval_runs_total{dataset_id}` | 可观测 + 趋势 |
| **(I) REST + 前端 最小集** | `GET/POST/PUT/DELETE /api/v1/eval/datasets`、`POST /api/v1/eval/runs`、`GET /api/v1/eval/runs/:id`、`GET /api/v1/eval/badcases`、前端 admin tab 一页式展示 | 后台可见,跑得动 |
| **(J) Harness + smoke** | `internal/application/service/eval_runner_test.go` ≥ 8 用例;`scripts/smoke-eval.sh` dry-run | 验收 + 可重放 |

### 4.2 OUT(明确不做)

| 主题 | 推到哪 |
| --- | --- |
| **Human review 后台 UI(评分滑块 + 多人合议)** | B31.x(后置):B31 先做自动 + 标 badcase,人评后台下一轮 |
| **CI 集成(eval 跑挂即卡 PR)** | B33+ 平台工程,需要先有 PR-CI 闸门(B-T2 已铺 GitHub Actions 但只做 wiki verify) |
| **多 judge 模型对比** | B31.x:先 1 个 judge 模型,后续多模型对比 |
| **跨租户共享 eval 集** | 留待企业版 |
| **eval 历史趋势图 / Grafana** | 跟 B29 一样走 docs/observability.md,Grafana panel 推到 B31.x |
| **prompt injection 检测 / 答案有害性** | 推到安全合规线 |
| **Dataset 多人协作标注 / 评分冲突解决** | B31.x |
| **Eval run 历史对比 UI(diff between runs)** | B31.x |

### 4.3 复用

| 复用对象 | 来源 | 怎么用 |
| --- | --- | --- |
| `interfaces.EvaluationService` + `HookMetric` | 今天 | 保留 `Evaluation` / `EvaluationResult` 入口,扩展 EvalRun 持久化层 |
| `interfaces.DatasetService` | 今天 | 改 fixture 加载为"按 ID 查 `eval_datasets` 表",fixture 降级为种子导入器 |
| `logger.CloneContext` + `CorrelationIDFromContext` | B25 | eval goroutine 上下文传递 + audit 关联 |
| `WikiAuditSourceActivity` | B24 | `eval.*` 事件投影进 KB audit feed |
| `types.ModelService.ListModels` | 现有 | eval runner 选 chat / rerank 模型 |
| `ModelType{KnowledgeQA, Rerank}` | `internal/types/model.go:18-20` | judge 用 `ModelTypeKnowledgeQA` |
| `chat_manage.events`(包含 `event.REFLECTION`) | 现有 + B30 B1 | 评测里抽 `REFLECTION` 触发次数 |
| `chat_manage.CitationIndex` | B30 B3 | 评测 citation fidelity |
| B29 Prom 注册风格 | B29 | eval 的 Prom counter 沿用同一 promauto 模式 |
| 后台 admin 路由命名空间 | B24 | eval 走 `/api/v1/eval/*`(避开已有 `/evaluation` 以示区分) |
| 现有 RBAC + api-key capability | B12+ | eval 用 `apiKeyRunEvaluations` + Admin |

---

## 5. 设计(高层)

### 5.1 数据模型

```
eval_datasets (
  id, tenant_id, name, description,
  qa_count, schema_version, created_by, created_at, updated_at
)
eval_dataset_qa (
  dataset_id, qid,
  question, expected_answer (text),
  expected_passages (jsonb, [{pid, text}]),
  tags (text[]),  -- e.g. {"math","citation-heavy","reflection-required"}
  created_at
)
eval_runs (
  id, tenant_id, dataset_id,
  chat_model_id, rerank_model_id, reflection_enabled,
  judge_model_id, judge_prompt_version,
  status, started_at, finished_at, error,
  summary jsonb,  -- {total, passed, factuality, citation_fidelity, reflection_necessity, hit_ratio}
  correlation_id
)
eval_run_results (
  run_id, qid,
  question, model_answer, expected_answer,
  search_top_k jsonb, citation_index jsonb, reflection_events jsonb,
  factuality_score, citation_fidelity_score, reflection_necessity_score,
  passed boolean, badcase_flag_reason,
  created_at
)
eval_badcases (
  id, tenant_id, run_id, qid,
  flag_source (auto | human_promote),
  severity (low | medium | high | critical),
  status (open | triaged | resolved | wontfix),
  notes text, jump_chat_message_id text,
  promoted_by, promoted_at, resolved_at,
  created_at
)
```

> 没有新 audit 表;`eval.*` 走 audit_logs。`eval_*` 四张表都是新表。
> migration:`000103_eval_system.up.sql` / `.down.sql`,一次性建四张表。

### 5.2 LLM-as-judge prompt 模板(初版,可 override)

```
You are grading a RAG answer.

QUESTION: {{.Question}}
EXPECTED ANSWER: {{.ExpectedAnswer}}
SUPPORTING PASSAGES:
{{range .Passages}}
[{{.PID}}] {{.Text}}
{{end}}
MODEL ANSWER: {{.ModelAnswer}}
CITATIONS USED: {{.CitationIndex}}

Score 1-5 on three dimensions (1 worst, 5 best):
- factuality: Does the model answer convey only what is supported by the passages?
- citation_fidelity: Does each [[cite:N]] point at a passage that actually supports the claim it sits next to?
- conciseness: Is the answer free of padding, hedging, and repeated statements?

Respond as JSON only: {"factuality":N, "citation_fidelity":N, "conciseness":N, "reason":"one sentence"}
```

- 默认 `judge_prompt_version` = `v1`,存在 `eval_runs.judge_prompt_version` 字段
- 调模型走 `internal/eval/judge/llm_judge.go`:`LLMJudge.Score` 把 prompt 丢给 `ModelService` 选出的 chat 模型,解析 JSON 返回
- 单题评分失败 → 整题记 `passed=false, badcase_flag_reason="judge_error"`,不阻断其它题

### 5.3 Eval runner 流程

```
POST /api/v1/eval/runs
  body: {dataset_id, chat_model_id, rerank_model_id, reflection_enabled, judge_model_id}
  ↓
  1. 创建 eval_runs(status=running, correlation_id=X-Request-ID)
  2. audit: eval.run_started
  3. 异步 goroutine(同名 worker 模型,沿用 errgroup + GOMAXPROCS-1):
     for qa in eval_dataset_qa where dataset_id=…:
        chatManage := Clone(...)
        chatManage.Query = qa.Question
        chatManage.ReflectionEnabled = body.reflection_enabled
        session.KnowledgeQAByEvent(...)       // 复用 B30 chat pipeline
        citations := chatManage.CitationIndex
        reflection_events := chatManage.Events.filter(REFLECTION)
        scores := LLMJudge.Score(...)
        eval_run_results.insert(...)
        if avg(scores) < threshold or judge_error:
            eval_badcases.insert(auto)
            audit: eval.badcase_flagged
     4. summary := aggregate(...)
     eval_runs.update(summary, finished_at)
     audit: eval.run_completed
```

### 5.4 Citation fidelity 算法

```python
for cit in citation_index:                # B30: [{chunk_id, citation_index, kb_id, title}]
    top_passage := find(passages, chunk_id == cit.chunk_id)
    if not top_passage:
        fidelity_score = 0; break
    # 简化版:chunk 出现在 top-K 即给 1 分,缺失给 0
    fidelity_score += 1
return fidelity_score / len(citation_index)
```

> 复杂版(NLI / entailment)推到 B31.x。

### 5.5 Reflection necessity 算法

```
top1_score := search_results[0].score
expected := top1_score < 0.5
actual := REFLECTION event exists in chat_manage.events
if expected == actual: score = 1.0
elif expected and not actual: score = 0.0  # 该反思没反思
else: score = 0.5                         # 不该反思却反思了(浪费 token)
```

---

## 6. 决策清单(D1-D8)

| ID | 决策 | 候选 | 暂选 |
| --- | --- | --- | --- |
| **D1** | Reflection 触发阈值 | (a) 沿用 B30 默认 0.5 / (b) 改为按 KB 维度配置 / (c) 关闭反思再单跑一次评估必要性 | **(a)** — 暂沿用 B30 默认,后续 B31.x 接 KB 维度 |
| **D2** | LLM-as-judge 用哪个 chat 模型 | (a) 强制 admin 选 / (b) 默认 `ModelTypeKnowledgeQA` 的第一个 / (c) 沿用 eval_runs 的 chat_model_id | **(b)** — admin 可 override;默认让 judge = eval 用的同一个 chat 模型(避免引入新模型) |
| **D3** | Badcase 自动阈值 | (a) 任何维度 < 3 即入 badcase / (b) 总分 < 3 即入 / (c) 全部入 | **(b)** — 总分 < 3 触发 auto badcase,其它情况管理员可手动 promote |
| **D4** | Eval run 历史保留时长 | (a) 永久 / (b) 90 天滚动 / (c) 保留最近 100 跑 | **(b)** — 90 天滚动;`eval_runs.created_at < now() - interval '90 days'` 由 sweeper 清理(沿用 B22 cleanup cron 模式) |
| **D5** | Dataset 上限 | (a) 每租户 100 个 dataset / (b) 无上限 / (c) 单 dataset 10000 题上限 | **(a)+(c)** — 每租户 100 dataset,单 dataset 10000 题,超限 422 |
| **D6** | Badcase 库可见性 | (a) 全租户可见 / (b) admin-only / (c) 创建人 + admin | **(b)** — Admin+ 看 |
| **D7** | Eval 路由命名空间 | (a) 复用 `/api/v1/evaluation` / (b) 新建 `/api/v1/eval/*` | **(b)** — 不破坏既有 `/evaluation`;eval_set / eval_runs / eval_badcases 三个子路径 |
| **D8** | Frontend 入口 | (a) 集成到现有 admin 后台 / (b) 独立 `/eval` 路由 | **(a)** — 沿用现有 AdminTabs;新加 "Eval" tab |

---

## 7. 风险与对策

| 风险 | 概率 | 影响 | 对策 |
| --- | --- | --- | --- |
| **judge 模型评分不稳定** | 中 | 同一份数据跑两次结果差异 > 0.3 | judge_prompt_version 锁住,replay 同一 version 应稳定;Prom `eval_judge_score_drift` 监控 |
| **跑一次 eval 烧钱(LLM 调用 N 次)** | 高 | 200 条 × 一次生成 + 一次 judge = 400 次 chat 模型调用 | 默认限制 `eval_run_max_questions = 200`;超限需 Admin;跑前 dry-run 看预估费用 |
| **chat pipeline 改了 → eval 结果不可比** | 中 | 跨 PR 对比失效 | `eval_runs.chat_model_id` + `rerank_model_id` + `judge_prompt_version` 入参,跑时也记 `git_sha` 字段 |
| **dataset 上传撞乱码 / 错列** | 中 | eval 结果废 | 上传时 dry-run 校验:必填字段、passages 非空、tags 去重 |
| **badcase 库爆炸** | 中 | 表越来越大 | 沿用 B22 cleanup cron,`eval_badcases.status='resolved'` 90 天后归档 |
| **eval goroutine 跨请求泄露** | 低 | ctx 没 cancel | 沿用 B30 `logger.CloneContext` + 在 eval_runs 加 `canceled_at` 字段,管理员可手动 cancel |
| **judge 解析 JSON 失败** | 中 | 单题评分失败 | 单题记 `passed=false, badcase_flag_reason="judge_parse_error"`,继续其它题 |
| **并发跑 N 个 eval 撞 worker pool** | 低 | GOMAXPROCS-1 不够用 | 沿用 errgroup 但新增 `eval_concurrency_limit = 2`(可配),超过排队 |

---

## 8. 验收(A1-A12)

### 8.1 Schema + 持久化(A1-A3)
| ID | 描述 | 验证命令(留作 spec) |
| --- | --- | --- |
| **A1** | 四张 eval_* 表创建,migration 000103 可重放 | `migrate up; migrate down; migrate up` 三步无错 |
| **A2** | Dataset CRUD:创建 200 条 QA → 列表 / 详情 / 删除 / 上传 JSON | `POST /api/v1/eval/datasets` + `GET /:id` round-trip |
| **A3** | Eval run 持久化,跨进程重启可查 | 跑一次 eval → 重启进程 → `GET /api/v1/eval/runs/:id` 仍能查到 |

### 8.2 LLM-as-judge + 三维度(A4-A7)
| ID | 描述 |
| --- | --- |
| **A4** | LLMJudge.Score 三维度(factuality / citation_fidelity / conciseness),JSON 解析容错 |
| **A5** | Citation fidelity 算法,缺 citation 给 0,top-K 命中给 1 |
| **A6** | Reflection necessity 算法,与 chat_manage.events 的 REFLECTION 事件对齐 |
| **A7** | 单题 judge_error 不阻断其它题,坏题进 badcase 库 |

### 8.3 Runner + REST(A8-A10)
| ID | 描述 |
| --- | --- |
| **A8** | POST /eval/runs 异步跑,GET /:id 拿到结果 |
| **A9** | Run summary JSON 含 total / passed / 维度分 / hit_ratio |
| **A10** | audit: `eval.run_started / run_completed / badcase_flagged` 落库;通过 `WikiAuditSourceActivity` 出现在 audit feed |

### 8.4 Badcase + UI(A11-A12)
| ID | 描述 |
| --- | --- |
| **A11** | Badcase 自动圈 + 手动 promote;前端 AdminTabs.Eval 页可见 + 可标注 |
| **A12** | 前端 i18n 4 locale + smoke 脚本 |

---

## 9. 排期

| 阶段 | 估计 |
| --- | --- |
| Backend: migration + 4 张表 + repo + service | 1.5d |
| Backend: LLM-as-judge + 三维度算法 | 1.0d |
| Backend: Runner + 持久化 + audit 联动 | 1.0d |
| Frontend: AdminTabs.Eval + dataset CRUD + run 列表 + badcase 列表 | 1.0d |
| i18n + harness ≥ 8 + smoke | 0.5d |
| verify + commit + push + reply | 0.3d |
| **合计** | **~5.3d** |

---

## 10. [blocking] 状态

| 阻塞项 | 需要你 |
| --- | --- |
| **Z1** | **D1** reflection 阈值是否沿用 B30 默认 0.5?(我推荐 ✅ 不动) |
| **Z2** | **D2** LLM-as-judge 是否就用 eval 用的同一个 chat 模型?(我推荐 ✅,避免引入新模型管理复杂度) |
| **Z3** | **D4** eval_runs 90 天滚动清理是否接受?(我推荐 ✅,跟 B22 cleanup cron 风格一致) |
| **Z4** | **D7** 命名空间 `/api/v1/eval/*` vs 复用 `/api/v1/evaluation`?(我推荐 ✅ 新命名空间,避开既有 fixture 入口) |

无 [blocking] 技术决策,只有上面 4 条决定需要你点头。

---

## 11. 与 B30 / B24 / B25 衔接

| 衔接点 | 描述 |
| --- | --- |
| B30 chat pipeline | eval runner 复用 `sessionService.KnowledgeQAByEvent` 跑全 chat 流程;`reflection_enabled` 字段控制是否走反思 |
| B30 CitationIndex | eval 读 `chatManage.CitationIndex` 做 citation fidelity |
| B30 ToolCache Prom | eval_run_results 算 `hit_ratio = hits / (hits + misses)` |
| B24 WikiAuditService | `eval.*` 五个 AuditAction 走同一条 Log 路径 |
| B25 correlation_id | eval_runs 跑前后的事件通过 correlation_id 关联,B25 的 WikiAuditDrawer chip 可跳到 eval 详情 |
| B22 cleanup cron | 90 天滚动清理 `eval_runs` + 归档 `eval_badcases.status='resolved'` |
| B29 Prom 风格 | `eval_runs_total{dataset_id}` + `eval_judge_score_drift` 沿用 promauto + DefaultRegisterer |

---

**等你点头就开 Build**(默认推荐:全部 ✅ + 不拆 + 接受上面 4 条决定)。

如果你想砍掉某块:
- 砍 (G) audit 联动:eval 跟 audit feed 解耦,运维侧看不到 eval 事件(不推荐)
- 砍 (H) telemetry 关联:badcase 不带 `jump_chat_message_id`(不推荐)
- 砍 (I) 前端最小集:只剩后台 API + curl 调试(不推荐,验证困难)

如果你想加某块:
- 加 human review 后台评分滑块 → B31 拆 B31 + B31.x,B31.x 做 UI
- 加 CI 卡 PR → 推到 B33 平台工程线