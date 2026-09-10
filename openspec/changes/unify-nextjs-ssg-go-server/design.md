## Context

动机见 proposal.md。当前前端是 Next.js 16 App Router，检查时有 27 个 page.tsx、43 个 route.ts，另有 Server Actions、服务端数据访问、Auth.js Credentials 和 TS AI SDK。Go 使用 Go 1.26.1、database/sql 与 pgx/v5/stdlib，已有 MQTT、调度、outbox、算法回调和监控处理器。

现有业务不是简单 CRUD：包含 PostGIS、JSONB、FOR UPDATE SKIP LOCKED、advisory lock 和跨模块 *sql.Tx。历史迁移记录包含 name、checksum、adopted、execution_ms、applied_at。AUTH_SECRET 同时参与历史凭据加密，不能随 Auth.js 移除。

## Goals / Non-Goals

**Goals:**
- 生产一个 Go 应用进程和 HTTP 监听端口提供静态页面、API、回调与后台处理。
- Gin 负责 HTTP，sqlc 负责类型化 SQL，领域层保持现有权限、审计和事务语义。
- 开发由 Next dev 提供页面和 API 代理，保持热更新；生产构建直接验证静态导出限制。
- 所有现有功能完成迁移后才切换默认生产启动入口。

**Non-Goals:**
- 不合并 PostgreSQL、MQTT、MediaMTX 或外部 AI/算法服务，不代理媒体传输以强行减少其端口。
- 不引入 GORM、自动 ORM 迁移、新消息队列、Redis、额外客户端路由框架。
- 不重做 UI、权限模型、调度算法或设备协议；不兼容旧 Auth.js 会话。
- 不在本轮规划中改业务代码或执行迁移。

## Decisions

### 1. Gin 与开源 HTTP 组件

最终采用 Gin + sqlc，这是用户明确选择。此前 Chi 方案偏重已有标准 http.Handler 的直接复用；Gin 提供集成的绑定、校验、路由分组和 JSON 响应，更适合本次大量 API 迁入。已有标准处理器通过 Gin.WrapH 或外层 net/http 组合复用，领域服务不得依赖 gin.Context。

| 职责 | 组件 | 使用边界 |
| --- | --- | --- |
| 路由、JSON 绑定 | github.com/gin-gonic/gin | gin.New；ShouldBindJSON 返回错误后统一映射，避免自动写出错误破坏已有契约 |
| 请求 ID | github.com/gin-contrib/requestid | 请求与响应 X-Request-ID；过滤非法或超长输入后生成新值；向 context 和审计传播 |
| 日志 | log/slog + github.com/samber/slog-gin | 复用 JSON 日志；记录方法、模板路由、状态、耗时、请求 ID；不记录正文、Cookie 或密钥 |
| 恢复 | Gin CustomRecovery | 记录 panic；未写响应时返回契约化 500，已开始流式响应则结束流，不追加 JSON |
| 静态压缩 | github.com/gin-contrib/gzip | 仅静态文本资源；不压缩 SSE、资产下载、Range 或浏览器私有 API |
| 限流 | github.com/go-chi/httprate | 独立 net/http 中间件，不依赖 Chi 路由；登录按可信客户端地址，写 API 按用户，内存计数 |
| 会话 | github.com/alexedwards/scs/v2 + github.com/alexedwards/scs/postgresstore | database/sql 存储；认证路由及浏览器 API 外层 LoadAndSave |
| CSRF | github.com/gorilla/csrf | 包裹浏览器 API，包括登录退出；回调使用独立认证组 |
| 安全头 | github.com/unrolled/secure | 包裹外层 handler，配置 CSP、nosniff、frame 防护与 HTTPS HSTS |
| 参数校验 | Gin 集成的 github.com/go-playground/validator/v10 | DTO 格式和范围；领域授权与状态机另行校验 |
| 指标 | github.com/prometheus/client_golang + promhttp | 保留业务指标名称、标签，补充 HTTP/运行时指标 |
| 静态服务与超时 | Go embed、io/fs、net/http、context | 标准库托管；请求 context deadline 与服务器读头/空闲超时 |

