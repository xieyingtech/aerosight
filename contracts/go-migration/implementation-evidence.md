# 实施证据

当前仍处于迁移阶段；默认构建与启动已切换为静态导出和统一 Go 应用。未勾选任务仍需实现和验证，尚未完成发布验收。

## 2026-09-07

- 7.3 outbox 必要消费者故障：RunWithWake 将领取、失败状态持久化或完成确认的存储错误返回监督器，不再仅记录日志后永久重试；普通 handler 错误仍由原 Fail/退避/死信流程处理。领取前、批次中和确认前检查取消，正常退出不把取消视为事件失败或提前确认成功。将上一批 readiness 集成测试的模拟故障替换为真实 outbox.Store/Consumer：测试数据库可 Ping 但缺少 outbox 表，实际领取 SQL 失败，验证 /readyz 从 200 变 503，等待同伴排空期间 /healthz 仍为 200。真实 DB 的 runtime 测试及 outbox/入口全部定向测试通过，覆盖领取前取消、处理中取消、重复投递、死信、退避上限；这些取消单元测试不代替完整重启租约恢复演练，7.3 仍未完成。

- 7.3 后台失败通知：Runtime 新增 RunWithFailure，在必要组件错误或意外正常返回时先报告失败，再取消并等待其余组件，避免等待资源排空期间 readiness 仍为成功。统一入口收到报告立即撤销 ready 并取消应用；HTTP 显式绑定端口成功后才设置 ready，退出开始即停止会话清理，退出路径关闭 HTTP server。新增单元测试覆盖错误/意外退出立即报告、同伴取消、等待同伴排空、保留原始错误、正常取消不误报；真实独立 PostgreSQL 与 HTTP 验证先 /readyz 200，组件失败且同伴仍阻塞退出时 /readyz 503、/healthz 200。go test -tags dev ./internal/runtime ./cmd/aerosight -count=1 -v 全部通过，测试容器已清理。此批以受控失败组件验证监督器，未代替真实 MQTT/outbox/调度租约与生产信号退出演练，7.3 保持未勾选。

- 7.3 会话清理生命周期：根据 SCS postgresstore 官方文档关闭内置无 context 清理，保留五分钟周期、原 sessions 格式和过期条件；改由 sqlc DeleteExpiredHTTPSessions 执行有 deadline 的清理，Server.Close 取消正在运行的 SQL 并等待 goroutine 退出，重复及并发关闭安全。真实独立 PostGIS 验证仅删除过期会话、保留有效会话数据，并在 ACCESS EXCLUSIVE 表锁阻塞 DELETE 时关闭清理，确认无需释放表锁即可取消退出。单元测试覆盖周期操作超时、关闭取消及八个并发关闭调用；TestLoginCSRFRestartAndLogout 回归和 pnpm db:check 通过。HTTP/后台整体退出、必要组件 readiness 与租约恢复仍待验证，7.3 不提前勾选。再次构建完整镜像仍因 auth.docker.io 连接超时失败，记录于 .build/unified-image.log，7.5 未验收。

- 7.2 地图浏览器验收：新增 browser-map，真实测试库插入 adapter/device/observation/PointZ pose，经 Go 快照 API 和当前 ProjectMap 处理，断言 MapLibre blob worker 启动、设备点渲染且点击后右侧显示对应设备。公共 demo style 请求保留原 HTTPS 来源但通过浏览器 route 提供确定性纯背景 style，验证来源 CSP 和实际 GeoJSON 渲染链，不把公共瓦片服务可用性作为通过依据。无 CSP violation，缩放前后保留截图和画布尺寸；使用完整容纳地图的 1280×1000 视口消除全页截取离屏 WebGL 区域的异常。最终证据 .build/production-browser-3bc820af-eb9d-42e1-aab2-0f93acd8ac62/map.json、map-selected.png、map-resized.png，已查看完整背景和居中设备选中效果。完整生产浏览器流程通过并清理；直播/媒体 CSP 验收仍待完成，7.2 保持未勾选。

## 2026-09-06

- 完成 6.4：新增 browser-legacy-links，真实 Go 生产和 Next 开发入口均对 16 类旧路径逐项执行 GET/HEAD，断言 307、固定目标路径、路径 ID 覆盖重复冲突 ID、重复 layer 与中文特殊字符参数保留；五类非法 ID 返回 404。浏览器从项目列表进入新建项目的旧 URL，确认重定向详情数据，再后退/前进确认正确页面与筛选参数。生产证据 .build/production-browser-2b44ab7a-3b86-4dec-b20c-98e24ae7f48c/legacy-links.json，开发证据 .build/development-browser-a21db3e9-1377-4ee1-ba95-a61e736f7b5f/legacy-links.json；完整两套浏览器流程均通过并退出。复核 report_drafts 与 agent_read_tools 使用 projectPageURL；重跑 TestLegacyPageRedirects / TestProjectPageURLScopeAndEncoding 通过（含所有 16 类映射、尾斜线、方法、int32 边界、参数编码及 mutation 不重定向）。

- 完成 6.2：生产浏览器增加 browser-page-states 验收，覆盖项目列表、团队列表、个人页和五个管理页面（总览/用户/团队/项目/AI Provider）。使用浏览器网络 fixture 控制请求等待、返回 503 和 403，逐页断言加载提示、失败提示、移除 fixture 后重试真实 Go API 成功，以及拒绝时不渲染内容标题；项目真实空库列表显示“暂无数据”，团队/个人及管理页面在空资源状态正常加载。公共 SessionProvider 的会话 503 提示与重试恢复通过。另在真实 PostGIS 将账号平台角色降为 user，浏览器管理布局明确拒绝，实际 /api/admin/overview 返回 403，再恢复角色继续原流程。成功证据 .build/production-browser-3b57e9b2-baa9-494f-9bac-f60403e3f366/page-states.json 与 result.json；八页 fixture 验证的是 UI 状态处理，真实降权验证后端授权，不混淆两者。完整资源/会话/CSP 原流程同时通过，无 pageerror。7.2 地图/媒体加载验收仍未完成。

- 完成 6.1：同一浏览器验收脚本增加 --development / pnpm test:development-browser，通过真实 scripts/dev.mjs 的 Next beforeFiles 代理执行登录、CSRF 保护的团队/项目创建、详情重载、退出后 session 401、未登录详情跳转及登录后 session expiry 失效；开发 HTTP Cookie 为 HttpOnly/SameSite=Lax 且非 Secure。首次真实浏览器发现 Next 16 阻止 127.0.0.1 HMR，按官方 allowedDevOrigins 文档在 development phase 仅加入 PUBLIC_ORIGIN 主机名后通过，日志不再出现该拒绝。成功证据 .build/development-browser-3d3b3b2f-2905-4519-83aa-33156f5d057a；共用脚本的生产分支再次通过，证据 .build/production-browser-1a5d12a0-b8e4-4eb9-82c9-6b1701b509ed，仍验证 Secure Cookie 与 CSP 拦截。pnpm typecheck、diff 检查通过；两次运行均退出并清理。API 客户端保留同源相对路径限制、CSRF 获取/更新以及 401 失效事件，浏览器验证覆盖实际调用链。

- 6.1/6.3 生产浏览器第二批：扩展 test:production-browser，真实界面创建团队、刷新团队列表、选择默认可管理团队创建项目、导航到固定详情 query URL 并重新加载，验证构建后新增项目无需重建。通过用户菜单退出后 API session 返回 401，直接访问项目详情回到登录页；再次登录后项目仍可见，测试库将 session expiry 设为过去后刷新会回到登录。过程无 pageerror，详情页无意外 CSP violation。真实 Edge 成功运行证据 .build/production-browser-8ed406a9-9b34-4dd0-b127-e2ecd17a3db1/result.json 与 created-project.png；已查看截图确认新项目名称和统计卡片可见。地图只显示容器，不据此认定底图/worker/直播通过；开发浏览器会话和其他嵌套详情仍未覆盖，6.1/6.3/7.2 保持未勾选。

- 7.2/6.1 浏览器验收第一批：固定根开发依赖 playwright 1.61.0，新增 pnpm test:production-browser，使用已构建生产 Go 二进制（工作目录为独立 .build 测试目录）、临时 PostGIS、测试 HTTPS 终止代理和真实 Edge。验证登录表单 hydration/提交、导航后项目列表加载完成、Secure/HttpOnly/SameSite=Lax Cookie、没有 pageerror 或意外 CSP violation，注入未允许内联脚本后浏览器报告拦截且脚本未执行。最终成功证据位于 .build/production-browser-616bff9a-c0ca-4bbf-8027-856e2470b0a3/result.json 与 projects.png，已查看截图确认空列表显示完整。首次截图在加载中，补充可见“新建项目”断言后重跑成功；修复 TLS 测试连接清理，进程和容器已退出。7.2 的地图/直播以及 6.1 完整开发/生产会话生命周期仍待后续验收，任务不提前勾选。

