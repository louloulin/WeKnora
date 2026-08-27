# B30 — chat-multiturn-tools-evidence spec

> 详见 `brief.md`,本 spec 仅列验收矩阵 + 强制 checklist + 验证命令 + Prom 查询参考。

## 验收矩阵

| ID | 测试 | 命令 | 期望 |
| --- | --- | --- | --- |
| **A1** | `PluginReflection` 注册 | `grep -c 'NewPluginReflection' internal/application/service/chat_pipeline/*.go` | ≥ 1 |
| **A2** | 反思触发条件 | `go test -short -run 'TestReflectionTrigger' ./internal/application/service/chat_pipeline/...` | exit 0;top-1 score<阈值触发,≥ 阈值不触发;SearchResult 空触发 |
| **A3** | 反思事件 emit | `grep -c 'EventAgentReflection\|REFLECTION' internal/event/*.go` | ≥ 1(类型 + 事件常量) |
| **A4** | `ToolCache` TTL+LRU | `grep -c 'type ToolCache\|NewToolCache\|Get\|Set' internal/application/service/tool_cache.go` | ≥ 4 个核心方法 |
| **A5** | Prom metrics | `grep -c 'chat_tool_cache' internal/application/service/tool_cache_metrics.go` | ≥ 3(hits/misses/evictions) |
| **A6** | write hook invalidate | `grep -c 'toolCache.Invalidate' internal/application/service/wiki_*.go internal/application/service/acl_*.go` | ≥ 2 处 hook 调用点 |
| **A7** | citation token 生成 | `grep -c 'attachCitations\|\\[\\[cite:' internal/application/service/chat_pipeline/into_chat_message.go` | ≥ 1 |
| **A8** | citation_log handler | `test -f internal/handler/citation_log.go && grep -c 'LogAuditEvent\|citation_access' internal/handler/citation_log.go` | exit 0,≥ 2 处引用 |
| **A9** | cross-turn source_message_id | `grep -c 'source_message_id' internal/handler/citation_log.go frontend/src/components/WikiAuditDrawer.vue` | ≥ 2 |
| **A10** | harness + vitest | `go test -short -run 'TestReflection\|TestToolCache\|TestCitationLog' ./...` + `cd frontend && npx vitest run --reporter=verbose AgentStreamDisplay.citation` | exit 0;Go ≥ 6 个 case,Vitest ≥ 3 个 case |

## 强制 checklist(commit 前自查)

### 后端

- [ ] `PluginReflection` 注册时 `ActivationEvents()` 返回 `[]types.EventType{types.REFLECTION}`,且只在 `MultiTurnEnabled=true` 时实际运行(D10)
- [ ] `PluginReflection` 决策函数**只读 `chatManage.SearchResult`**,不直接调外部 service(单测要能脱离 DB / 外部依赖)
- [ ] 反思重试时 `embedding_top_k +50%`(整数化,向上取整),`vector_threshold -0.05`(下限 0.0,不能再低),**不修改 RerankModelID**
- [ ] `ToolCache.Get(key)` 命中时直接返回 `*ToolResult`,不调用 `tool.Execute`;未命中时调 `tool.Execute` 并 `Set(key, result)`
- [ ] `ToolCache` key 计算:对 `args` 做 `json.Marshal` + `sort.SliceStable` 字段顺序 + `sha256`,确保同语义 args 命中同 key(D5)
- [ ] `ToolCache.Set` 时如 LRU 已满,evict 最久未用 entry + Prom `chat_tool_cache_evictions_total` +1
- [ ] `ToolCache.Set` 时检查 TTL(默认 5min);Get 时如果 entry 超过 TTL,返回 cache miss + 删除 entry
- [ ] `chat_tool_cache_*` Prom 注册用 `promauto`,与 B29 风格一致;labels `[]string{"kb_id", "tool_name"}`,顺序在 hits/misses/evictions 三者一致
- [ ] `IntoChatMessage.attachCitations` 只在 `CitationEnabled=true` 时插入 `[[cite:N]]` token(D7 + 兼容默认开)
- [ ] `IntoChatMessage.attachCitations` 用 `chatManage.MergeResult` 的 index 作为 N(与 `<context id="N">` 一致);N 必须是 1-based 数字
- [ ] `citation_log` handler 接收 `{chunk_id, source_message_id, citation_index}` 三字段,缺一返回 400
- [ ] `citation_log` 写入 `wiki_audit` 时 `correlation_id` 从 `CorrelationIDFromContext(ctx)` 取,**fallback 到 `request_id`**(B25 行为);audit 表 schema 不变
- [ ] `citation_log` 写 audit 用 `go func() { ... }()` fire-and-forget,失败 `logger.Warn` 不阻断 200
- [ ] `PluginChatCompletion` 接到 REFLECTION event 后,做第二次 chat 调用,把反思后的 SearchResult 注入上下文
- [ ] `cmd/server/wire.go` 把 `ToolCache` 注入到 `chatpipeline.NewEventManager`,默认 LRU=1000、TTL=5min
- [ ] **不**修改 `MaxRounds` 默认值;**不**改 `wiki_audit` schema;**不**新增 event type 字段(只新增 `REFLECTION` 常量)

