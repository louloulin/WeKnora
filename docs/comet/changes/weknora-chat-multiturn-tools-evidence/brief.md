# Build B30 — chat 多轮反思 + tool 缓存 + 证据渲染 + 审计联动

> Phase P1 上半。在已有 chat pipeline / B19 搜索 / B21 wiki cache / B24-B25 wiki audit 的基础上,补齐 chat 多轮反思、tool 调用缓存、结构化证据渲染、审计联动四条主线。
> 基线:`lumos0826` HEAD = `cee584c5`(B29 已 ship)。Phase 0(B-T1 + B-T2) + B19.x + B20-B29 全部 ship。

## 一句话

让 chat pipeline 在多轮对话中具备 **(a)** 反思检索质量并按需重检索的能力、**(b)** tool 调用的 TTL+LRU 跨轮复用、**(c)** 答案与原文章节的结构化 citation token + 弹层跳转、**(d)** 每条 citation 写入 wiki_audit 形成可追溯链路。

## 现状(Why we need it)

| 模块 | 现状 | 缺口 |
| --- | --- | --- |
| **chat 多轮** | `MaxRounds > 0` 时 `LoadHistory` 加载历史 → 直接进 `CHAT_COMPLETION_STREAM`。`MultiTurnEnabled` 在 `AgentConfig` 控制 | 模型不会反思**上轮检索结果**:看到 turn 1 的 top-1 score=0.42,不会自动调参重查。`ResponseTypeReflection` 类型已存在但**没有任何 fast pipeline 实际产出** |
| **tool 缓存** | 每次 tool call(`WikiBacklinkGraph`、`WikiSearch`、`WebSearch`、MCP 工具)全量重算,跨 turn 完全不复用 | turn 2 同一个 wiki_search 会再跑一遍 vector + jieba + rerank;MCP 工具调用还要再 round-trip 一次 |
| **citation 渲染** | `IntoChatMessage` 用 `<context id="N">` 标号,前端 `ChatReferencesDrawer` 弹层展示当前 turn 的 refs(通过 `EventAgentReferences` 事件) | (1) 模型 answer 文本里**没有任何结构化 citation token**,citation 链接只在 drawer 里集中展示,无法在文本流内联跳转;(2) cross-turn 不可见:turn 2 的答案如果引用 turn 1 的 chunk,drawer 里看不到 |
| **chat → audit** | `wiki_audit` 表已经 ship(B24),`correlation_id` 已经能透传(B25),`WikiAuditDrawer` 能显示 source_event_id chip | chat pipeline **从未写过 audit_log**:运营看不到"谁在哪轮问了什么、引用了哪个 chunk"。wiki 操作有审计,chat 操作无审计,链路断裂 |
| **reflection event** | `AgentReflectionData` 类型已存在(agent pipeline 用),`ResponseTypeReflection` 也已声明 | fast pipeline 没有产生该事件;前端没有"反思中..."的可视化 |

## 目标(用户可见结果)

| 场景 | 期望 |
| --- | --- |
| 多轮追问"再展开说说 X" | 模型看到 turn 1 的 chunk 集合,如果 top-1 score < 0.5,触发反思(最多 1 次),调高 `embedding_top_k` 重检索,把反思事件 emit 给前端("反思中..." loading) |
| 多轮重复 query | turn 2 再次 wiki_search 同样的 query,直接命中 turn 1 的 tool cache(< 1ms),emit `tool_cache_hit` 指标,跨轮持久化(直到 TTL 到期或 KB 写操作触发 invalidate) |
| 答案中点 `[来源 1]` | answer 文本里嵌入 `[[cite:N]]` token,前端渲染成可点击的 `[来源 1]`,点击 → 抽屉直接打开对应 chunk 的 source drawer,而不是只看到当前 turn 的汇总 |
| 跨轮追溯 | turn 3 的答案引用 turn 1 的 chunk C,在 `wiki_audit` 写入一行 `op=citation_access, correlation_id=<turn-3-request-id>, payload={chunk_id: C, source_message_id: <turn-1-msg-id>}`。`WikiAuditDrawer` 通过 `source_event_id` chip 跳回到 chat turn 3 |
| KB 写操作 → cache 自动失效 | wiki page 被修改时,关联的 tool cache key 自动 evict(参考 B21 wiki cache 的 `invalidation_strategy` 机制) |