- 7.2 第二批：使用已固定版本 unrolled/secure 的 ContentSecurityPolicy/Process 为每个 HTML 应用脚本 self + 当前页哈希策略，禁用脚本属性、eval、object 和外部嵌入本应用；保留内联样式及 blob worker。新增 CSP_MAP_ORIGINS（默认 MapLibre demo tiles）和 CSP_MEDIA_ORIGINS，加载配置时拒绝通配符、凭据、路径、查询、片段及指令注入，生产只接受 HTTPS，开发可用 HTTP；媒体/地图来源不扩展 script-src。页面 GET/HEAD/304/404、页间哈希隔离、配置拒绝测试及静态 Gin 集成测试通过。依据官方 https://github.com/unrolled/secure 与本地 v1.17.0 API 接入；Context7 未提供该库准确匹配。生产浏览器 hydration、地图 worker 和直播验收尚未执行，7.2 保持未勾选。

- 7.2 第一批：prepare:web 按导出 HTML 生成 csp-manifest.json，记录每页文件 SHA-256 和去重排序的内联脚本 CSP 哈希；Docker 构建包含生成模块。生产 Embedded 使用 Go HTML tokenizer 独立重新计算脚本哈希，拒绝清单缺失、文件/哈希漂移、未知页面及版本不匹配；静态 handler 不对外提供清单。Node 测试验证 Unicode、原始实体文本、带 > 的引号属性、换行归一化和字节差异；Go 故障注入与实际 333 文件生产 embed 测试通过，pnpm build:server 通过。此批尚未发送 CSP 响应头，地图/媒体来源配置与生产浏览器验收待后续完成，7.2 保持未勾选。

- 完成 6.6：Web 包移除 pg/@types/pg，快照与回放 core 仅保留浏览器类型；原 SQL 实现和四项历史测试迁到 contracts/go-migration/legacy-web，五个数据库维护脚本迁到 scripts/legacy-db。根包仅以开发依赖保留 pg，正式 db:migrate 仍由 Go 执行；pnpm test:web / test:security 继续执行历史测试。新增 check:web-boundary 纳入 pnpm check，扫描 252 个 Web 源文件及依赖，拒绝 pg/Auth.js/服务端 AI/next server imports、Route Handlers、Server Actions 与历史 DB 模块导入。清除 DATABASE_URL、AUTH_SECRET、CSRF_AUTH_KEY、GO_API_ORIGIN 后 pnpm build 通过（Web 无本地环境文件，333 静态文件与 52 迁移嵌入 Go）；导出 JS/HTML/TXT 中未发现这些服务端配置名。pnpm check 通过（288 Web + 4 历史测试，Go 本轮非数据库测试），frozen/offline lockfile 安装和安全测试通过。迁移回归、升级/回滚脚本及基准实际执行通过；基准 fixture 补齐迁移后必填 device_type_id，未改运行时业务。历史 SQL/回滚工具验证不代替 8.2/8.3/8.4 的统一应用生产演练。

- 完成 4.6：复核 GetProjectAccess、effectivePermissions 与 authorizeWrite 的成员/项目范围、权限别名及事务内锁定复查，并在真实 PostGIS 重跑完整 HTTP 和 Agent 测试（分别 120.255s / 0.949s）。新增 TestQueuedJobDatabaseReauthorization 九场景，覆盖当前授权、删除授权、移除成员、关闭会话、会话用户缺失、上下文过期、event:handle → issue:handle 别名，以及别名不得扩展到 issue:assign / mission:approve。拒绝场景调用真实 JobProcessor.ProcessNext，验证任务记录授权失败和完成时间、不进入 running、下一次轮询不重复领取。修复后台授权 SQL 在缺失成员时返回 NULL 导致扫描失败的问题，同时补齐后台别名查询并用 EXISTS 防止多授权行扩大结果集；缺失会话发起人按未授权处理。现有 HTTP 集成测试覆盖跨租户不可见、案件读写别名、撤权后幂等重试拒绝、任务与设备命令权限和事务回滚。OpenSpec strict 与 git diff --check 通过。

- 完成 3.4：新增真实 PostGIS schema 一致性测试，分别在独立临时库执行全部嵌入迁移和 db/schema.sql，比较应用关系、列类型/维度/SRID/默认值、具名约束、索引、触发器、视图、函数定义、序列参数、RLS 策略和枚举；排除扩展自身对象及迁移 ledger，函数文本仅统一 Git 换行符。测试实际发现并修复快照的约束名称、三个 unique constraint/index 表达差异、降序索引 NULL 排序和缺失的 device_types 外键；函数定义同步为迁移结果，不改历史迁移。扩展快照 API 集成测试：参数化 GeoJSON PointZ 写入，显式经纬高投影及 LineString GeoJSON 输出逐点比对，保留回放 bbox/类型过滤验收。真实 PostGIS 的 TestSchemaSnapshotMatchesFullMigration、TestSnapshotPostGISDevicePoseAndTrack 与 pnpm db:check 通过。本项不代替第 8 节整体兼容验收。

- 完成 4.5/6.5：新增 pnpm test:dev-proxy，通过实际 scripts/dev.mjs 启动 Next dev 与 Go dev，使用随机 loopback 端口、独立 PostGIS 容器和临时对象目录。实测 Next beforeFiles rewrites 下匿名 401、CSRF/Set-Cookie、缺 Token/非法 Origin 登录拒绝、有效登录/session、创建团队/项目、媒体 Range/HEAD、算法资产无 Cookie 签名 Range/错误签名拒绝、SSE 首帧心跳五秒内到达及客户端取消后 Go 日志确认 handler 返回、错误 Token 退出不失效/有效退出后 401。直接 Go dev /login/ 返回 404，未反向代理前端。脚本初版与增加签名资产检查后均通过；测试树与数据库已停止，日志保留在忽略目录。Windows 测试清理使用 taskkill /T，仅针对本次启动 PID，不作为优雅退出证据。结合上一批真实 TLS/dev Cookie、CSRF 拒绝矩阵和有效机器回调测试完成 4.5。再次 pnpm build 完整通过，所有页面 Static、333 文件/52 SQL 嵌入并生成 Go 二进制；out 扫描未发现 GO_API_ORIGIN/默认 Go 内部地址/服务端密钥配置名，完成 6.5 生产隔离部分。此脚本是 HTTP/协议验收，尚不替代浏览器 hydration、页面交互或生产生命周期验收。

- 4.5 CSRF 矩阵第一批：真实 PostGIS 中登录/退出/团队写入口分别测试缺 Token、格式错误 Token、其他客户端的有效 Token、非法 Origin 与 Origin:null；伪造 X-Forwarded-Host/Proto 不影响拒绝。全部返回 403/CSRF_FAILED、请求 ID 和 no-store；拒绝写入前后团队数量一致，拒绝退出后会话仍有效，正确 Token 的写入和退出仍成功。真实 TLS 服务验证 CSRF Cookie 的 Secure/HttpOnly/Path、会话 Cookie 的 Secure/HttpOnly/SameSite=Lax 与 HTTPS 登录/会话读取；开发登录测试补充 Secure=false 与 SameSite=Lax。与既有登录持久化及有效机器回调独立认证回归共四项通过（5.318s），开发 Cookie 断言单独重跑通过；Go HTTP 非数据库检查通过，临时容器已停止。真实 Next rewrites 的代理请求验收仍待，4.5 保持未勾选。

- 完成 2.2：补齐 HTTP 配置矩阵，验证默认 HTTP/worker 连接池 20/10、覆盖为 7/3、SSE 默认/覆盖额度、非法整数预算、CSRF base64 长度与 dev 默认公开来源。修复监听地址只做 SplitHostPort、错误端口可能延迟到迁移后才失败的问题，现在预先校验数字端口范围。dev 标签启动测试直接调用 serve，使用不可连接数据库地址，验证缺 CSRF、短 AUTH_SECRET、无效预算和端口均先返回相应配置错误；同时断言 dev Embedded 返回 nil 而不访问前端产物。main 使用两个独立 database.Open 分别传入预算，migrate 独立使用单连接。结合此前 7.1 缺 dist 的 dev 编译成功/生产编译失败证据及统一命令 migrate 首次 52/重复 0 项证据完成此任务。定向配置/启动测试和 Go dev 全包非数据库测试通过；开发 Next/Go 实际联调仍在第 6/8 节验收。

