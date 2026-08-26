-- Build #14 — wiki 批量操作审计日志
--
-- 在 Build #13 的 wiki_batch_jobs 表之上,落"每一次状态转换 + undo 操作"
-- 作为不可变审计行。BIGSERIAL 主键 + 三个查询索引:
--   - 按 KB + 时间倒序:KB 管理员看近期操作
--   - 按 batch_job_id:单 job 完整审计链
--   - 按 actor + 时间倒序:追责 / 合规导出

CREATE TABLE IF NOT EXISTS wiki_batch_job_audit (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id UUID NOT NULL,
    batch_job_id UUID NOT NULL,
    action TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB
);

-- 7 个事件:
--   enqueue / start / finish / undo_request / undo_done / cancel / expire
-- 范围外 not-in-scope: per-slug 事件、跨 KB 平台审计、> 90 天归档
CREATE INDEX IF NOT EXISTS idx_wbb_audit_kb_occurred
    ON wiki_batch_job_audit (knowledge_base_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_wbb_audit_job
    ON wiki_batch_job_audit (batch_job_id);

CREATE INDEX IF NOT EXISTS idx_wbb_audit_actor
    ON wiki_batch_job_audit (actor_id, occurred_at DESC);

-- action 列只在 7 个枚举里查,加个轻量索引给过滤用
CREATE INDEX IF NOT EXISTS idx_wbb_audit_action
    ON wiki_batch_job_audit (knowledge_base_id, action, occurred_at DESC);