## 验收(A1-A10)

| ID | 验收项 | 落点 |
| --- | --- | --- |
| **A1** | `PluginReflection` 注册到 `EventManager`,处理 `REFLECTION` event | `internal/application/service/chat_pipeline/reflection.go`(NEW) |
| **A2** | 反思触发条件:首次 SearchResult 为空 OR top-1 score < 阈值(默认 0.5,可在 AgentConfig 配);触发后重跑 `CHUNK_SEARCH` 一次,top_k +50%,vector_threshold -0.05 | `reflection.go` 决策函数,带 harness 单测 |
| **A3** | 反思事件 `ResponseTypeReflection` 通过 `EventBus.Emit` 发送,前端 `AgentStreamDisplay` 显示"反思中..."状态(已有 `loadingText`,只增加 i18n key) | `event_data.go` + `AgentStreamDisplay.vue` |
| **A4** | `ToolCache` 包装 `Tool.Execute`,key = sha256(`tool_name` + canonical_json(args) + tenant_id),TTL 默认 5min,LRU 容量 1000/tenant | `internal/application/service/tool_cache.go`(NEW) |
| **A5** | cache 命中/未命中 Prometheus 指标 `chat_tool_cache_{hits,misses,evictions}_total{kb_id, tool_name}` | `tool_cache_metrics.go`(NEW) |
| **A6** | 写操作触发 cache invalidate:wiki page 修改 / KB 写 / agent config 修改时,按 tenant 清空该 tenant 的 tool cache | hook 接 B21 `InvalidateBacklinksCache` 同款机制 |
| **A7** | citation token `[[cite:N]]` 由后端 `IntoChatMessage` 在 chat answer 后处理阶段插入(把 `<context id="N">` 反向链接写到答案里),前端 `AgentStreamDisplay.vue:750` 已知 `[[...]]` 解析路径,直接复用 + 加 citation 类 | `into_chat_message.go` 新增 `attachCitations` 函数,前端 `citationChunkCache` 增加跨 turn lookup |
| **A8** | 每次 citation 被前端点击 / 答案提交时,后端 `citation_log` handler 写一行 `wiki_audit`,带 `correlation_id`(从 B25 helper 取) | `internal/handler/citation_log.go`(NEW),handler route 注册 |
| **A9** | cross-turn 引用:turn 2 答案引用 turn 1 的 chunk,`WikiAuditDrawer` chip 显示 `source_message_id = <turn-1-msg-id>`,点击跳到 chat 历史的 turn 1 | `WikiAuditDrawer.vue` 增加 source message 跳转,后端 audit payload 加 `source_message_id` |
| **A10** | harness test ≥ 6 条(reflection 决策、cache hit/miss/expire/invalidate、citation token 生成、audit 写入);frontend vitest ≥ 3 条(citation token 渲染、cross-turn drawer 打开、audit chip 跳转) | `chat_pipeline/reflection_test.go` + `service/tool_cache_test.go` + `handler/citation_log_test.go` + 前端 `AgentStreamDisplay.citation.test.mjs` |

## 关键决策(D1-D10)