- 完成 4.3：将连接控制器与取消感知分段传输下沉到 httptransport，媒体与算法资产复用。算法资产的 SQL 改由 sqlc 生成，保持资源/项目/版本/available/deleted 过滤并防止 int32 转换溢出；移除算法资产整条路由的普通期限，数据库与存储读取使用配置的独立操作期限，内容阶段回到原连接 context。新增 HEAD 与 Range/416，仍逐次验证签名与作用域、不压缩私有内容。真实 PostGIS 测试验证 100ms 操作期限下首段延迟 150ms 的完整约 320KB 传输、Range 精确字节、HEAD 长度/空正文、资产表锁阻塞 504。算法回调签名/幂等/16MiB 限制和资产跨项目/版本/删除/过期签名回归、媒体慢传输/Range/HEAD、SSE 15 秒后登出断流共五项通过（22.496s）。Go dev 非数据库全包、db:check 通过，临时容器停止。结合本日已记录的慢 SQL 原子回滚、认证/会话取消、2MiB/慢正文、读头/空闲 TCP、gzip/304/Range 证据完成此任务；整体运行生命周期、CSP 和全量交付验收仍独立待办。

- 4.3 媒体文件期限拆分：媒体 access 授权接口保持普通业务期限；content GET/HEAD 移除整条路由期限，权限/资产数据库查询单独限时，完成后回到原始连接 context。ServeContent 保留 Range/HEAD，并通过取消感知 ReadSeeker 与 32KiB 分段 writer 处理传输，每段写使用真实连接控制器设置 10 秒写期限，完成后清除。确定性取消测试验证首段取消后不再读写；真实 PostGIS 媒体测试使用 100ms 查询期限和首段 150ms 延迟，约 280KB 下载仍完整成功。媒体 Range/HEAD/签名/租户回归及项目 SSE 超过 15 秒后登出断流通过（19.166s）；Go dev 非数据库全包通过，测试容器已停止。算法资产处理器仍需同类期限隔离及分段访问修正，4.3 尚未勾选。

- 4.3 流式控制器修复：准备拆分文件授权与传输期限时发现 slog-gin 的响应包装未暴露 Unwrap，原 streamWrite 的 ResponseController.SetWriteDeadline 实际可返回 ErrNotSupported 而被忽略。现在在外层 HTTP dispatch 将底层连接控制器放入请求 context，SSE 使用该控制器设置分段写期限；每帧 Flush 后清除写期限，避免把 10 秒写预算误用于 15 秒心跳间隔。真实 HTTP 测试经过完整 Gin/日志链验证写期限设置与清除成功，不接受 ErrNotSupported；静态压缩组合测试和 Go dev 非数据库全包通过。文件授权/传输期限拆分尚未实现，4.3 保持未勾选。

- 4.3 请求体与连接期限：把 API 2MiB 限制提前至 SCS/CSRF 之前，防止 gorilla/csrf 表单解析先读取超大正文；API 与算法回调写请求设置 socket 读取期限，算法回调保留独立 16MiB 大小上限。真实 HTTP 测试验证 2MiB 完整读取、超出一字节拒绝、缺 CSRF header 的大表单读取不超过 2MiB+1、只发送部分正文的连接在期限后结束读取。测试捕获并修复过早清除读取期限导致 net/http 排空未发送正文时再次等待的问题；连接后续期限由 net/http 管理。提取 main 共用的 HTTP server 构造函数，断言生产 ReadHeaderTimeout=5s、IdleTimeout=60s、无全局 ReadTimeout/WriteTimeout；真实 TCP 测试缩短相同实例期限到 100ms 后验证未结束请求头和 keep-alive 空闲连接关闭。Go dev 全包通过。继续审查发现媒体内容/算法资产仍挂普通业务 timeout，文件流期限隔离尚需修正验证，4.3 继续未勾选。

- 4.3 认证期限：检查已锁定 postgresstore 源码确认其使用无 context 的 database/sql 操作；新增 SCS CtxStore 适配，通过 sqlc 执行同格式 Find/Commit/Delete，保留嵌入 postgresstore 与清理生命周期，每次操作受父 context 和普通请求期限约束。登录/退出路由补业务期限，requireUser 的用户查询独立设置期限且不把该短期限传入后续 SSE。SCS 存储错误返回脱敏 JSON，期限错误为 504/REQUEST_TIMEOUT。真实 PostGIS ACCESS EXCLUSIVE 锁测试验证会话读取 HTTP 504、提交/删除取消、父 context 取消、解除阻塞后原会话仍有效，以及 users 查询阻塞也返回 504。登录/轮换/重启持久化/退出和项目 SSE 登出断流回归通过（19.171s）；补充 users 锁测试再次通过。Go dev 非数据库全包与 db:check 通过；临时容器已停止。服务器读头/空闲期限、请求体限制验收和会话清理停止的生命周期验收仍待，4.3/7.3 未勾选。

- 4.3 请求期限第一批：统一 failure 映射在请求 context 已 DeadlineExceeded 时返回 504/REQUEST_TIMEOUT，避免取消的任务写入误报 409 业务冲突。独立 PostGIS 故障注入在 outbox 插入触发器 pg_sleep(5)，HTTP 请求期限设为 100ms，验证两秒内取消、504、任务状态/版本原样保留，审计/project_events/outbox 全部回滚。重跑项目与频道 SSE 真实 DB 测试通过：普通 API 期限为 1 秒，SSE 持续到 5/15 秒授权复查并正常发送撤权事件，未被普通期限或静态 gzip 截断，等待期间无连接占用。三项真实 DB 测试通过（24.332s），Go dev 全包非数据库检查通过，临时容器已停止。当前期限仍从业务路由 middleware 开始，认证/会话读取的期限边界与服务器读头/空闲期限验收需要继续补齐；4.3 保持未勾选。

- 4.3 静态 gzip：使用已锁定 gin-contrib/gzip 1.2.7，仅接入 Gin 静态 NoRoute 链，排除 API/算法资产、旧链接重定向、二进制资源、HEAD、Range/If-Range 和升级请求；解析 Accept-Encoding 权重，显式 gzip;q=0 优先于通配符。静态文本按 Accept-Encoding 区分缓存，压缩响应的内容哈希 ETag 转为弱标识，保留身份编码强 ETag 和分段读取语义。真实 HTTP 服务测试覆盖 HTML/JS 解压后内容一致、压缩拒绝、HEAD 无正文、Range 206 精确字节与 Content-Range、字体不压缩、SSE 不压缩、页面/API/算法资产 404、压缩 ETag/If-None-Match 304 无正文。Go dev 全包通过（本轮未连接数据库）。普通请求取消/数据库超时回滚、读头/空闲期限和超长 SSE 的 4.3 其余验收仍待，该任务保持未勾选。

- 完成 4.2 数据库验收：独立 PostGIS 中先以普通团队写入耗尽用户额度，下一写入返回 429/Retry-After 且团队数量不变；普通任务 pause 同样限流，emergency_stop 仍完成 running→canceling。相同旧版本重试返回 409；随后撤销团队管理角色，另一个运行急停返回 403 且状态/版本不变。数据库验证仅产生一份项目审计、事件和 outbox，无拒绝/重试额外副作用。首版测试错误预期急停发布两条 outbox，核对单次 w.Publish 行为后修正为一条。既有设备安全测试增加普通额度耗尽步骤，确认普通命令 429 后返航仍通过原确认、活动任务例外与高优先级检查。真实 TLS 响应验证 nosniff、DENY、HSTS；不可信 X-Forwarded-Proto 不在明文响应开启 HSTS。定向集成测试通过；完整 HTTP 包真实 DB 回归通过（102.787s），随后新增/修改测试也已定向执行。临时数据库容器已停止。上一批伪造 IP、用户隔离、JSON 429 与 Retry-After 证据继续有效；CSP/长连接期限仍属 7.2/4.3。

