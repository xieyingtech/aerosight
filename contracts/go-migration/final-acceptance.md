# 统一应用最终验收

核对日期：2026-09-08。变更：unify-nextjs-ssg-go-server。下表逐项覆盖六个 capability 的 69 个 Scenario；同一行列出的多个场景共享具名验证，不省略失败路径。HTTP 测试位于 apps/server/internal/httpapi，其余注明包名。全部真实 PostGIS 测试结果为 `.build/final-go-postgis.jsonl`。

## go-auth-session

| Scenario | 证据 |
| --- | --- |
| 历史账号登录、错误密码 | TestLoginCSRFRestartAndLogout；旧库切换演练使用升级前 bcrypt 账号 |
| 升级旧会话、超时会话 | TestSessionRotationExpiryAndLegacyCookieRejection；config.TestHTTPConfigBudgetsAndValidation 核对配置 |
| 重启保留会话 | 正式镜像三次进程启动保留 Cookie；TestLoginCSRFRestartAndLogout |
| 退出失效 | TestLoginCSRFRestartAndLogout；TestProjectStreamResumeOverflowAndLogout |
| 缺少 Token | TestCSRFDenialsPreserveSessionAndData；TestProductionCSRFCookiesAndLogin |
| 开发代理保护 | `.build/dev-proxy-final.log`，真实 Next rewrites，包含非法来源拒绝 |
| 有效机器回调 | TestUnifiedAlgorithmCallbacksAndAssets；正式镜像真实签发凭据的回调与重放 |
| 跨项目读取 | TestDirectoryIsolationAndJSONContracts 及 acceptance-matrix.md 各领域作用域测试 |
| 权限撤销 | TestQueuedJobDatabaseReauthorization；TestFlightHubManagerRoleAndProjectRevalidation；事务写入撤权测试 |
| 成员权限兼容 | TestPermissionsMatchFrozenLegacyMatrix（45 组旧输出）；TestIssueReadContractAndPermissions |

## go-business-api

| Scenario | 证据 |
| --- | --- |
| 原业务接口调用 | acceptance-matrix.md：99 个入口/查询映射，73 个具名测试在最终真实 PostGIS 回归全部 pass；旧包装 fixture 另有 96 场景 |
| 原服务端页面与表单 | 生产浏览器 page-states、project-workspaces、project-details、实际创建团队/项目；TestDirectoryCreationAndViews |
| 认证重定向隔离 | TestLoginCSRFRestartAndLogout 与开发代理匿名 401；页面认证状态由客户端处理 |
| 事务中途失败 | database.TestAuditedIdempotentWriteAtomicity；设备命令、算法、任务、案件、报告和适配器故障注入回滚 |
| 重复控制请求 | TestConcurrentDeviceCommandOnlyQueuesOnce；TestDeviceCommandSafetyIdempotencyAndAudit；正式 MQTT 任务一条分发/确认 |
| 断线续传、过期游标 | TestChannelResumeBackpressureAndRevocation；TestProjectStreamResumeOverflowAndLogout |
| 权限撤销断流 | 上述两项 SSE 测试验证实际连接结束及授权复查 |
| 有效回调 | TestUnifiedAlgorithmCallbacksAndAssets；正式镜像 API→outbox→HTTPS→签名回调→结果持久化 |
| 非法签名资产 | TestMediaAccessHTTPRangeAndAuthorization；TestUnifiedAlgorithmCallbacksAndAssets；开发代理签名资产拒绝 |
| 媒体鉴权 | TestMediaAuthMachineContract；真实 MediaMTX 管理 API 和生产浏览器 WHEP/HLS 播放 |
| 查询型对话 | TestChatResponsesToolLoop；正式镜像六个工具执行、中文回复与证据留存 |
| 越界工具参数 | TestChatReadToolsProjectScope；TestChatStepLimitAndFailures |
| 上游故障 | TestChatStepLimitAndFailures；TestAIProviderHealth；正式镜像 503 与活动请求取消，均不写虚假回复 |

## go-http-platform

