# 真实验收证据

本文件只记录脱敏的验收场景、可重复命令与结果，不保存 Token、SN、UUID、坐标、临时 URL 或上游响应值。

## 10.2 真实只读接口验收

- 日期：2026-09-02
- 范围：中国公有云、本地已连接司空连接器、全部 59 个 released GET。
- 结果：59/59 endpoint 已写入 `live-read` capability evidence；合法空状态、参数要求、权限/错误类别均保留为安全分类。
- 安全检查：evidence 敏感详情扫描为 0；`field-write` 记录未被创建或扩大。

## 10.3 状态流与地图回填

- 日期：2026-09-02
- 范围：项目 1123 的一台 Dock 2 与一台 M3TD。
- 结果：两台设备均获得新的 `dji.flighthub.state` telemetry 与 pose，`device-state` 水位成功推进且重试计数归零。
- 地图检查：M3TD 映射为 `device-drone`，Dock 2 映射为 `device-dock`；两者均为 `online/fresh`。
- 坐标边界：位置继续标记为 `unverified`，在 7.7 完成前不用于控制预检。

## 10.4 目录计数与关联

| 目录 | 司空计数 | 本地活动投影 | Canonical 关联 | 结果 |
| --- | ---: | ---: | ---: | --- |
| 航线 | 13 | 13 | 13 | 通过 |
| 模型 | 2 | 2 | 2 | 通过 |
| 可访问飞行任务 | 0 | 0 | 0 | 通过（合法空状态） |
| 开放模型、直播分享、码流转换器、项目飞行区 | 0 | 0 | 不适用 | 通过（合法空状态） |

抽样仅验证关联存在性，没有输出远端标识。

## 10.5 端到端故障矩阵

| 场景 | 可重复证据 | 结果 |
| --- | --- | --- |
| 跨租户 | `TestSQLResourceRepositoryTenantIsolationAndIdempotency`、Web diagnostics 跨项目测试 | 通过 |
| 断开/禁用连接器 | `TestDisabledConnectorRejectsOutboxLeaseSyncAndResourceWritesButKeepsHistory`、Web lifecycle 测试 | 通过 |
| 部分页失败 | `TestCatalogStreamsDeduplicateRepeatedPagesAndProtectPartialSnapshots` | 通过 |
| 429/5xx | `TestClientRetriesRateLimitAndServerFailure`、Web client retry 测试 | 通过 |
| 业务空码 | `TestEndpointBusinessCodeProfiles`、`TestLiveCatalogEmptySnapshotsRemainHealthyAndComplete` | 通过 |
| 临时 URL | `TestTemporaryLinkPurposeHostAndExpiryValidation`、`TestSQLFlightAssetsAreIdempotentProjectScopedAndRefreshExpiredURLs` | 通过 |
| 命令未知结果 | `TestFlightTaskCreateAcceptedButFinallyUnknownBlocksWithoutRepeat` | 通过 |
| Worker 重启 | `TestCommandStatusPollingResumesAfterWorkerRestartWithoutEarlySuccess`、`TestSQLFlightActionJobRestartsReconcilesAndKeepsIntentEncrypted` | 通过 |

PostgreSQL 集成用例使用当前迁移后的本地测试数据库实际执行，没有以 skip 结果代替通过证据。

## 10.6 低风险现场写入（部分，未通过）

- 日期：2026-09-03
- 授权边界：仅执行不会上传对象、创建资源、启动直播/建模/任务或控制设备的临时凭据签发；明确禁止实际起飞及其他物理控制。
- 目标：当前账号、项目 `1123`，接口 `454273351e0`（项目存储 STS）与 `458069518e0`（开放建模上传凭据）。执行前已核对连接器、账号指纹、13 条航线、2 个模型、2 台纳管设备和既有 live-read evidence；未输出远端标识。
- 结果：`454273351e0=upstream_unavailable`，`458069518e0=upstream_error`；两项都没有重试、没有使用返回凭据，也没有后续上传/callback/重建/任务动作。
- 门禁结论：runner 因验收不完整拒绝持久化，项目 `field-write` 仍为 0；`security.temporary-credential`、`flight.execute`、`model.write` 及其他所有 action 均保持未验证和默认关闭。
- 官方资料复核：司空公有云 V2.0 用户手册确认飞行任务、模型重建与开放建模属于公开能力；逐接口参数仍以官方 Apifox 为准，官方示例仓库当前未提供 STS/open-model 示例。因此本次不根据猜测修改请求或盲目重试。

## 10.9 最终本地验证

- 日期：2026-09-02
- Go 全量：在 `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` 下运行 `go test ./...`，Worker 全部 package 通过。
- 数据库迁移：`pnpm test:migrations` 通过 PostGIS、空库、现有库和重复执行验证。
- 升级/回滚兼容：`pnpm drill:upgrade-rollback` 通过；72/72 migrations 完成，legacy baseline 正确 adopted，升级前、升级后和应用回滚后三阶段页面契约均可读；新证据保持 available/legal hold，未知 outbox 事件保持 pending、attempts=0、consumptions=0。
- 项目检查：`pnpm check` 通过；TypeScript 无错误，Web 388 项测试中 383 通过、5 项按设计在未提供显式 PostgreSQL 测试 URL 时跳过，Worker 全量测试再次通过。
- 生产构建：`pnpm build` 通过；Next.js 编译、类型检查、页面数据和静态页面生成完成，Go Worker 二进制构建完成。
- 补充发布门禁：司空文档/manifest 检查、OpenSpec strict 和 `git diff --check` 在记录本节后重新执行。
- 代码变更后复验：2026-09-03 新增低风险验收 runner 并记录现场失败结果后，使用 `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` 重跑 `go test ./...`、`pnpm check` 和 `pnpm build`，结果全部通过；Web 388 项中 383 通过、5 项按设计在未配置显式 PostgreSQL 测试 URL 时跳过。
- Dock 视频通道修复后复验：2026-09-03 将 Dock 2 官方型号通道投影接入独立 `device-state` 流；数据库集成验证覆盖幂等、状态降级、跨项目隔离及非法 camera index 写前拒绝。随后重跑 `go test ./...`、`pnpm test:migrations`、`pnpm check` 和 `pnpm build`，全部通过；Web 388 项中 383 通过、5 项按设计跳过。

上述验证不替代 7.7 坐标基准或 10.6/10.7 现场写入验收；现场结果若导致代码变化，发布前必须重新运行本节全部命令并更新记录。