- 4.2 限流接线第一批：发现 WRITE_RATE_LIMIT 原先只读取配置、未执行限制；现于 requireUser 完成认证后以用户 ID 使用 httprate 普通写桶，项目与频道 SSE 共用独立建连桶，新增 SSE_RATE_LIMIT 默认 30/min。读请求不消耗写额度，机器回调/媒体鉴权不进入浏览器用户桶。任务控制与设备命令在解析动作后使用同一写桶，emergency_stop 与 flight.return_home 不受普通写桶阻断，仍执行原权限、事务复查、安全和幂等逻辑。环境示例补齐三种额度与可信代理列表。定向测试验证默认不信任代理时轮换 X-Forwarded-For/X-Real-IP 仍命中相同登录额度，显式可信 loopback 按转发来源区分；429 JSON 与 Retry-After、跨用户隔离、读/写/SSE 桶隔离及项目/频道 SSE 共桶均通过。Go dev 全包通过，数据库测试未在本轮运行；额度耗尽后真实授权急停/拒绝越权的集成验收与安全头验证仍待，因此 4.2 不勾选。

- 完成 4.1：在既有 Gin 分组、标准 handler、请求 ID、恢复与错误映射基础上补齐 HTTP 边界测试。检查已锁定 slog-gin 源码发现，即使关闭 body/header 仍输出 path/query/params/referer，并可将 Gin error 文本作为日志 message；新增仅用于该中间件的 slog 输出筛选器，保留请求 ID、模板路由、方法、时间、长度、状态与耗时，使用固定 message 和正确 4xx/5xx 日志级别。原请求不被改写。Gin CustomRecovery 使用 nil writer 避免无用地生成请求/堆栈 dump，只记录错误类别与请求 ID。测试覆盖路径/签名 query/Authorization/Cookie/Referer/User-Agent/正文/响应 Cookie/响应正文/Gin error 敏感值均不进入日志，合法关联 ID 保留、非法/过长 ID 替换；真实 httptest HTTP 服务验证普通 panic 的 500/请求 ID、已 Flush SSE 原数据保留且结束时不附加 JSON、panic 值不泄露。定向测试及 Go dev 全包通过（本轮不连接数据库，既有 DB 测试跳过）；不以本轮结果替代限流、期限、CSP 和生产端到端剩余验收。

- 单应用镜像第一批：根 Dockerfile 分离 Node/pnpm 静态构建、Go 1.26.1 编译和 Debian/CA 运行阶段；运行层仅复制二进制，使用 UID/GID 10001、对象持久目录和单个 8080 端口，ENTRYPOINT 直接执行 Go。新增 .dockerignore 排除环境文件、源码工具缓存及本地生成物，README 给出环境/卷/迁移/停止预算说明。Docker web-build 实际通过 Linux frozen-lockfile 安装、Next 静态导出、333 个页面产物与 52 个迁移复制。完整构建在拉取 Go/Debian 基础镜像时因 auth.docker.io 网络连接失败，尚未验证运行镜像，7.5 保持未勾选。没有用前端阶段成功替代最终镜像验收。
- 统一生产二进制实测：独立 PostGIS 空库、无源码的工作目录直接启动已构建 aerosight.exe；ready、内嵌登录页和详情壳、匿名 401、CSRF 登录、会话读取、构建后新团队/新项目写入与读取、相应静态壳、旧页面 307、API/缺失 chunk 404、退出后 401 均通过。首轮测试误把数据库初始化临时 Unix socket 就绪视为 TCP 就绪，改为 pg_isready -h 127.0.0.1 后通过；无应用代码修复。测试使用 development Cookie 模式，不代表生产 TLS、浏览器 hydration 或优雅退出已验收；测试进程与数据库容器已停止。

- 统一命令第一批：pnpm build 顺序执行 Next 静态构建、迁移/页面产物校验复制和 Go 生产编译；start 启动已有统一二进制，db:migrate 使用 Go migrate，dev 协调 Next 与 Go dev 两个进程并在任一退出时停止另一进程。启动脚本读取根 .env.local，删除旧默认 worker 启动器，保留独立维护编译入口。README 与环境示例补齐 CSRF、HTTP、PUBLIC_ORIGIN 及开发代理配置。新 pnpm build 完整通过（333 个静态文件、52 个 SQL 迁移）；独立 PostGIS 中新 db:migrate 首次应用 52 项、重复应用 0 项，容器已停止。pnpm check 通过（292 项 Web 测试和 Go dev 全包；此轮未配置数据库测试变量，不能代替先前数据库证据）。三个 Node 脚本语法检查通过。运行镜像、真实默认启动及开发进程联调尚待，7.5 保持未勾选。


- 完成 7.1：prepare:web 校验关键 HTML/chunks、拒绝 symlink/non-regular，再复制 333 个 out 文件到忽略目录；生产 all:dist embed 包含 Next 下划线元数据，New 在启动时检查页面并加载至内存，serve 接入 Gin 同一端口。dev 标签返回无静态 handler，测试命令使用 dev 标签，生产构建显式要求静态产物。静态响应含稳定 MIME、ETag/304、HEAD、Range、哈希目录 immutable、HTML/元数据 no-cache、页面 404；拒绝穿越/反斜杠/目录列出，API 与算法资产不回退 HTML。Gin HEAD 404 需显式提交 header，已由组合测试捕获并修正。真实嵌入页面（根、登录、项目及构建后 ID 详情壳）与 HTTP 边界测试通过；临时移走 dist 后生产编译按预期失败、dev 编译成功，恢复后生产 aerosight.exe 构建通过；临时移走 login/index.html 后复制在修改目标前拒绝，恢复后再次复制成功。迁移 SQL 嵌入沿用已验证 prepare:server。Go dev 全包（非数据库）、生产静态/Gin 测试和 OpenSpec strict 通过。7.2 CSP、4.3 gzip、7.5 默认构建启动入口及无 Node 镜像/完整数据库端到端仍待完成。


- 新链接统一：地图选中设备/案件、设备树实时入口及存量只读工具格式器不再生成旧 ID 路径；共享 projectNavigationHref 支持结构化参数并固定调用方项目 ID。Go 报告的运行/任务版本/设备/轨迹/步骤/事件/反馈/资产引用全部改用固定页面查询参数，与聊天引用共用 projectPageURL；历史已存报告保持原内容，通过 307 兼容。新增报告八种引用地址断言及 Go/TS 特殊字符、参数覆盖、调用方参数不变测试，更新两个旧工具地址断言。TypeScript、292 项 Web 单元测试及 Go 非数据库全包通过。6.4 的浏览器前进后退/完整页面跳转验收仍待生产托管后执行，因此未勾选。


- 旧页面兼容第一批：Go NoRoute 对已知团队/项目/10 个工作台/四类详情 GET/HEAD 返回 307，路径 ID 覆盖冲突查询，保留重复筛选与 UTF-8；只接受规范正 int32/UUID，未知或非法地址不猜测，写方法不重定向。Go HTTP 测试覆盖 16 类、末尾斜杠、HEAD 空正文、缓存、ID 上下界、查询冲突和非法路径。Next 开发 redirects 使用等价范围 regex，跳过默认斜杠重定向；真实 127.0.0.1:3307 Next dev 实测 32 次 GET/HEAD、重复筛选与冲突参数、五项 404、带斜杠单次 307 通过。旧 instrumentation 的 dev 生成缓存导致首次启动失败；递归删除被自动策略拒绝后，将已校验缓存目录保留移入 .build/next-dev-before-redirect-test，重新启动通过，测试进程已停止。TypeScript/Go 非数据库全包通过。全站新链接和报告地址仍需统一，浏览器前进后退验收尚待，6.4 未勾选。


- TS 服务端实现清理：独立 web-api-types 承载 Go JSON DTO（时间字符串、不含算法私有 inputSnapshot），页面/表单不再通过服务端查询函数推导类型。删除 44 个 server-only 模块、Auth.js 实现/扩展类型及 Next instrumentation 管理员初始化，移除 next-auth/bcryptjs/server-only/ai/@ai-sdk/openai 包。删除旧 AI SDK registry 包装和两项专属模拟流测试（真实 Go Responses/health 集成测试已在 5.9 验证），保留并重命名无 SDK 的默认 provider/凭据兼容测试及维护脚本入口。pg 转至开发依赖，仍供旧维护脚本和存量 SQL core 类型使用，尚未算作 6.6 完成。TypeScript、291 项 Web 测试及无 DATABASE_URL/AUTH_SECRET/CREDENTIAL_ENCRYPTION_KEY 的静态构建再次通过；运行期业务读写与初始化已由 Go 接管，Go embed/生产端到端尚待完成。