| Scenario | 证据 |
| --- | --- |
| 请求异常、流式异常 | TestHTTPPanicBoundaries；TestSessionCommitFailureOwnsResponse；请求 ID 与单响应边界 |
| 伪造来源 | TestLoginRateUsesTrustedClientAddress |
| 急停独立 | TestExhaustedWriteLimitPreservesEmergencyAuthorization；TestDeviceCommandSafetyIdempotencyAndAudit |
| 写请求超限 | TestAuthenticatedRateBuckets；实际写额度与 Retry-After 测试 |
| 普通查询超时 | TestRequestDeadlineCancelsSQLAndRollsBack；TestSessionStoreDeadlineAndCancellation |
| SSE 持续运行 | 正式镜像 24 条 SSE 在普通 30 秒期限后全部读到新事件，再随 SIGTERM 结束 |
| 资产分段访问 | TestMediaAccessHTTPRangeAndAuthorization；TestAlgorithmAssetRangeAndTransferDeadline；TestStaticCompressionHTTPBoundary |
| 正常页面资源 | 生产浏览器严格 CSP、真实 MapLibre worker、WebRTC/HLS 解码及未授权脚本阻断 |
| 指标访问 | TestMetricsAuthorizationAndRouteLabels；observability 包业务指标兼容回归 |
| 带凭据请求 | TestHTTPAccessLogsExcludeSensitiveInput |

## single-service-runtime

| Scenario | 证据 |
| --- | --- |
| 干净运行环境 | 正式镜像 sha256:7d1cc9e8ef0c68edc3e27ee2c5eeac93ce1311e876a2367a5c74a04fb6c4062a：默认 UID 10001、Go PID 1、一个 TCP 8080 listener、无 Node/pnpm、无源码/二进制挂载 |
| 缺失前端产物 | webassets.TestStaticExportMissingAndSymlink；生产缺 embed 产物编译失败与 dev 标签独立编译证据见 implementation-evidence.md 的 7.1/2.2 |
| 现有库升级 | `.build/upgrade-rollback-7e1a2d8e4c301333/result.json`：旧 51 项账本原样保留，只增加 sessions 第 52 项 |
| 历史文件被改、并发与重复启动 | migrations.TestFreshRepeatConcurrentAndHistoricalChecksum |
| 空库初始化 | migrations.TestLegacyBaselineAndFailedMigrationRollback；database.TestBootstrapAndSharedTransactionRollback；正式容器空库启动 |
| 必要后台失败 | runtime.TestComponentFailureRevokesHTTPReadinessWhileDraining，真实消费者领取失败使 ready 非就绪 |
| 正常停止 | 正式镜像负载停止 541 ms，SSE EOF、数据库客户端归零；MQTT、调度、outbox、会话清理专用测试 |
| 任务未完成 | `.build/shutdown-recovery-7ce5d933-834b-4f2f-a2cb-4d8e5914abe3/result.json`：实际退出预算耗尽、exit 1、未提交成功，重启等待原 30 秒租约恢复，同一 run 一次提交 |
| 历史凭据 | credentials.TestCredentialEncryptionMatchesWebVector；冻结原 TS envelope 测试；适配器/算法/AI 领域加密与 AAD 测试 |
| 缺失新密钥 | config.TestHTTPConfigRequiresKeysAndOrigin；cmd/aerosight.TestDevelopmentStartupRejectsInvalidConfigurationBeforeDatabase |
| 版本回退 | 上述旧库演练实际停止 Go 后启动旧 Next 与 worker，旧密码、原项目及 Go 创建项目可读，不执行破坏性 down |

## sqlc-data-access

