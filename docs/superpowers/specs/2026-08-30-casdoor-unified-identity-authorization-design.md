# Casdoor × OpenBuddy × WeKnora 统一身份与权限设计

## 目标与边界

Casdoor 是统一身份控制面：负责 OIDC 登录、组织、成员、角色、权限、禁用和审计事实。OpenBuddy 和 WeKnora 是独立资源服务，分别在自己的进程内执行最终授权；两者不共享 access token 或 refresh token，也不把 token 放入 URL、Renderer 或日志。

稳定主体键为 `issuer + sub`。email、username 和 Casdoor `isAdmin` 仅用于展示或显式迁移，不能作为长期主键或自动管理员映射。所有未知 claim、未知组织和未知权限默认拒绝。

## 统一权限词典

| 权限 | 作用 | 执行方 |
|---|---|---|
| `weknora.platform.admin` | WeKnora 平台管理 | WeKnora |
| `weknora.workspace.read` | 读取指定空间 | WeKnora |
| `weknora.workspace.contribute` | 写入/执行空间内容 | WeKnora |
| `weknora.workspace.admin` | 管理空间成员与配置 | WeKnora |
| `weknora.workspace.owner` | 空间所有者能力 | WeKnora |
| `openbuddy.team.workspace` | OpenBuddy 团队工作区 | OpenBuddy |
| `openbuddy.protected.resources` | OpenBuddy 受保护资源 | OpenBuddy |

权限必须整值匹配，禁止通过字符串包含、后缀截断或 `isAdmin` 推导平台权限。Casdoor 组织只有在显式 `organization -> tenant ID` 映射存在时才进入 WeKnora；不会因首次登录自动创建生产租户。

## 登录与会话

1. 客户端使用 Authorization Code + PKCE（S256）；WeKnora Web 回调由后端处理，OpenBuddy 回调由主进程处理。
2. 服务端校验 discovery issuer、JWT 签名算法（RS256）、JWKS `kid`、`iss`、`aud`、`exp`、`iat`、`nonce` 和注册的 redirect URI。
3. UserInfo 只能补充非授权 profile 字段；授权字段以已验签 ID Token 为准。
4. 本地系统签发本地会话令牌；跨产品访问必须使用短期、固定 audience 的 token exchange，不携带 refresh token。
5. Casdoor 禁用/删除或绑定被撤销时拒绝登录。OIDC 撤权只影响 `role_source=oidc` 的成员和 `oidc_managed_system_admin`，人工本地授权不被覆盖。

## 授权模型

请求上下文至少包含 `issuer`、`subject`、`tenant_id`、`session_id`、权限集合、策略版本和 trace ID。资源服务执行 deny-by-default：先验证主体和租户绑定，再验证资源权限，最后执行操作。拒绝响应只返回稳定 reason code，不泄漏资源是否存在。

WeKnora 继续以 `tenant_members` 和知识库 ACL 为最终事实；OIDC 同步仅更新外部托管成员。最后一位 Owner 不得被同步或撤权操作移除。OpenBuddy 的 session 必须绑定 `{tenantId, subject}`，切换租户或主体时清除不匹配的本地会话。

## 配置与发布门禁

生产使用 HTTPS、独立 Casdoor application、精确回调 URI、最小 scopes（`openid profile email offline_access` 按需启用），client secret 只注入运行环境。首次上线保持 `OIDC_AUTH_SYNC_ROLES=false`，完成组织映射、权限矩阵和回滚演练后再逐租户启用。

上线前必须验证：新用户、邮箱变化、组织变化、角色升降级、禁用、撤权、JWKS rotation、跨租户读写拒绝、旧 session 拒绝、OpenBuddy/WeKnora token 隔离、审计关联和迁移回滚。未完成 Casdoor 专用 application、secret、回调和真实登录验收前，不能宣称端到端接入完成。

## 变更与回滚

身份绑定表使用 `issuer + subject` 唯一约束，并支持 `revoked_at`。数据库迁移必须同时提供 PostgreSQL/SQLite up/down 文件。回滚先停止角色同步，再回滚应用，最后执行对应 down migration；若已有外部绑定，先导出审计和映射数据，避免误删本地用户。