中间件组合分为外层安全头与浏览器会话/CSRF包装、Gin 内请求 ID/日志/恢复、各组限流/授权/校验。使用完整 handler 包装保留 before/after 和 Flush 语义，不将标准中间件生硬转换为一个不能延续后续处理的 Gin handler。

普通 API context 默认 30 秒，AI 回合默认 120 秒，配置可调；数据库与上游调用必须传递 context。ReadHeaderTimeout 默认 5 秒，IdleTimeout 默认 60 秒；不对整个服务器设置会截断 SSE 的固定 WriteTimeout。不使用全局缓存响应式超时中间件，也不通过异步调用 c.Next 强行制造超时。SSE 和文件传输按连接取消、上游超时和分段写入期限管理。

登录默认每 IP 每分钟 10 次；普通写 API 每用户每分钟 120 次；SSE 建连按用户每分钟 30 次，已建立连接不累计请求限流。全部可配置，返回 429 和 Retry-After。急停/安全停止不共享普通业务写限流桶，仍执行认证、授权和幂等。

可信代理列表默认空，Gin 不无条件信任转发头。开发只信任 Next dev 的明确 loopback 地址；生产按配置的代理 CIDR 解析。httprate 使用经过可信规则确定的键，不使用无条件信任 RealIP 的辅助方法。生产和开发浏览器均同源，不启用宽泛 CORS。

安全头必须兼容现有地图、媒体地址和静态内联脚本：构建时从导出 HTML 汇总脚本哈希用于 CSP，脚本允许 self 与这些哈希；地图 worker、媒体和连接域采用明确配置，样式按现有 UI 所需允许内联。开发单独允许 HMR 必需策略，生产不沿用开发策略。HSTS 仅 HTTPS 开启。

### 2. sqlc 数据访问与事务

sqlc 是构建期工具，不是运行期 ORM。采用 PostgreSQL engine，Go sql_package 为 database/sql；底层仍由 pgx/v5/stdlib 提供 PostgreSQL 驱动。相比立刻切换原生 pgxpool，这样可以直接把已有 *sql.Tx 传给生成查询的 WithTx，避免同时重写所有 worker 事务接口。

