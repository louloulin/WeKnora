# Build B-T2 — CI 红绿灯

## 一句话

在 `.github/workflows/wiki-verify.yml` 落地一条 GitHub Actions workflow,触发条件是 push 到 `lumos0826` + PR 到 `lumos0826` + workflow_dispatch,跑 3 个 job(`go-test` / `frontend` / `smoke-dryrun`)覆盖 B19-B28 全部 harness / 全部 wiki frontend test / 全部 dry-run safe smoke script。任何 commit 失败都会在 PR 上挂红 ❌。

## 现状(Why we need it)

| 缺口 | 影响 |
| --- | --- |
| 现有 `frontend.yml` / `app.yml` / `go-lint.yml` 都不监听 `lumos0826` | 推到 `lumos0826` 后没有自动信号,只有手动跑 |
| `app.yml` 在 push main 时跑 `go test ./...`,但 B19-B28 的 harness 没法保证每次都跑到(B24+ stub 类型改动过去踩过坑) | 静默回归风险 |
| Smoke 脚本(`scripts/smoke-wiki-*.sh`)有 22 个,dry-run safe 但 CI 不调 | 没人保证 `bash -n` 之后语法还对 |
| 12 处历史 vue-tsc 错误清掉后(刚 B-T1),新 PR 没有"baseline 0 error"的强制门 | 下次还会再累积 |

## 验收(A1-A6)

| ID | 验收项 | 落点 |
| --- | --- | --- |
| **A1** | 推到 `lumos0826` 后,GitHub Actions 看到 `wiki-verify` 在跑 | `on.push.branches: [lumos0826]` |
| **A2** | PR 到 `lumos0826` 时 `wiki-verify` 也是 required check | `branches: [lumos0826]` + repo settings "Require status checks" |
| **A3** | `go-test` job 跑 B19-B28 全部 harness + cache 子包 + 反向索引 | `go test -run "TestWikiAudit\|TestInvalidator\|TestPutAcl\|TestWikiBacklinksCache\|TestWikiAcl\|TestWikiBatch\|TestCache_Invalidation" ./internal/application/service/...` |
| **A4** | `frontend` job 跑 `type-check` + `build-only` + `check-i18n` + 全部 `frontend/src/components/wiki/*.test.ts` | `pnpm run type-check` + `pnpm run build-only` + `pnpm run check-i18n` + `npx tsx --test src/components/wiki/*.test.ts` |
| **A5** | `smoke-dryrun` job 跑 6 个代表性 smoke(B23/B24/B25/B26/B27/B28 各一) | 6 个 `bash scripts/smoke-wiki-*.sh` 无 BASE_URL → 0 退出 |
| **A6** | cache `pnpm` + Go module,总耗时 < 12 min | `cache-dependency-path` + 复用 `~/.cache/go-build` |

## 关键决策(D1-D8)

| ID | 决策 | 推荐 |
| --- | --- | --- |
| **D1** | 走新文件还是修改 `app.yml` + `frontend.yml`? | **新文件 `wiki-verify.yml`**(不动现有 workflow,降低风险) |
| **D2** | 触发分支? | **`lumos0826` + `main`**,外加 `workflow_dispatch` |
| **D3** | 跑全 `go test ./internal/application/service/...` 还是只跑 wiki 子集? | **全跑**(`go test -short ./internal/application/service/...`),不强求 DB;harness 全部 in-memory |
| **D4** | cache 用 GitHub Actions 内置? | **是**(pnpm + Go module),节省 60-90s |
| **D5** | Go 版本矩阵? | **`1.22` + `1.26` 矩阵**(go.mod 写 `go 1.26.0`,但保留 1.22 兼容性) |
| **D6** | smoke 跑哪几个? | **6 个代表性**:`smoke-wiki-cache-observability.sh` + `smoke-wiki-audit.sh` + `smoke-wiki-audit-correlation.sh` + `smoke-wiki-backlinks-backref.sh` + `smoke-wiki-acl-snapshot-hash.sh` + `smoke-wiki-cache-invalidators.sh` |
| **D7** | lint 还是只跑 `go test`? | **只 `go test`**(`go-lint.yml` 已经覆盖 lint,且 `only-new-issues` 不适合 lumos0826 整支线) |
| **D8** | 不打 Docker 镜像 | **不打**(单独 `docker-image.yml` 已存在;CI 红绿灯只关心源码正确性) |

## 改动清单

| 文件 | 行数 | 用途 |
| --- | --- | --- |
| `.github/workflows/wiki-verify.yml`(NEW) | +90 | 3 个 job:go-test / frontend / smoke-dryrun |
| `docs/comet/changes/weknora-ci-pipeline/brief.md`(NEW) | (本文) | Shape brief |
| `docs/comet/changes/weknora-ci-pipeline/specs/ci-pipeline/spec.md`(NEW) | +30 | A1-A6 验收矩阵 + D1-D8 决策 |
| `frontend/package.json` | 0(无) | 已含 `type-check` / `build-only` / `check-i18n` |
| `frontend/src/components/wiki/*.test.ts` | 0(无) | 已有 6 个测试文件,直接用 |

## 范围外(明确不做)

- ❌ 不在 `lumos0826` 启用 "Require status checks" 强制门(留给用户去 repo settings 勾选)
- ❌ 不引入 `golangci-lint` 该 workflow(已在 `go-lint.yml`,不重复)
- ❌ 不跑 live smoke(需要 PG/MySQL/SQLite + 真实后端,留给 docker-compose CI)
- ❌ 不发 Slack 通知(GitHub 原生 PR 评论足够)
- ❌ 不做 cross-platform(只 ubuntu-latest)

## 风险点

| 风险 | 缓解 |
| --- | --- |
| `wiki-verify.yml` 自身有 bug 导致 CI 全挂 | workflow 文件保持简单 + `actionlint` 风格命名 |
| Go 1.22 与 1.26 之间有 build tag 行为差异 | matrix 跑全,任一失败就 fail-fast |
| pnpm 缓存命中失败,每次重装 ~2min | `cache-dependency-path: frontend/pnpm-lock.yaml` |
| 某些 harness 需要 `DATABASE_URL` | CI 不传,只跑 in-memory;`go test -short` 跳过 DB-harness |

## 排期

| 步骤 | 预计时间 |
| --- | --- |
| 写 `.github/workflows/wiki-verify.yml` + spec | 0.5 天 |
| yaml lint + 自检 | 0.1 天 |
| 推到 `lumos0826` 触发,看 GitHub Actions 跑过 | < 0.1 天(CI 异步) |
| 修任何红 → 再推 | 0.5 天 buffer |
| **总计** | **1-2 天** |