- 首次生产静态导出通过：比对冻结清单确认全部 43 个 Next Route Handler 已有迁移记录后移除，删除无页面调用的 Server Actions；生产 phase 启用 output:export/trailingSlash，根页客户端跳转，开发 API/资产 rewrites 保留。构建发现并修正项目列表、团队列表及 AI provider 页 use client 指令位置错误（此前 tsc 无法捕获）。DATABASE_URL/AUTH_SECRET/CREDENTIAL_ENCRYPTION_KEY 为空时 next build 成功，全部页面标为 Static，生成 out；扫描 HTML/JS 未见上述配置名、GO_API_ORIGIN 或默认内部 Go 地址。FlightHub 旧 Route Handler 源码边界测试迁至 Go sqlc 写锁及无凭据读投影；293 项 Web 测试与 OpenSpec strict 通过。Auth.js、pg、AI SDK 和仅服务端 TS 库仍在仓库，类型需要解耦后继续清理；因此 6.6 未完成。Go embed、无 Node 运行与完整静态浏览器验收尚待执行，不能把本次导出成功视为单服务交付完成。


- 实时前端接线：设备命令/直播启动、频道启动、直播停止、播放授权/下载、快照刷新和回放统一使用 Go API transport，保留原 JSON/取消信号和同源媒体行为；网络/解析失败恢复操作 busy 并给出状态提示，不自动重放写请求。实时选择更新保留 projectId 和其他筛选，只替换 deviceId/streamId，新增重复参数及清空选择回归。历史媒体按项目/资源 key 隔离，避免切换时复用旧 URL；刷新失败明确提示仍显示上次快照。删除无调用方的旧事件操作/草案按钮，当前历史事件页继续只读，Go 对应兼容 API 保留。TypeScript 和全部 293 项 Web 单元测试通过；实际 SSE/代理/Range/生产浏览器联调仍在第 6/8 节，未以单元测试替代。


- 前端工作台迁移：算法与连接器页采用固定路径，移除最后两个 projects/[id] 页面及布局。算法页从项目角色/显式权限决定是否读取服务配置，目录使用 Go definitions envelope，保留普通成员查看运行/目录；服务配置写后重新加载，算法启动跳转固定详情。连接器从 Go 聚合读取连接/发现身份/同步记录，功能开关来自 Go features；DJI 与 FlightHub 写请求使用 CSRF transport，保留结构化错误和向导本地状态，DJI 网络/解析异常恢复 busy。更新旧目录断言为固定路径，导航测试从真实链接验证页面存在及 projectId，连接器测试保留成员不渲染管理工作台边界。Next typegen、TypeScript 和全部 292 项 Web 单元测试通过。尚有实时组件 transport、全站旧链接、旧 API/服务端依赖移除与生产构建/浏览器验收，第 6 节继续未勾选。


- 前端详情迁移：任务运行、案件、算法运行和历史事件详情采用固定 detail 路径 + projectId/资源 ID，统一 Suspense、参数验证和 Go API 加载。算法 ID/历史事件 ID 按 UUID 校验，数据库日期按 JSON 字符串显示，算法详情类型明确不包含私有 inputSnapshot。任务控制与案件协作通过 CSRF 客户端写入后重新读取数据，保留 expectedVersion、独立权限和错误提示；算法重试跳转固定新运行地址。证据预览改用可取消的共享 API hook，避免切换资源时展示上一个资源的 URL；旧事件列表入口客户端转至案件列表并保留查询参数。Next 路由类型生成、TypeScript 及 API/任务权限/算法诊断/案件证据 13 项测试通过。剩余算法/连接器工作台、全站旧链接兼容与浏览器生产验收未完成，第 6 节保持未勾选。


- 前端接线补齐素材列表遗漏：GET /api/projects/:id/assets 使用 sqlc 显式投影，保持旧 listProjectItems 的 available 筛选、创建时间倒序、数值 ID、nullable MIME/采集时间及 ISO 时间；不返回存储路径。真实 PostGIS TestProjectAssetListContract 验证空数组、筛选/排序/DTO、项目隔离和撤权 404。Go 非数据库全包、db:check 和 OpenSpec strict 通过。任务、案件、素材库和设置页面改为静态查询参数入口，保留表格和管理权限提示，任务模板与运行列表分别加载；Next 路由类型生成和 TypeScript 检查通过。详情链接已使用目标固定路径，详情页和浏览器联调仍待完成，第 6 节保持未勾选。


- 前端静态化第一批：登录/退出、公共会话布局、团队/项目列表与创建、团队/项目概览、平台管理和个人资料改为浏览器加载 Go API；统一加载、错误重试和访问拒绝状态。团队/项目详情及设备、智能体、实时作业采用固定路径和查询参数，查询参数读取包在 Suspense 中，侧栏项目切换与高亮适配固定路径。智能体和 AI provider 写操作使用 CSRF 客户端并在成功后重新请求数据。Next 开发 phase 添加 beforeFiles API/算法资产 rewrites，生产 export 尚未启用。next typegen、TypeScript 和 11 项 API/导航测试通过；清理了引用已删除页面的旧 .next/dev/types/validator.ts 生成缓存。剩余项目工作台、详情、旧地址兼容、浏览器联调和生产构建仍待完成，第 6 节保持未勾选，不能视为完整静态站已可部署。

- 第 6 节第一批：新增浏览器同源 API transport、typed JSON 错误、CSRF 合并获取/失效、可选认证与受保护 401 通知、登录/退出/会话方法；禁止外部/归一化后越出 /api 的路径，不自动重试业务写入，保留 Response 用于 Range/流式消费。新增 useAPI 和统一加载/失败显示，切换路径或刷新时不展示旧项目数据，卸载取消请求。7 项 transport 测试验证并发 CSRF、Header/凭据、401/403、取消隔离、不重放写入、路径限制及 HTML 错误；全项目 typecheck 通过。Next.js 静态导出限制已用 Context7 /vercel/next.js 核对。此批为页面迁移基础，尚未把旧登录/SSR 页面切到 Go 会话，也未启用 export 或完成双入口验收，6.1/6.2 保持未勾选。

- 完成 5.1 当前入口清单：复核原 4 个 Server Actions（登录/退出已由 4.4 接管，业务写入为创建团队/项目），个人资料及平台用户/团队/项目页面只有只读入口，不新增不存在的编辑功能。现有 Go/sqlc 目录 API 已覆盖全部目标；修正创建名称为 trim 后 UTF-16 最大 100，并接受项目表单的字符串 teamId。真实 PostGIS 验证空列表、团队创建/owner、项目创建和自动 Copilot、资料及管理员投影不泄露密码、非法输入、owner 插入失败时团队回滚、owner/admin 允许建项目/member 和已移除成员拒绝、平台 admin 不绕过团队管理权限、scope 列表及团队/项目/平台权限拒绝。创建继续保留原无业务审计契约，项目写入前在同一事务内锁定 manager 成员记录。Go 非数据库全包、sqlc 和 OpenSpec strict 通过。Server Action 的 Go 替代入口已就绪；第 6 节负责前端调用切换和删除原 actions，尚未宣称静态前端构建完成。

- 完成 5.11：复核 cmd/aerosight 已把 runtime.Callbacks 接入 Gin 同一端口，并给算法回调/资产组补 context deadline，继续使用原独立机器认证（不经过 SCS/CSRF）。修复回调 LimitReader 静默截断，实际正文超过 16 MiB 明确返回 413；算法资产查询排除 deleted_at 非空记录。真实 PostGIS 的统一 HTTP 集成测试通过无 Cookie/CSRF 外部签名回调、processing→completed、重复回调只存一次原始结果、同 callback ID 不同正文 409、过期/跨项目 401、超大正文 413；资产签名有效读取、篡改项目/过期 403、签名有效但项目或版本不匹配 404、软删除 404，未授权请求不触发存储读取。已有算法签名/状态机单元测试、Go 非数据库全包、sqlc 和 OpenSpec strict 通过。部署公共地址切换与生产端到端仍由第 7/8 节完成。

- 完成 5.9 当前入口清单：聊天 POST 接入官方 openai-go/v3 Responses API；保留配置模型/base URL/数据库密钥、中文系统提示、最近 20 条历史、最多 8 次模型调用及六个只读工具。无 Node sidecar；请求关闭自动重试和上游 store，逐轮带回 output message phase、加密 reasoning 与 function_call_output，每次模型/工具调用前复查 agent:use 和 open 会话，工具内再校验项目范围；最终只落库脱敏回答/精简证据。独立 AI_REQUEST_TIMEOUT 默认 120s，错误返回安全代码，响应正文上限 4 MiB，失败只保留原先已提交的用户消息，不伪造助手回复；达到八步无文本时保留原存储 fallback 和空 content 返回行为。真实 PostGIS + SDK 假上游覆盖完整两轮六工具 Responses 请求与回传/20 条历史/推理续传/模型 ID/最终证据、八步、500/超大正文/超时、非法输入/无 provider/关闭会话/越权参数/未知写工具/中途撤权。全量 Go（真实 DB）、13 项原 TS 相关测试、sqlc 和 OpenSpec strict 通过。第 6 节仍负责客户端静态化及 TS 服务端依赖移除，第 8 节负责生产单二进制端到端。