| ID | 决策 | 推荐 | 备选 |
| --- | --- | --- | --- |
| **D1** | 反思触发模型 | **启发式 + 模型可选** — heuristic 是 fast path(默认启用),模型在 system prompt 里被告知"如果检索结果不足可调用反思工具",工具路径是可选 | (A) 纯启发式(简单,模型不可控);(B) 纯模型判断(每次都要等模型回答,延迟+成本) |
| **D2** | 反思重试上限 | **1 次** — turn 内 max 1 次反思重检索,防止死循环 | (A) 2 次(可能引入无限循环风险);(B) 0 次(就退化为纯启发式) |
| **D3** | 反思重试参数调整幅度 | **embedding_top_k +50%, vector_threshold -0.05**(保守) | (A) +100%(激进);(B) 全部重置为默认值 |
| **D4** | tool cache 范围 | **per-tenant 内存 LRU + TTL**,跨 turn 持久化(在 `ChatManage` 持有 cache 引用,scope = session) | (A) 全局单例(无 tenant 隔离);(B) Redis 持久(超范围,推到 B31) |
| **D5** | cache key 设计 | `sha256(tool_name + sorted_json(args_without_session_id) + tenant_id)` — session-bound 字段排除在 key 之外 | (A) 直接用 args JSON(顺序敏感,易漏 cache);(B) 加 version 字段(过度) |
| **D6** | cache TTL 默认 | **5 分钟** — 与 B21 wiki backlinks cache 默认 TTL 对齐 | (A) 1 分钟(过短);(B) 1 小时(过长,易脏) |
| **D7** | citation token 格式 | **`[[cite:N]]`** — 后端插入,前端已有解析路径(`AgentStreamDisplay.vue:750`) | (A) `[^N]` markdown 脚注(渲染层冲突,某些模型会自带);(B) `<cite data-id="N"/>` HTML 标签(过度侵入) |
| **D8** | citation 写入时机 | **per-citation**(每个 chunk 单独写一行 audit)— 粒度细,运营易分析 | (A) per-turn(一次一行,粒度粗);(B) 答案生成时一次性写 |
| **D9** | audit 写入同步/异步 | **异步 fire-and-forget**,失败不阻断 chat — 参考 B24 的 audit write pattern | (A) 同步(影响 chat 延迟);(B) 批处理(复杂) |
| **D10** | reflection event 触发 vs AgentConfig.MultiTurnEnabled | **必须 MultiTurnEnabled=true** 才允许反思(否则反思没历史可反思,反而浪费 token) | (A) 无视 MultiTurnEnabled(浪费);(B) 默认开启(违反用户配置) |

## 改动清单(预估 ~12 文件)

| 文件 | 类型 | 行数 | 用途 |
| --- | --- | --- | --- |
| `internal/application/service/chat_pipeline/reflection.go` | NEW | +180 | `PluginReflection` + 启发式决策 + 反思重试编排 |
| `internal/application/service/chat_pipeline/reflection_test.go` | NEW | +120 | 决策 + 重试参数单测 |
| `internal/types/chat_manage.go` | MODIFY | +8 | `ReflectionAttempted int`,`ReflectionContext *ReflectionState` |
| `internal/types/event.go` 或 `chat_manage.go` | MODIFY | +6 | `REFLECTION` event type 常量 |
| `internal/event/event_data.go` | MODIFY | +15 | `ReflectionData{Reason, AdjustedParams, Iteration, Done}` |
| `internal/application/service/tool_cache.go` | NEW | +220 | TTL+LRU 缓存 + key 计算 + metrics |
| `internal/application/service/tool_cache_metrics.go` | NEW | +50 | Prom counters |
| `internal/application/service/tool_cache_test.go` | NEW | +180 | hit/miss/expire/invalidate 6+ 用例 |
| `internal/application/service/chat_pipeline/into_chat_message.go` | MODIFY | +60 | `attachCitations` 函数 + 把 `<context id="N">` 反向写到答案 |
| `internal/handler/citation_log.go` | NEW | +80 | POST `/citation-log`,写 wiki_audit 一行 |
| `internal/handler/routes.go`(或对应文件) | MODIFY | +6 | 注册新路由 |
| `internal/handler/citation_log_test.go` | NEW | +100 | 写 audit + correlation_id 验证 |
| `internal/application/service/chat_pipeline/chat_completion.go` | MODIFY | +20 | `PluginChatCompletion` 接到 REFLECTION event 后的二次 chat |
| `internal/application/service/chat_pipeline/chat_completion_stream.go` | MODIFY | +15 | stream 路径处理反思事件 |
| `cmd/server/wire.go` | MODIFY | +10 | DI 接入 `ToolCache` |
| `frontend/src/views/chat/components/AgentStreamDisplay.vue` | MODIFY | +30 | 反射 loading 状态 + citation token 类型 |
| `frontend/src/components/WikiAuditDrawer.vue` | MODIFY | +25 | source_message_id 跳转 chat turn |
| `frontend/src/utils/citationChunkCache.ts` | MODIFY | +30 | 跨 turn chunk lookup |
| `frontend/src/i18n/locales/{zh-CN,en-US,ko-KR,ru-RU}.ts` | MODIFY | +5×4=20 | reflection / cross-turn i18n key |
| `frontend/src/views/chat/components/AgentStreamDisplay.citation.test.mjs` | NEW | +80 | citation token 渲染 + cross-turn 测试 |
| `docs/observability/chat-tool-cache.md` | NEW | +100 | Prom 查询 + 故障排查 |
| `docs/comet/changes/weknora-chat-multiturn-tools-evidence/{brief,spec}.md` | NEW | - | 本套产物 |