### 前端

- [ ] `AgentStreamDisplay.vue` 的 `[[...]]` 解析器(`AgentStreamDisplay.vue:750`)增加 `[[cite:N]]` 分支,匹配 `[0-9]+` 才走 citation 路径
- [ ] `[[cite:N]]` 渲染成 `<sup class="citation-token" data-cite-id="N">[来源 N]</sup>`,点击触发 `referencesDrawer.open({...})` 并定位 chunk
- [ ] `AgentStreamDisplay.vue` 新增 reflection loading 状态:收到 `ResponseTypeReflection` 事件后,显示 "反思中..."(i18n key)
- [ ] `WikiAuditDrawer.vue` 渲染 `source_message_id` chip,点击跳到 chat history 对应 message(`router.push({name: 'chat', params: {sessionId}, query: {messageId}})`)
- [ ] `citationChunkCache.ts` 增加 cross-turn lookup 接口 `getChunkAcrossTurns(sessionId, chunkId): SearchResult | null`
- [ ] i18n 4 locale 都加 `agentStream.reflection.loading` / `agentStream.citation.crossTurnHint`,**不**新增翻译,直接复用既有 `referencesDrawerTitle` 等
- [ ] `AgentStreamDisplay.citation.test.mjs` 覆盖:render token / click → drawer / cross-turn lookup 找到 / 找不到 fallback

### 跨端

- [ ] **不**在 production 默认开启 reflection — `AgentConfig.ReflectionEnabled *bool` 默认 nil → false;运营显式开启才工作(D10)
- [ ] **不**把 reflection 状态写进 wiki_audit(只写 citation_access 操作)
- [ ] **不**为 tool cache 增加 Redis / 跨实例持久化(D4)
- [ ] **不**改 `AuditLog` 的 struct 字段(B25 已 ship,本次不破坏)

## 验证命令(本机 dry-run)

```bash
# 后端单元测试
go test -short -run 'TestReflectionTrigger|TestReflectionRetry|TestToolCache|TestCitationLog' \
  ./internal/application/service/... \
  ./internal/handler/...

# 后端 build 通过
go build ./...

# 前端类型检查
cd frontend && npx vue-tsc --noEmit && cd ..

# 前端 vitest
cd frontend && npx vitest run --reporter=verbose AgentStreamDisplay.citation && cd ..

# 前端 i18n 校验
cd frontend && node scripts/check-i18n-completeness.mjs && cd ..

# 后端 harness 全量(防止回归)
go test -short ./internal/...

# smoke 脚本(本地 dry-run safe)
bash scripts/smoke-chat-tool-cache.sh
bash scripts/smoke-citation-audit-link.sh
```

## Prom 查询模板(写入 `docs/observability/chat-tool-cache.md`)