- 5.9 第四批迁移：六个聊天只读工具的 sqlc 查询、严格输入校验、递归 scope 注入拒绝、只读快照权限复查及结果格式接入 Go。保留设备/任务/案件/可用资产/轨迹/地图计数与质量字段、数据新鲜度、100 条/64 KiB 上限；证据链接直接采用设计中的固定页面查询参数，数值 ID 引用避免科学计数法。修复旧 query_issues 的 limit 与底层 query_events schema 不一致问题，设备 ID 筛选和任务/案件 limit 在查询中生效；检测关联字符串拒绝超出 bigint 范围，避免坏链接破坏案件查询。真实 PostGIS 覆盖六工具空值/两项目隔离/撤权拒绝、设备筛选、案件 limit、仅 available 资产且不泄露 storage_key、两点 LineString 和项目计数；格式测试覆盖嵌套注入、写工具拒绝、引用 URL 编码、记录/字节上限及新鲜度。原 TS 3 项、会话集成回归、Go 非数据库全包、sqlc 和 OpenSpec strict 通过。Responses 编排尚待接入，5.9 保持未勾选。

- 5.9 第三批迁移：会话创建 POST、用于替代 SSR 的会话列表 GET，以及聊天内部消息追加/最近历史查询迁入 Go/sqlc。列表在同一只读快照中校验 agent:use，仅返回当前用户最新 50 个会话和对应消息；追加在审计事务中锁定用户的 open 会话并重新授权；历史限定最新 20 条 user/assistant，再按 ID 正序。复用原最小留存规则：临时 URL/API key/Authorization 脱敏（含 JS Unicode 空白字符）、UTF-16 长度限制、最多 50 工具记录及每条 100 个证据引用，不保留原始参数/结果和临时引用 URL。真实 PostGIS 测试覆盖空数组/null/ID/时间、跨用户/跨项目、关闭/撤权拒绝、列表/历史上限及插入故障下消息和审计整体回滚；原 TS 2 项、Go 非数据库全包、追加脱敏单元测试、sqlc 和 OpenSpec strict 通过。消息追加是供下一批编排调用的内部方法，尚未把未完成的聊天 POST 暴露为成功；Responses 循环和六个工具仍待实现，5.9 保持未勾选。

- 5.9 第二批迁移：AI provider test 使用固定的官方 openai-go/v3 Models.List，关闭自动重试，保留原 GET /models、10 秒上限和 HTTP 状态健康结果；只检查状态并关闭原正文，不缓存或泄露上游错误内容。自定义客户端固定已校验的公网 DNS 地址，禁用代理、保留原主机名 TLS 校验并拒绝重定向；provider 行锁、管理员角色锁与平台审计覆盖健康写入。修正创建缺密钥错误码为原 AI_PROVIDER_API_KEY_REQUIRED。SDK 假上游验证 200/204/401/429/500 单次请求、请求密钥/路径、deadline、取消和不读取正文；真实 TLS 验证固定地址连接、目标变化拒绝和重定向拒绝；真实 PostGIS 验证健康落库、审计、私网拒绝及管理员权限。全量 Go（真实 DB）、sqlc 和 OpenSpec strict 通过。SDK 文档经 Context7 /openai/openai-go 核对。聊天会话、Responses 循环和六个只读工具尚待实现，5.9 保持未勾选。

- 5.9 第一批迁移：平台 AI provider 列表、创建、PATCH、DELETE 接入 Go/sqlc。保留 OpenAI 类型、默认项必须启用、空密钥保留原 envelope、替换密钥重置 untested、平台 AAD 和 write-only 凭据、bigint 字符串/null/时间格式；复用 HTTPS 与全部 DNS 地址校验。注册表事务锁序列化默认切换，用户角色行锁完成写入前再授权，业务和平台审计整体提交。真实 PostGIS 测试覆盖 4 并发默认创建、密文兼容解密、空白更新、加密失败时默认项/审计回滚、删除不存在项及 URL 校验期间撤权；相关算法 provider 回归、Go 非数据库全包测试、原 TS 2 项、sqlc 和 OpenSpec strict 通过。官方 SDK 测试端点、聊天会话和六个只读工具尚待实现，5.9 保持未勾选。

- 完成 5.8 当前入口清单：直播启动迁入 Go/sqlc，锁设备后校验在线/可用能力/显式授权、选择频道、同频道重放和驱动并发额度；DJI 拓扑生成 video_id，验证适配器推流凭据，只把无密码的服务端 RTMP 目标写入命令，原子提交会话/命令/outbox/项目事件/审计。保留现有 Route Handler 不接收 taskRunId 的行为；模拟器直接 live，DJI requested。启动/停止统一 device→session 锁顺序。真实 PostGIS 覆盖 4 并发只建一次、第二频道额度、离线/无能力/缺频道/拓扑/凭据拒绝、同频道重放前再授权、DJI 参数/优先级、outbox 失败全回滚、并发启动/停止及跨项目。全量 Go（真实 DB）、额外并发测试、原 TS 13 项、sqlc 和 OpenSpec strict 通过。媒体 Linux 补验使用本地 Go 交叉编译测试二进制，在 bookworm 容器实际执行 TestMediaProjectFileBoundary，符号链接逃逸分支通过（无 skip）；此前 Windows 权限限制的补验已完成。前端切换与生产端到端仍由第 6/8 节覆盖。

- 5.8 第四批迁移：停止直播 POST 接入 Go/sqlc；事务内复查 mission:operate 并锁会话，保留 DJI stopping/45 秒租约/优先级 30 停止命令、模拟器及失败会话直接 stopped、重复停止重放和 replay 模式拒绝。命令冲突返回实际 ID，修复旧逻辑派发不存在 UUID 的边界；命令/outbox 与状态/审计原子提交。真实 PostGIS 验证 4 并发仅一条停止命令/派发事件、已有命令不重复派发、模拟器与 failed 行为、outbox 故障回滚、跨项目/权限拒绝，并重跑播放及媒体鉴权测试；Go 非数据库全包、sqlc 和 OpenSpec strict 通过。直播启动仍待迁移，Linux 符号链接补验仍待执行，5.8 保持未勾选。

- 5.8 第三批迁移：浏览器直播 playback API 接入 Go/sqlc；在事务内锁授权与会话，复核 stream.* 能力显式 deny/allow，签发 60 秒候选并更新 locator 期限。保留 requested/stopped/无 ref/无协议原因、模拟器 locator、WebRTC→HLS 顺序、数值 session ID 与 null 字段；模拟器固定签名与原 TS 字节一致。真实 PostGIS 测试验证过期 locator 刷新、签发 DJI token 通过 media-auth、owner 显式 deny、成员独立能力授权、停止后不可用、跨项目与撤权拒绝；Go 非数据库全包、sqlc 和 OpenSpec strict 通过。直播启动/停止仍待迁移，5.8 保持未勾选。

- 5.8 第二批迁移：media-auth 机器接口与播放 token 签发/验证核心。按原接口区分管理员 API、绑定活动 live_stream 的适配器推流凭据、read/playback 的路径/协议/期限 token；额外 MediaMTX 字段不影响解析，不依赖 Cookie/CSRF，也不创建浏览器会话。保留 MEDIA_ADMIN_USER/PASSWORD 原字符串；加密凭据沿用 device-adapter AAD。固定时间 token 与原 TS 字节一致。真实 PostGIS 测试覆盖管理员凭据、推流状态/路径/错误密码/错误 AAD、query token、协议/路径越界与过期，重跑资产访问集成测试；Go 非数据库全包、原 TS 13 项直播核心测试、sqlc 与 OpenSpec strict 通过。直播启动/停止及浏览器播放地址签发 API 仍待迁移，5.8 保持未勾选。