| Scenario | 证据 |
| --- | --- |
| 生成可重现 | 最终 pnpm build 内 db:check：sqlc v1.31.1 generated files match |
| 漂移检测 | scripts/sqlc.mjs 临时目录生成后按文件集合及字节比较，差异抛错；实施时人工漂移失败证据见 implementation-evidence.md；检查不改生成目录 |
| 空值与 ID 兼容 | TestDirectoryIsolationAndJSONContracts；provider/算法/任务/案件/报告实际 DTO 测试；旧包装 null/空数组/ID fixture |
| 项目过滤 | acceptance-matrix.md 各领域跨项目 HTTP/PostGIS 断言 |
| 混合查询回滚 | database.TestBootstrapAndSharedTransactionRollback：原 SQL 与 sqlc WithTx 同时回滚 |
| 提交一致性 | database.TestAuditedIdempotentWriteAtomicity；实际命令/API→outbox→MQTT 与算法分发 |
| 时空往返 | TestSnapshotPostGISDevicePoseAndTrack：真实 PointZ、LineString、坐标与回放筛选 |
| 真实 schema 验证 | migrations.TestSchemaSnapshotMatchesFullMigration；全部生成查询在真实迁移库运行 |
| 生成不迁移 | scripts/sqlc.mjs 只读取 schema/queries 并调用固定 sqlc；无数据库连接或迁移调用；数据库升级由 Go migrate 独立执行 |

## static-web-hosting

| Scenario | 证据 |
| --- | --- |
| 离线业务依赖构建 | 正式 Docker web-build 无业务数据库/密钥；Next 全页面 Static、333 个导出文件；check:web-boundary 和静态产物内部配置扫描 |
| 无前端运行时访问 | 正式镜像本身无 Node，登录页/CSP、会话、目录与业务 API 实际运行 |
| 构建后创建资源 | 生产浏览器先构建再创建项目、任务运行、算法运行、案件、事件，四种详情与团队详情可打开 |
| 旧链接访问、冲突参数 | TestLegacyPageRedirects；生产/开发浏览器 legacy-links 与前进后退 |
| 无效资源 | 生产浏览器缺参、跨项目/不存在详情与页面拒绝；各领域读取测试 |
| 缓存更新 | webassets.TestPageCSPHeadersAndCaching、TestStaticExportHTTP；HTML revalidate 与哈希资产 immutable |
| 未知路径与文件穿越 | TestStaticPagesGinFallbackBoundary；webassets.TestStaticExportHTTP、TestStaticExportMissingAndSymlink；媒体项目文件边界 Linux 补验 |
| 开发登录、开发实时连接 | `.build/dev-proxy-final.log`：真实 Next rewrites，Cookie/Set-Cookie/CSRF/写入/Range/HEAD/SSE 首帧和取消 |
| 生产构建隔离 | Next phase 隔离、最终静态构建与内部地址扫描；正式 Go 直连，无开发代理依赖 |

## 最终命令与证据

| 门槛 | 已通过证据 |
| --- | --- |
| pnpm check | `.build/final-check.log`：边界、类型、Web/Go 回归；包含旧契约固定输出检查 |
| pnpm build / pnpm db:check | `.build/final-build.log`：类型、静态导出、sqlc 无漂移、Go 嵌入 52 SQL/333 文件 |
| 真实数据库全量 | `.build/final-go-postgis.jsonl`：29 包、437 测试/子测试 pass，0 fail；矩阵 73 个具名测试全部 pass |
| pnpm test:migrations / test:security | `.build/final-migrations.log`、`.build/final-security.log` |
| MQTT/MediaMTX 环境型测试 | `.build/final-mqtt.log`、`.build/final-media-inspector.log`，补齐全量 Go 中 4 项依赖外部服务的 skip |
| 正式镜像全链路 | `.build/container-lifecycle-31a30e35-38e0-40a9-a3f8-8334ff987511/`；详见 single-binary-e2e.md、runtime-acceptance.md |
| 生产浏览器 | `.build/production-browser-6e320e2c-4d36-4cae-8622-d36634126ebe/`，截图、地图、媒体、页面和详情证据 |
| 旧实现基线 | `.build/legacy-baseline-final.log`：原 commit 的 285 测试 pass，无 skip；来源和范围见 baseline.md |

外部协议依旧需要其原幂等保障：测试验证可恢复事务、重复接收和本次控制链路，不声称网络中断后外部设备/供应商无条件 exactly-once。媒体、MQTT、PostGIS 是独立基础设施；应用交付只有一个 Go 常驻服务。源码保留的旧 worker 入口用于兼容维护，不在默认启动或发布镜像中运行。

主 spec 同步和 OpenSpec strict 校验在最终提交时记录于 implementation-evidence.md。本变更只实施并同步，不自动归档、不推送远端；用户原有 docs 工作文件保持原样。
