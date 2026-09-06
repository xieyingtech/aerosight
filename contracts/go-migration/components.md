# 实施依赖锁定

Go 版本为 1.26.1。直接安装和校验使用仓库 `go.mod` / `go.sum`；2026-09-06 `go mod verify` 通过。sqlc 由 `scripts/sqlc.mjs` 精确锁定 v1.31.1，生成和漂移检查均已通过。前端沿用 `pnpm-lock.yaml`，使用 pnpm 10.33.0。

| 组件 | 锁定版本 | 用途与官方来源 |
| --- | --- | --- |
| Gin | v1.12.0 | [HTTP 路由与 handler](https://github.com/gin-gonic/gin) |
| gin-contrib/requestid | v1.0.7 | [请求关联 ID](https://github.com/gin-contrib/requestid) |
| samber/slog-gin | v1.21.1 | [标准 slog 请求日志](https://github.com/samber/slog-gin) |
| go-chi/httprate | v0.16.0 | [限流](https://github.com/go-chi/httprate)，通过 net/http 接口适配 Gin，不引入 chi 路由器 |
| unrolled/secure | v1.17.0 | [安全响应头](https://github.com/unrolled/secure) |
| gin-contrib/gzip | v1.2.7 | [静态文本压缩](https://github.com/gin-contrib/gzip)，待静态托管阶段接入 |
| SCS | v2.9.0 | [服务端会话](https://github.com/alexedwards/scs) |
| SCS postgresstore | 209de6e426de（2025-10-02） | [PostgreSQL 会话持久化](https://github.com/alexedwards/scs/tree/master/postgresstore) |
| gorilla/csrf | v1.7.3 | [CSRF Token 校验](https://github.com/gorilla/csrf) |
| x/crypto | v0.55.0 | [bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt)，保留历史密码验证 |
| sqlc | v1.31.1 | [SQL 生成工具](https://docs.sqlc.dev/en/stable/)，database/sql 模式；无 GORM |
| pgx/v5 | v5.6.0 | [现有 PostgreSQL 驱动](https://github.com/jackc/pgx)，继续通过 stdlib 共享原事务 |
| Prometheus client_golang | v1.24.1 | [业务与 HTTP 指标](https://github.com/prometheus/client_golang) |
| OpenAI Go SDK v3 | v3.56.0 | [Responses API](https://github.com/openai/openai-go)，待 AI 业务迁移阶段接入 |

版本锁定和模块校验不代表所有功能已验收；压缩、AI 等组件的具体行为仍由对应未完成任务追踪。Go HTTP 与 sqlc 的已接入部分已经编译及数据库测试。运行 Go 全套测试前需执行 `pnpm prepare:server`，复制原始迁移字节到嵌入目录；`pnpm test:worker` 已自动执行此步骤。