- 5.8 第一批迁移：媒体 access/content 接入 Go/sqlc。签发与读取分别复核项目和敏感下载权限，已发布 evidence_links 也触发敏感规则；敏感下载签发使用事务审计，签名失败回滚审计。HMAC 与原 TS 固定样本逐字节一致，保留 120 秒 TTL、动作/项目/资产绑定及 NFKC 下载名。内容通过限定项目根的文件句柄流式输出，拒绝路径穿越/反斜杠/目录别名/符号链接逃逸，并支持 Range、HEAD、私有 no-store。真实 PostGIS 测试覆盖全文/部分与后缀范围/416/HEAD、过期和动作篡改、敏感下载/权限别名/审计回滚/撤权后旧签名拒绝/跨项目；原 TS 3 项测试、Go 非数据库全包测试、sqlc 与 OpenSpec strict 通过。Windows 无创建符号链接权限，该分支测试未执行，需在 Linux 生产回归中补验；词法路径拒绝已验证。直播启动/停止/播放及 media-auth 仍待迁移，5.8 保持未勾选。

- 完成 5.7 当前入口清单：provider 列表/创建/PATCH/测试已接入 Go/sqlc，配置读取原 ALGORITHM_ALLOWED_HOSTS。保留 write-only 加密凭据、空白更新保留 envelope、认证类型切换必须提供匹配凭据、默认 disabled、不支持的 adapter 显式拒绝；测试端点按原逻辑只验证适配器与 HTTPS/allowlist/全部 DNS 地址，不虚构远端推理探测。复用 Go 能力表与原凭据 AES-GCM/AAD；写事务在 DNS 完成后再授权。真实 PostGIS 覆盖创建/更新/密文兼容解密/空白保留/认证切换/跨项目/无密钥回滚/校验期间撤权，单元测试覆盖通配主机、混合地址与 IPv4-mapped 私网拒绝；全量 Go（真实 DB）、原 TS 11 项相关测试、sqlc 和 OpenSpec strict 通过。算法 provider/definition/run/retry 业务入口完成；前端调用切换和最终端到端仍由第 6/8 节完成。

- 5.7 第二批迁移：算法定义 POST/PUT 与动态目录 GET 接入 Go/sqlc。配置保留通用 JSON Schema、configuration/version 输入兼容、默认映射/显示元数据、阈值校验和 UTF-16 文本长度；事务锁 provider/definition，分配递增配置版本并原子退役旧快照、发布新快照、更新当前指针及审计。保留创建返回 bigint 字符串、更新返回数值 definitionId 的原端点差异。真实 PostGIS 测试覆盖目录空数组/null、禁用 provider 可见性、4 并发保存版本连续且仅一个 published、插入快照失败回滚创建/改名/退役/审计、跨项目及撤权拒绝，并重跑算法运行集成测试。对应 TS 3 项测试、Go 非数据库全包测试、sqlc 漂移和 OpenSpec strict 通过。Provider 管理与测试接口仍待迁移，5.7 保持未勾选。

- 5.7 第一批迁移：算法运行创建、列表、详情/attempt 和失败重试接入 Go/sqlc。创建在事务中锁定当前发布配置、启用 provider 和可用资产，固定 SHA-256/版本/参数，并原子提交运行、审计和请求事件/outbox；重试保留源配置与参数，只允许 failed/timed_out。修复旧 TS 重试复制旧 runId 导致 Go processor 拒绝快照的问题：重试使用新 runId/请求上下文，清除旧 callback 和资产签名，worker 再签发。原 SSR 页面只展示安全诊断；新浏览器 API 不返回私有 inputSnapshot，保留安全 view 和标准化结果。真实 PostGIS 测试覆盖创建、worker Input 解码及范围一致性、详情/attempt/null、输入别名、跨项目、禁用 provider、缺 checksum、撤权、重试及 outbox 故障全回滚；对应 TS 4 项测试、Go 非数据库全包测试、sqlc 漂移及 OpenSpec strict 通过。Provider/definition 配置管理尚待迁移，5.7 保持未勾选。

- 完成 5.6 当前入口清单：案件 actions 的 comment/status/labels/assign/unassign 已迁入 Go/sqlc，保留 clientKey 重放、版本冲突、负责人 no-op、UTF-16 长度限制与可见 Copilot 提及规则。事务内重新读取受锁保护的权限，作用域校验负责人，并原子提交案件版本/活动/负责人/Copilot 会话和任务/审计/项目事件；issue.updated 不加入 outbox。真实 PostGIS 测试覆盖开关状态、标签去重、负责人及 Copilot、无 agent:use 时普通评论、权限别名、撤权后重试拒绝、跨项目、4 请求同键只执行一次及末端事件失败整体回滚；全量 Go（真实 DB）、原 TS 9 项对应测试、sqlc 漂移与 OpenSpec strict 检查通过。复核 app/API/组件调用：旧事件 actions/agent-drafts 原本固定 410，generateOnDemandAlertDraft 和 AgentEventDraftButton 没有应用调用；案件草稿由现有 Go issue_copilot 后台链路生成并由已迁移详情读取，不新增未暴露的 TS 草稿工具端点。前端切换及端到端验收仍由第 6/8 节完成。

- 5.6 第二批迁移：案件列表和完整详情读取，包含活动/链接/检测/资产/有效负责人/项目成员/活动 Agent/关联草稿。sqlc 保留原查询排序、资产筛选、非法数字检测链接处理及 bigint 字符串，在同一只读快照中完成。真实 PostGIS 测试覆盖空关联集合、系统活动、直接关联资产、负责人及草稿、无地图坐标、跨项目拒绝和 event:handle → issue:handle 权限别名（不扩展指派/Agent 权限）；sqlc 和 Go 非数据库测试通过。案件协作写入与草稿操作尚待迁移，5.6 保持未勾选。

- 5.6 第一批迁移：旧 perception event 详情只读 API，保留检测证据投影、位置说明、历史反馈、ID/时间序列化及原有说明文字。核对实际 Route Handler 后保留 actions/agent-drafts 的 410 LEGACY_EVENT_READ_ONLY；没有复活未被调用的 handlePerceptionEvent 写入服务。真实 PostGIS 测试验证详情空证据/反馈、group bigint 字符串、跨项目/撤权拒绝，两个废弃写接口不改变事件状态或写审计。原 TS 证据显示两项测试、sqlc 检查和 Go 非数据库测试通过。案件读取/协作/草稿仍待迁移，5.6 保持未勾选。

- 完成 5.5 当前入口清单：新增任务模板列表/详情 Go 查询，测试空数组、nullable description、触发类型、跨项目及成员删除后拒绝。核对 app 页面/Route Handler/Server Actions：任务定义只有读取入口；task-versions.ts 的 createTaskDraft/publishTaskDraft/listTaskVersions 无应用调用，没有把未暴露的 TS 工具函数新增为公开 API。现有任务运行控制、工作台、审计、演练与报告入口已迁移，后台调度/执行继续复用 Go。全量 Go 真实 PostGIS 回归、sqlc 和 OpenSpec strict 通过。前端调用切换及旧 TS 清理继续由第 6 节完成，完整端到端验收仍由第 8 节覆盖。

- 5.5 第五批迁移：报告草稿聚合创建。单 SQL 快照读取任务/版本/设备、步骤、窗口内轨迹、关联事件与反馈、有效资产，生成缺口和可追溯结论；新草稿时间值及时间证据版本沿用 Go ISO UTC 规范，已有报告版本不改写。事务内复核权限及任务状态版本、锁报告记录分配递增版本、退役旧草稿，并原子写入证据/审计。真实 PostGIS 测试覆盖非终态拒绝、缺口/资产证据、证据写入故障回滚旧草稿退役与新版本、重复生成递增版本、跨项目拒绝；聚合测试覆盖全部八类证据及人工结论。sqlc 检查和 Go 非数据库测试通过；完整正向轨迹/事件数据集的端到端验收仍归入 8.1。任务定义尚待迁移，5.5 保持未勾选。

- 5.5 第四批迁移：报告发布与 JSON 附件导出接入 Go/sqlc。发布锁定报告及草稿，事务内校验 mission:operate、完整性、版本切换、证据资产去重保留及审计；导出在只读快照内校验独立 report:export，仅导出当前发布版本，保留文件名/类型/private no-store。真实 PostGIS 测试覆盖草稿不可导出、不完整确认、failed 禁止发布、资产保留失败全回滚、重复发布、旧版本退役与最新版本导出、跨项目及独立导出权限；sqlc 检查、原 TS 四项报告测试和 Go 非数据库测试通过。报告草稿聚合创建及任务定义尚待迁移，5.5 保持未勾选。

