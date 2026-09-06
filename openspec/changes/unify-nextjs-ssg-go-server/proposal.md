## Why

当前 Next.js 承担页面渲染、认证、同步数据库操作和业务 API，Go 承担后台处理与回调，生产必须维护两套应用运行时。将前端静态导出并统一由 Go 提供页面、API 和后台处理，降低部署复杂度，同时使用 Gin 与 sqlc 建立清晰的 HTTP 和数据访问边界。

## What Changes

- Next.js 改为 SSG 静态页面壳，业务数据在浏览器通过同源 Go API 加载；产物嵌入 Go 二进制。
- Go 使用 Gin，迁入全部 Next.js Route Handler、Server Actions、页面服务端查询和 AI 调用，合并已有 worker 生命周期。
- 数据层采用 sqlc 生成类型安全的查询方法，使用 database/sql 与 pgx stdlib；保留显式 SQL、PostGIS、事务、outbox、审计和租户约束。
- 通用 HTTP 中间件采用开源组件：请求 ID、日志、恢复、压缩、限流、会话、CSRF、安全头与指标；领域授权继续执行现有业务规则。
- **BREAKING**：运行时 ID 页面改为固定路径加查询参数；Go 为已知旧页面 URL 提供重定向。业务 API 的 ID 路径保持兼容。
- **BREAKING**：Auth.js 会话改为 Go 管理的 PostgreSQL 会话，原用户需要重新登录一次，账号和密码保留。
- 开发浏览器访问 Next dev，由 Next.js rewrites 代理 API 到 Go；生产不运行 Next 服务，Go 不实现开发反向代理。
- 将 SQL 迁移与初始化移入 Go，保留历史迁移账本及校验和；生产仅一个应用进程和监听端口。

## Capabilities

### New Capabilities

- `static-web-hosting`：静态导出、嵌入托管、页面路由迁移与开发期 Next.js API 代理。
- `go-http-platform`：Gin HTTP 边界、开源中间件、流式处理、日志和指标。
- `go-auth-session`：Go 登录会话、CSRF、账号兼容和租户权限。
- `go-business-api`：同步业务 API、页面数据、SSE 和 AI 的行为等价迁移。
- `sqlc-data-access`：生成查询、共享事务、PostGIS 类型边界与代码生成检查。
- `single-service-runtime`：单应用部署、数据库迁移、后台运行和优雅退出。

### Modified Capabilities

无。当前 openspec/specs 尚无已同步能力；上述新增能力承接现有代码的运行时职责，不复制或改写其他活动变更的业务要求。

## Impact

- 前端 apps/web、Go apps/worker（实现时整合为 apps/server）、db、构建启动脚本、部署说明和 OpenSpec 项目上下文。
- 移除生产对 Node.js、Auth.js、pg、TS 服务端 AI SDK 的依赖；引入 Gin、sqlc 及 design 中列明的中间件。
- 用户可见变化限于详情 URL、首次升级重新登录与加载状态；现有业务功能、数据、权限、审计、幂等、设备控制安全边界保持。
- PostgreSQL/PostGIS、MQTT、MediaMTX、外部算法与 AI 服务仍为外部基础设施；不合并媒体传输端口，不重做 UI，不新增业务能力，不引入 ORM 或 Redis。
- 本变更的架构决策取代此前 Next.js 承担服务端职责的描述；其他活动变更继续维护自己的业务语义，实施时更新项目上下文并核对交叉影响。
- 本轮只生成规划文档，不执行实现、数据库迁移或部署。