## 范围外(明确不做)

- ❌ 不实现模型在 chat 内**主动**选择反思(只有启发式触发 + 可选 system prompt 提示,见 D1)
- ❌ 不做反思重试的 prompt 改写/查询扩展(留给 B31 eval 优化)
- ❌ 不上 Redis 持久化 tool cache(推到 B31)
- ❌ 不做 chunk-level 全文校验(只做 key 级别 cache,语义不重算)
- ❌ 不改 `MaxRounds` 默认值(保持向后兼容)
- ❌ 不上 reflection 的 UI 切换开关(管理员用 agent config 控制)
- ❌ 不改 audit 表 schema(B24 已 ship,只新增 route + handler)
- ❌ 不引入新的 citation 格式(`AgentStreamDisplay.vue:750` 的 `[[...]]` 路径够用)

## 风险点

| 风险 | 缓解 |
| --- | --- |
| 反思重检索可能引入 2x token 成本 | D2 限制 max 1 次;Prom 指标 `chat_reflection_total{outcome=hit/miss}` 让运营可监控 |
| tool cache 跨 tenant 误命中 | D5 key 含 tenant_id;harness test 覆盖跨 tenant 隔离 |
| cache 脏读(KB 已修改但 cache 未失效) | D6 TTL 5min 上限 + A6 write hook;通过 wiki_audit 反查 cache age |
| citation token `[[cite:N]]` 与某些模型的 `[[wiki_link]]` 冲突 | 前端 `AgentStreamDisplay.vue:750` 的解析器已存在 `[[slug\|name]]` 路径,新增 `[[cite:N]]` 分支;N 必须是数字,不会冲突 |
| 反思事件 emit 失败 → 前端卡在"反思中..." | emit 失败用 `pipelineWarn` 记日志,不阻断 chat;前端用 5s timeout 自动恢复 loading 状态 |
| audit 写入失败 → citation 追溯断链 | D9 异步 fire-and-forget,失败不影响主流程;Prom 指标 `citation_log_write_errors_total` 监控 |
| 反思 + cache 相互作用复杂(反思时要不要 bust cache?) | 反思重检索**不读 cache**(强制 fresh search),但反思后产出的新 SearchResult **写回 cache**;harness test 覆盖 |

## 与既有 Build 的衔接

| 上游 Build | 用到的产物 |
| --- | --- |
| B21 wiki backlinks cache | `InvalidateBacklinksCache(invalidation_strategy)` 机制 → 复用做 tool cache invalidate hook(B30 A6) |
| B24 wiki audit | `WikiAuditService.Log(op, payload, request_id)` → 复用做 citation_log(B30 A8) |
| B25 correlation_id | `CorrelationIDFromContext(ctx)` / `WithBackgroundCorrelationID(ctx)` → 透传到 citation_log |
| B29 prom multi-instance | Prom 注册模式 → 沿用 `promauto` + `DefaultRegisterer`,label cardinality 一致 |

## 排期(预估)

| 步骤 | 预计 |
| --- | --- |
| Shape(brief + spec + 用户确认) | 当前 |
| 后端 B1 — `PluginReflection` + harness test | 0.5 天 |
| 后端 B2 — `ToolCache` + Prom metrics + harness test | 0.5 天 |
| 后端 B3 — `IntoChatMessage` 嵌入 citation token | 0.3 天 |
| 后端 B4 — `citation_log` handler + audit 写入 | 0.3 天 |
| 前端 B5 — citation token 渲染 + cross-turn drawer | 0.4 天 |
| 前端 B6 — audit chip 跳转 + reflection loading | 0.2 天 |
| 集成 B7 — wire DI + i18n + smoke 脚本 | 0.3 天 |
| Verify + commit + push + reply | 0.3 天 |
| **总计** | **~2.8 天**(实际可能 4-5 天,含 review 缓冲) |

## [blocking] 待用户确认

无。Shape 阶段已基于代码现状推导出全部决策,无可阻塞外部输入。**等待用户对 Shape 的整体认可**;认可后进入 Build。