- 5.5 第三批迁移：GET task-runs 列表和单次运行工作台，使用 sqlc 显式 JSON 投影及共享只读快照事务。保留时间/ID/null/空步骤、最新命令及操作顺序，分别计算 mission:operate 与 mission:approve。真实 PostGIS 测试覆盖运行列表、无关联设备、无步骤、关联版本/设备/步骤、最新命令、成员无控制按钮、独立审批权限与跨项目拒绝；sqlc 检查和存量 Go 非数据库测试通过。任务定义与报告尚待迁移，5.5 保持未勾选。

- 5.5 第二批迁移：任务 audit-trace 与 emergency-stop-drill 接入 Go/sqlc。审计查询在只读 Repeatable Read 事务内鉴权和读取请求/预检/审批/命令最新尝试，保留 bigint 字符串、null/空数组、缺失项顺序及最后一条高优先级命令的安全状态。演练只允许 dryRun，使用事务审计并保留 ACK/NACK/timeout/disconnected 结果；真实 DB 验证五次演练不生成设备命令/项目事件/outbox、不修改任务状态和版本，拒绝越权；审计集成测试覆盖空轨迹、agent 来源、最新尝试、策略 ID 类型与跨项目拒绝。Go 单元测试覆盖审批未完成、缺少尝试及缺少能力时不虚报确认；原 TS 六项审计演练测试通过。任务定义、工作台与报告仍待迁移，5.5 保持未勾选。

- 5.5 第一批迁移：任务控制 POST 接入 Go/sqlc，保留 pause/resume/cancel/emergency_stop/approve、乐观版本、排队取消、取消中重复急停及原事件类型；写事务内复查独立的 mission:operate/mission:approve 权限。真实 DB 测试覆盖 4 并发同版本仅一次生效、跨项目、撤销操作权限、状态转换拒绝、outbox 故障回滚状态/版本/审计/事件、审批不推进任务版本、不产生队列副作用，以及数据库审批分离/过期/重复审批约束。任务定义、工作台、审计轨迹、急停演练和报告尚待迁移，5.5 保持未勾选。

- 完成 5.3 剩余连接测试和 DJI 设置 API。Go 网络校验保留 LAN/public 策略、全 DNS 结果校验、五类协议、服务端验证与设备待验证区别；实际探测固定已校验 IP、HTTP HEAD 不跟随重定向、TLS 校验证书与主机、5 秒探测上限受请求取消控制。单元测试覆盖混合私网/环回 DNS、IPv6、脱敏、HTTP Host/IP 固定、重定向、503、无效证书及取消释放连接。真实 PostGIS 测试验证 DJI 网络/适配器/凭据原子创建、六个 MQTT topic、兼容 envelope、配置占位符、有效/失败健康状态、模拟器与无网络配置、写失败整体回滚及探测期间撤权。原 TS adapter-policy/network-profile/connection-check 共 16 项测试通过；全量 Go（真实 DB）、sqlc drift 与 OpenSpec strict 通过。设备 API 清单已覆盖；前端移除旧 Route Handler 属于 6.6，尚未执行。

- 设备适配器列表、创建、启停、DJI 凭据更新及发现绑定已迁入 Go/sqlc。保留 ID 字符串/数值的原端点差异、null/空列表、空白凭据不修改、密码原值、重复绑定及能力版本递增语义；权限在写事务内复查，凭据沿用原 AES-GCM/AAD。真实 PostGIS 测试覆盖 4 请求并发绑定仅创建设备一次、未知类型回退、跨项目/非管理成员拒绝、能力写入故障回滚、凭据兼容解密与错误作用域拒绝、嵌套 config 密钥拒绝、无加密密钥时创建及审计整体回滚。适配器连接测试与 DJI 设置仍待迁移，5.3 保持未勾选。

- 设备命令 API 已迁入 Go：保留原命令账本幂等语义、显式 deny 优先、在线与任务冲突检查、高风险确认、返航优先级和队列事件。使用事务内最新成员角色，并锁住设备/能力/授权记录。真实 DB 测试覆盖确认、离线、任务冲突、返航例外、重试返回原命令、重试前撤权、跨项目、回放拒绝、输入校验、outbox 故障整体回滚；4 个并发同键请求只生成一条命令和一条 dispatch 事件。5.3 的适配器配置/发现绑定/连接测试/DJI 设置尚待迁移，该任务保持未勾选。

- FlightHub HTTP 迁移：上游项目发现/校验、连接器创建/列表/发现活动、token 更新、同步与断开。复用官方中国区 Go 客户端、原 AES-GCM envelope 和 worker token resolver；配置保留项目翻页上限。真实 DB 测试覆盖重复创建回滚、同步去重、token 解密兼容、断开禁用绑定/取消待执行和运行中同步/保留成功历史、跨项目 ID 拒绝、管理角色限制、上游期间撤权、输入严格校验、上游凭据错误、发现限流和失败审计。审计不包含 token。模拟 HTTP transport 验证官方 origin 与 X-User-Token 请求契约。
- 修复取消 pending 同步的数据库约束冲突：没有 started_at 的待执行记录取消时同时补齐 started_at 和 finished_at；已开始记录保留原 started_at。此路径已在断开集成测试覆盖。

- 业务写边界新增 `AuditedWrite`、`AuditedPlatformWrite`、事务内 `Idempotent` 和 `Publish`；生成查询通过 WithTx 共享原始事务。真实数据库故障注入覆盖业务、项目事件、outbox 插入、幂等完成、审计完成失败时所有记录回滚。8 个并发同键请求仅执行一次；不同请求复用键拒绝；撤权后不返回历史幂等成功。
- `audit-hashes.json` 从原 TS 实现生成，记录采样 Node 的 zh-CN locale。覆盖嵌套/空值、Unicode、HTML 字符、整数属性名、大数字及字面转义。新哈希固定 English 排序；旧幂等哈希匹配支持库提供的语言排序，测试验证中文旧哈希仍能重放相同请求。

- 原 `pnpm test:web`：285 项通过。目录迁移后的 `pnpm test:migrations` 重跑通过，覆盖 PostGIS、空库、已有库及重复执行。
- Go 迁移器集成测试：并发迁移锁、原始字节 checksum、历史篡改阻止待执行迁移、已有基线收养、失败 DDL 回滚。
- Go 初始化测试：重复启动保留管理员密码；sqlc 与原始 SQL 共享事务并整体回滚。
- `pnpm db:check`：通过；人为修改生成结果后检查正确失败且没有覆盖改动，恢复后再次通过。
- HTTP 数据库测试：登录 CSRF/Origin 校验、持久会话重启恢复、退出后失效、团队/项目 JSON null 与空数组、跨团队读写拒绝。
- 原 bcryptjs 生成的密码 hash 登录成功；二次登录轮换 token 并使旧 token 失效；Auth.js Cookie 拒绝；数据库会话过期后返回 401。
- 项目 SSE 集成测试：502 条事件触发 snapshot.required；Last-Event-ID 优先于 query cursor；按数值游标续传；等待期间连接池 InUse 为零；退出后 15 秒复查断流。
- 频道 SSE 集成测试：502 条样本触发 backpressure_limit_exceeded；从 501 续传第 502 条；等待期间无连接占用；新增显式 deny 后 5 秒复查断流；再次访问返回 404。
- 项目快照测试：空图层、诊断 nullable deviceId、隐藏 role、撤权 404、显式 deny、离线设备操作禁用、依赖降级；真实 PostGIS PointZ 坐标、双点 LineString 轨迹和在线新鲜度通过。
- 设备/回放测试：设备列表和树读取、循环关系终止、回放类型与 bbox 筛选、非法时间窗与 bbox、2002 条事件截断为 2000 条并标记 truncated，以及四个读取入口撤权后返回 404。
- Prometheus：原业务指标目录、标签拒绝和 gauge 语义测试通过；HTTP 测试验证匿名拒绝、Bearer 访问、路由模板标签及 Go runtime 指标。
- 当前 `pnpm check` 通过：TypeScript 类型检查、285 项 Web 测试、Go 全套测试（新增数据库测试使用真实独立 PostGIS 服务）。`pnpm db:check`、`go mod verify`、OpenSpec strict 校验通过。未把这些阶段性检查作为未迁移业务或生产静态构建的验收。

新集成测试使用独立 PostGIS 测试容器，通过 `AEROSIGHT_MIGRATION_TEST_DATABASE_URL` 指定测试服务，并为每项测试创建和清理随机临时数据库；未设置该变量时会跳过，不代表数据库行为已经验收。
