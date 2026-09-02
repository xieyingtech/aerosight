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