目标布局：
- apps/server/sqlc.yaml：固定生成配置。
- apps/server/internal/database/queries/*.sql：按领域组织命名 SQL。
- apps/server/internal/database/sqlcgen：生成代码，提交版本库，禁止手改。
- db/schema.sql：完整数据库 schema 快照，供 sqlc 分析；db/migrations 保持有序增量迁移并做一致性检查。

新迁入的 Web 业务查询和本次新增查询全部使用 sqlc。现有 worker 稳定 SQL 不强制一次性重写；涉及迁移业务的共享查询逐步收敛到生成层。允许的手写 SQL 边界仅为迁移执行、LISTEN/NOTIFY 等控制语句、尚未触及的 worker 存量查询；不在 Gin handler 内直接写 SQL。

使用明确列投影和命名参数，保留 nullable 与 JSONB 语义。生成数据库模型不直接作为 HTTP DTO：单独映射 JSON 字段、null、日期、空数组和现有数字/字符串 ID 形状，避免 Go 序列化改变既有 API。

事务由领域服务开始并提交；业务写入、审计、幂等、outbox 和项目事件必须使用同一 *sql.Tx。禁止在 WithTx 查询链中调用基础连接池完成部分写入。权限敏感写入在事务内重新验证作用域和状态，防止权限/状态变化的时间窗口。

PostGIS 运算继续在 SQL 中完成。返回边界投影为 ST_AsGeoJSON(...)::text、ST_X/ST_Y 浮点值或显式 bytea，不直接把未知 geometry 类型暴露成 interface{}。输入使用坐标参数及 ST_SetSRID/ST_MakePoint 等现有函数；添加明确 SQL cast 使生成结果可预测。不为了类型映射引入 GEOS/CGO。PostGIS 查询使用真实 PostGIS 测试数据库验证，不以代码生成成功代替数据库执行测试。

sqlc 不负责数据库迁移。构建期固定工具版本，提交生成代码；提供 pnpm db:generate 与 pnpm db:check，后者在临时输出目录生成并比较，检查无漂移，不重写开发者文件。

### 3. 前端静态导出与开发入口

生产：Next output=export、trailingSlash=true，生成 out；复制完整产物到 Go embed 目录后构建。静态构建不读取数据库、用户会话或服务端密钥，不保留 Server Actions、动态 Route Handler、cookies/headers 或运行期 Next 服务依赖。保留现有 React、Tailwind、Radix、MapLibre；图片采用静态资源或 unoptimized，字体使用可重现的本地资源。

开发：浏览器访问 Next dev，Next 配置依据 PHASE_DEVELOPMENT_SERVER 返回 rewrites，把 /api/:path* 和 /algorithm-assets/:path* 代理到仅服务端可见的 GO_API_ORIGIN。默认 Next 3000，Go 8080，Go 绑定 loopback。开发不设置 output=export；生产构建设置 export 且不返回 rewrites。不增加手写 Next API 转发 handler，也不在 Go 中创建开发 ReverseProxy。迁移期间 rewrites 使用 beforeFiles，最终删除旧服务端 handler。

前端始终使用相对 /api URL，EventSource 经同一代理。Cookie 不设置 Domain，Path=/；开发 PUBLIC_ORIGIN 指向 Next 地址，Go CSRF 校验接受该明确来源，不关闭 CSRF。代理正确传递 Cookie、Set-Cookie、Origin、Last-Event-ID、Range 与内容类型。专门验收 SSE 首条消息、长连接和断开取消；不得用缓冲整段响应的代理实现。机器回调和健康探测开发期直接访问 Go。

生产 Go 提供已导出文件、index.html 与 404.html；静态挂载必须排在明确 API/回调之后。不做全路径首页 fallback，不暴露目录索引或任意本地文件。API/回调未知路径保持其协议的 404/405，不能返回 HTML 页面。

带哈希 /_next/static 文件 public,max-age=31536000,immutable；HTML 和无哈希构建元数据 no-cache；私有 API no-store；SSE 使用 private,no-cache,no-transform。支持正确 MIME、HEAD 与压缩 Vary，缺失 chunk 返回真实 404。

### 4. 页面路径兼容

采用固定页面路径加业务查询参数，所有基于 useSearchParams 的客户端读取使用 Suspense。固定静态页面不改变业务含义：

| 原页面 | 新页面 |
| --- | --- |
| /teams/:id | /teams/detail/?teamId=:id |
| /projects/:id | /projects/detail/?projectId=:id |
| /projects/:id/:section | /projects/:section/?projectId=:id |
| /projects/:id/tasks/runs/:runId | /projects/tasks/runs/detail/?projectId=:id&runId=:runId |
| /projects/:id/algorithms/runs/:runId | /projects/algorithms/runs/detail/?projectId=:id&runId=:runId |
| /projects/:id/issues/:issueId | /projects/issues/detail/?projectId=:id&issueId=:issueId |
| /projects/:id/events/:eventId | /projects/events/detail/?projectId=:id&eventId=:eventId |

section 仅允许当前已有 tasks/settings/realtime/assets/issues/events/devices/algorithms/connectors/agents。/projects/new、列表、登录、管理、个人资料保持固定路径。Go 对已知旧页面 GET/HEAD 进行 307 重定向，保留已有筛选和设备选择参数，路径中的资源 ID 覆盖同名查询参数；未知路径不猜测路由。使用结构化 URL 编码，不拼接未验证字符串。前端新链接、报告和 AI 引用统一改写。开发通过仅开发期 Next redirects 表达同一映射，避免旧链接在开发失效；生产不依赖 Next redirects。

### 5. 认证和业务 API 契约

新增 GET /api/auth/csrf、POST /api/auth/login、POST /api/auth/logout、GET /api/auth/session。登录接收 username/password JSON，沿用邮箱/手机号匹配及 bcrypt 校验。session 返回明确的用户信息或 401；退出成功 204；CSRF 接口返回 csrfToken。浏览器写请求通过 X-CSRF-Token 提交。错误保持现有 error 字段习惯，请求 ID 通过响应头提供，不无条件重包现有成功响应。

SCS 数据库会话最长 7 天、闲置 24 小时，可配置；HttpOnly、SameSite=Lax、HTTPS Secure，登录 RenewToken，退出 Destroy。会话仅保存用户 ID，用户与权限从数据库重新获取。旧 Auth.js Cookie 不作为新认证凭据；升级清理旧 Cookie，不迁移旧 JWT。新增 sessions 表与 expiry 索引，通过增量迁移建立。

AUTH_SECRET 保留历史加密用途；CSRF 使用单独必填的 CSRF_AUTH_KEY（32 字节编码密钥），生产缺失或格式错误启动失败。本地开发配置说明必须包括生成方法。保留现有凭据 envelope/AAD，跨 TS/Go fixture 验证解密；前端构建不包含任何密钥。

所有项目操作保留 User→Team membership→Project 的访问边界、owner/admin/member、显式权限及 event:handle 对 issue:handle 的兼容关系。用户平台 admin 不自动绕过现有项目作用域规则。后台已排队任务仍在执行前复查权限，不依赖请求时授权快照。

迁移契约清单在实现第一步形成并提交，覆盖全部 route.ts 的各 HTTP 方法、Server Actions 和服务端页面读取；每行记录旧入口、新 endpoint、DTO、权限、事务/审计/幂等和回归用例。新增页面接口按资源提供 GET /api/teams、/api/teams/:id、/api/projects、/api/projects/:id、/api/profile、/api/admin/overview、/api/admin/users、/api/admin/teams、/api/admin/projects；创建团队/项目分别为对应集合 POST。项目子资源和详情沿用 /api/projects/:id/...，复用已存在路径，不能覆盖语义不同的端点。

项目事件 SSE 保留游标、Last-Event-ID 优先于 cursor、回放上限 500、2 秒轮询、15 秒心跳及权限复查；过期游标保持 snapshot-required。连接期间每 15 秒重新确认会话有效性与权限；登出/撤权后的连接在该周期内关闭。首次写入前完成会话提交，流中不修改 SCS 会话。每轮按需借用连接，不让每条 SSE 长期占用一个数据库连接。

算法回调、算法资产签名访问和 media-auth 使用现有服务认证，独立于浏览器会话与 CSRF。文件下载保留已有授权、流式与有效 Range 语义。不为统一端口移除签名、时间窗或租户校验。

AI 使用 OpenAI 官方 Go SDK，保持已有 Responses 协议、模型/baseURL/provider 配置和凭据解密；不擅自改为 Chat Completions。迁移当前六个只读工具、最近 20 条历史、最多 8 步循环、证据引用及消息落库；工具执行复查项目范围，不扩大设备写权限。TS SDK 移除后不得留一个 Node AI sidecar。

### 6. 统一运行时与迁移

apps/worker 整合为 apps/server，Go module 名改为 aerosight/server，入口 cmd/aerosight；模拟器和 rotate-credentials 保留为维护命令，不属于常驻第二服务。HTTP、outbox、设备会话、调度、会话清理共享根取消信号，各自有资源预算和错误报告。HTTP 与 worker 连接池分别设上限，但单个事务始终只使用一个池的一条连接。

提供 aerosight serve 与 aerosight migrate。启动顺序：校验配置→数据库连接→持有现有 advisory lock 执行待迁移项→初始化→后台启动→对外 ready。初始化采用现有空库管理员规则，已有库不重置账号；开发演示凭据行为不外溢为重复重置。迁移失败不得接受业务流量。

迁移 SQL 原文嵌入二进制，保留 SHA-256、文件名顺序、基线采纳、PostGIS 检查与历史 ledger。每次应用迁移及写 ledger 在同一事务中完成；历史文件校验失败立即停止。不更换 Goose 账本、不重写历史 SQL。sqlc schema 快照与迁移后 schema 在测试中验证一致。

/healthz 判断进程存活，/readyz 覆盖数据库、存储及已启用必要后台组件；可选集成故障体现 degraded，不使无关能力全部不可用。/metrics 保留指标并要求管理员或独立抓取 Bearer token，不暴露凭据/项目 ID 高基数标签。

SIGTERM/Interrupt：取消 ready→停止领取新任务/新回调工作→结束 SSE→HTTP Shutdown 与任务排空→关闭 MQTT 和池。默认预算 30 秒。未完成任务通过现有租约/outbox 恢复，不提前标记成功；必要后台组件异常退出会使 ready 失败并触发进程协调退出。

构建顺序：前端 typecheck/静态构建→校验并复制 out 与 migrations 到嵌入目录→sqlc 漂移检查→Go build。开发服务编译通过 dev build tag 不要求前端产物，生产构建无产物直接失败；生产没有代理降级路径。pnpm start 仅启动已编译 Go，直接运行二进制等价；pnpm dev 启动 Next dev + Go。新增单应用多阶段镜像，Node 和 Go 工具仅在构建阶段，运行镜像包含 CA 和所需时区数据并挂载媒体存储。

## Risks / Trade-offs

- 静态导出将暴露原本隐藏的运行期 Server Component 依赖 → 无数据库/无服务端密钥构建作为强制验收，新增数据读取不得回流 Next 服务端。
- Gin 与标准中间件的组合可能破坏 Flush/会话提交 → 使用完整 handler 包装，验收流首字节、取消、登出后断流和错误响应。
- sqlc 对 PostGIS 自定义类型和函数推断有限 → 显式投影/cast 与真实 PostGIS 测试，保留存量 worker SQL边界，不能静默退化为无类型新查询。
- 静态开发代理可能改变 Cookie/Origin 或缓冲 SSE → 分别执行开发代理和生产直连同一组协议测试。
- 一次迁入多个业务模块可能遗漏隐含契约 → 以完整入口清单、旧实现 fixture 和现有安全测试逐项验证。
- 统一进程扩大故障影响面 → 分组 deadline/连接池预算、后台状态纳入 ready、幂等与租约恢复测试。
- 内存限流只适用于单实例 → 本次明确单实例部署，后续多副本时再引入共享计数器，不宣称全局限流。
- 本地旧 Next 实例与新 Go 混跑可能竞争初始化 → 切换窗口停旧两服务再迁移启动，禁止双版本并行写入。

## Migration Plan

1. 冻结现有入口与测试契约，固定新增依赖和 sqlc 工具版本（实现时核对兼容稳定版本并写入锁文件，不使用浮动 latest）。
2. 新增 Go HTTP/会话/迁移与 sqlc 骨架，按模块迁入 Web 逻辑和共享查询，持续验证原子性。
3. 将页面改为客户端数据加载和固定路径，启用 Next 开发代理，清除服务端依赖。
4. 完成 embed、单服务启动、镜像、监控与升级演练；所有验收通过后切换默认启动。
5. 部署前备份数据库和存储，停止旧 Web/worker，执行新二进制迁移并启动；检查 ready、登录、SSE、任务与回调。
6. 回滚停新服务后恢复旧 Web/worker 和原配置，新增 sessions 等兼容表可保留；不自动执行破坏性 down，不恢复过期业务状态。历史加密密钥继续保留。
7. 更新 README、环境配置、构建脚本和 openspec/config.yaml 的职责描述；核对其他活动变更，归档前同步本变更能力并运行完整检查。

## References

以下为选型时核对的官方文档；sqlc、Gin、Next.js、会话资料已通过 Context7 查询。
- Gin：https://github.com/gin-gonic/gin
- sqlc 配置与事务：https://docs.sqlc.dev/en/latest/reference/config.html 、https://docs.sqlc.dev/en/latest/howto/transactions.html
- sqlc 类型：https://docs.sqlc.dev/en/latest/reference/datatypes.html
- Next 静态导出与配置：https://nextjs.org/docs/app/guides/static-exports 、https://nextjs.org/docs/app/api-reference/config/next-config-js
- 请求 ID / 压缩：https://github.com/gin-contrib/requestid 、https://github.com/gin-contrib/gzip
- 日志：https://github.com/samber/slog-gin
- 会话：https://github.com/alexedwards/scs
- CSRF / 安全头：https://github.com/gorilla/csrf 、https://github.com/unrolled/secure
- 限流 / 指标：https://github.com/go-chi/httprate 、https://github.com/prometheus/client_golang
- AI SDK：https://github.com/openai/openai-go