```promql
# Q1 — tool cache hit_ratio(按 tool 切分)
sum by (tool_name) (rate(chat_tool_cache_hits_total[5m]))
/
sum by (tool_name) (rate(chat_tool_cache_hits_total[5m]) + rate(chat_tool_cache_misses_total[5m]))

# Q2 — eviction rate(容量压力)
sum by (tool_name) (rate(chat_tool_cache_evictions_total[5m])) * 60

# Q3 — cache 命中率全局(单实例健康度)
sum(rate(chat_tool_cache_hits_total[5m]))
/
sum(rate(chat_tool_cache_hits_total[5m]) + rate(chat_tool_cache_misses_total[5m]))

# Q4 — 多实例 hit_ratio 对比(发现 stuck pod)
sum by (instance) (rate(chat_tool_cache_hits_total[5m]))
/
sum by (instance) (rate(chat_tool_cache_hits_total[5m]) + rate(chat_tool_cache_misses_total[5m]))
```

## 反思事件 schema(`internal/event/event_data.go` 新增)

```go
// ReflectionData emits when the fast chat pipeline decides to re-retrieve
// because first-pass results were insufficient (low score / empty).
// Done=true when the second chat call kicks off; consumers (frontend)
// can show a "反思中..." loading state until the final answer starts streaming.
type ReflectionData struct {
    Reason          string                 `json:"reason"`           // "low_top_score" / "empty_results" / "model_explicit"
    AdjustedParams  map[string]interface{} `json:"adjusted_params"`  // {"embedding_top_k": 15, "vector_threshold": 0.25}
    OriginalTopK    int                    `json:"original_top_k"`
    OriginalThresh  float64                `json:"original_threshold"`
    Iteration       int                    `json:"iteration"`
    Done            bool                   `json:"done"`
}
```

## 后端关键类型增量(`internal/types/chat_manage.go` / `event.go`)

```go
// EventType constants
const (
    // ... 既有 ...
    REFLECTION EventType = "reflection"  // B30 新增
)

// PipelineState 增量字段
type PipelineState struct {
    // ... 既有 ...
    ReflectionAttempted int                       `json:"reflection_attempted,omitempty"` // 0 = 未尝试,1 = 已反思一次
    ReflectionContext   *ReflectionContext        `json:"reflection_context,omitempty"`
}

type ReflectionContext struct {
    Reason         string  `json:"reason"`
    OriginalTopK   int     `json:"original_top_k"`
    OriginalThresh float64 `json:"original_threshold"`
    NewTopK        int     `json:"new_top_k"`
    NewThresh      float64 `json:"new_threshold"`
}
```

## 不在 spec 内(留给后续 Build)

- 反思的 prompt 改写 / 查询扩展(B31 eval 阶段做)
- Redis 持久化 tool cache(B31)
- 反思的 UI 切换开关 / 全局禁用(留给运营在 agent config 里配)
- 模型在 chat 内**主动**选择反思(本 Build 只做启发式 + system prompt 提示;主动调用由 B31 eval 评估)
- 跨 session cache 共享(B31 之后)
- 多模态 evidence(图片 / 表格 → citation)(B32)
- citation token 在 export(导出 PDF / Markdown)时的处理(B31)

## 已知 limit(写入 reply 文档)

1. 反思只在 fast pipeline(`session_knowledge_qa.go`)启用;agent pipeline(ReAct loop)继续走原有逻辑,本 Build 不动
2. tool cache 不跨进程 / 跨实例 — 多 pod 部署时每个 pod 各自一份 cache,通过 wiki_audit 写操作 invalidate hook 间接同步
3. `[[cite:N]]` 与某些模型偶发的 `[[wiki_link]]` 冲突时,优先匹配 cite(N 必须是纯数字,wiki_link 通常含字母),但极端情况可能误命中 — 实测后调整解析顺序
4. reflection event 在 streaming 路径下,如果 emit 失败前端会卡在 "反思中...";通过 5s timeout 恢复(写在 i18n key 注释里,不实际加 timeout 代码,依赖事件最终到达)