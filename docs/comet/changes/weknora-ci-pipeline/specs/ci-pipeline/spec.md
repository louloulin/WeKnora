# B-T2 — wiki-verify.yml spec

> 详见 `brief.md`,本 spec 仅列验收矩阵 + 强制 checklist。

## 验收矩阵

| ID | 测试 | 命令 | 期望 |
| --- | --- | --- | --- |
| A1 | 推送 trigger | push to `lumos0826` | workflow runs ✓ |
| A2 | PR trigger | PR to `lumos0826` | workflow runs ✓ + 红 ❌ 时 block merge |
| A3 | Go harness | `go test ... ./internal/application/service/...` | exit 0 |
| A4 | Frontend | `pnpm run type-check && pnpm run build-only && pnpm run check-i18n && npx tsx --test src/components/wiki/*.test.ts` | exit 0 |
| A5 | Smoke dry-run | 6 个 `bash scripts/smoke-wiki-*.sh`(无 BASE_URL) | exit 0 each |
| A6 | Cache hit | 第二次跑 < 6 min | total < 12 min |

## 强制 checklist(commit 前自查)

- [ ] `.github/workflows/wiki-verify.yml` 用 `actions/checkout@v6` + `actions/setup-go@v6` + `actions/setup-node@v6`(与现有 workflow 版本一致)
- [ ] 不引入任何外部 Action(只用官方 actions/* + golangci/golangci-lint-action(go-lint 已有))
- [ ] 不在 workflow 里 `git push` / 写 secrets / 上传 artifacts
- [ ] smoke 脚本**无 BASE_URL 时直接退 0**(已确认 dry-run safe)
- [ ] `concurrency` group = `${{ github.workflow }}-${{ github.ref }}` + `cancel-in-progress: true`
- [ ] `permissions: contents: read`(最小权限)
- [ ] `timeout-minutes` ≤ 20(避免 worker 占用)

## 不在 spec 内(留给后续 Build)

- cross-platform matrix(linux + mac + win)
- 真实 DB 启动 + live smoke
- 缓存命中分析(`actions/cache` hit-rate insights)
- workflow 自身的 `actionlint` 检查(单独 small Build)

## 验证命令(本机 dry-run)

```bash
# YAML 语法
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/wiki-verify.yml'))"

# 列出 smoke 跑前自检
for s in scripts/smoke-wiki-cache-observability.sh scripts/smoke-wiki-audit.sh \
         scripts/smoke-wiki-audit-correlation.sh scripts/smoke-wiki-backlinks-backref.sh \
         scripts/smoke-wiki-acl-snapshot-hash.sh scripts/smoke-wiki-cache-invalidators.sh; do
  bash -n "$s" && echo "OK: $s"
done
```
