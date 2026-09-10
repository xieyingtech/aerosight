# 实施前基线

- 前端单元测试：2026-09-06 执行 `pnpm test:web`，285/285 通过。
- Go 基线：迁移目录前执行 `go test ./...`，全部包通过；依赖 AEROSIGHT_TEST_DATABASE_URL 的集成用例未配置时由原测试跳过，不计作真实数据库验收。
- AST 清单：43 个 route.ts 共 50 个 HTTP 方法，4 个 Server Actions，27 个页面及 4 个布局；原 handler 正文、服务依赖与目标 API 位于 entrypoints.json。
- 数据库迁移测试：首次执行期间模块目录迁移使旧脚本的 apps/worker 引用失效；修复脚本路径后重新执行，结果另行记录，不将首次执行计作通过。

## 回归证据边界

2026-09-08 从旧发布 commit `bc314b3731ebc27b301ce0e15d0ffd50ab090a40` 的独立目录重新执行 `pnpm --dir .build/legacy-release-bc314b3/apps/web test:unit`，285/285 pass、0 skip，日志 `.build/legacy-baseline-final.log`。这次执行的是旧实现，未用当前 Web 测试数代替旧基线。

| 冻结项 | 内容与可执行验证 |
| --- | --- |
| HTTP 成功/失败 | `route-responses.json` 保存原 AST 清单中 48 个保留方法、每个方法成功/拒绝两种场景的 status、headers、原始 body、调用依赖和来源 hash。执行原 handler 的 CommonJS 转译，服务使用固定 fixture；所有非废弃入口成功场景必须为 2xx，两个只读历史写入口固定 410。Auth.js 的两个方法按设计被新认证接口替换。 |
| null、空数组与 ID | 响应 fixture 同时传入 `id: "17"`、`projectId: 3`、UUID runId、`description: null` 与 `items: []`；列表返回空数组。这是包装层序列化契约，不把通用 fixture 冒充每个领域完整 DTO；实际 DTO 另由各 Go HTTP/PostGIS 测试验证。 |
| 角色/授权 | `permission-matrix.json` 保存 owner/admin/member 各 15 组授权输入和旧策略输出，包含无授权、逐项授权、重复/未知权限及全部权限。原 TypeScript 和 Go `TestPermissionsMatchFrozenLegacyMatrix` 都读取/核对这些固定结果。平台管理员不跨越团队边界，仍由目录及各领域集成测试验证。 |
| 审计 | `legacy-web/audit-boundary.ts` 与原测试冻结审计先于副作用、失败回滚、稳定 hash；既有 `audit-hashes.json` 继续由 Go `TestLegacyAuditHashFixtures` 验证，包括旧 locale。 |
| 幂等 | 冻结 `legacy-web/device-commands.ts` 及其安全策略。`device-command-idempotency.test.ts` 实际运行旧 service：相同 device/key 且请求内容变化仍返回原 command/reused=true，显式 deny 在旧结果复用前生效，只插入/发布一次。query/audit runner 是 fixture；真实 SQL 并发、事务、审计与 outbox 数量由 `TestDeviceCommandSafetyIdempotencyAndAudit`、`TestConcurrentDeviceCommandOnlyQueuesOnce`、`TestDeviceCommandScopeReplayAndFailureRollback` 验证。 |
| 凭据 envelope | 冻结原 `credential-encryption.ts` 及原 3 个测试：固定 nonce 的 AES-256-GCM Go 共享向量、随机 nonce、错误 scope/secret 拒绝。Go `TestCredentialEncryptionMatchesWebVector` 使用同一固定密文，领域测试额外验证空凭据保留、脱敏与租户 AAD。 |

`pnpm test:legacy-contracts` 重放旧 handler 和旧权限输出，检查冻结原文件 SHA-256（换行归一化为 LF）；已接入 `pnpm test:web`，原策略测试也被该命令运行。不要通过 `--write`/`--capture` 接受迁移差异；它们只用于显式重新采集基线。两种证据层次明确分开：这里固定旧契约，任务 8.1 对照当前 Go 实际行为。

这些结果仅证明原实现基线。Go 新 API 的响应、权限、幂等、审计、凭据和数据库集成必须在迁移各模块时用独立测试验证，不能用原 TS 测试通过替代。
