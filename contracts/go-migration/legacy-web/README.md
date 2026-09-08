# 历史数据库契约基线

这里保存迁移前快照与回放 SQL 实现及其原有测试，供兼容回归和数据库基准脚本使用。它们不属于 Web 应用，不得由页面、组件或运行时模块导入。

浏览器 DTO 保留在 `apps/web/lib/project-snapshot-core.ts` 和 `project-replay-core.ts`；实际查询由 Go/sqlc 执行。`pnpm test:web` 和 `pnpm test:security` 继续运行这些历史测试。数据库维护脚本位于 `scripts/legacy-db`，仅使用根包的开发依赖 `pg`；正式 `db:migrate` 仍运行 Go 迁移器。

另保存旧 commit bc314b3 的权限、凭据、审计和设备命令源代码及原策略测试；来源 hash 见上层 permission-matrix.json。设备命令 service 不可直接作为应用模块导入，专用兼容测试通过隔离 VM 和 query fixture 运行它。完整基线范围、重放命令与真实数据库验证边界见上层 baseline.md。
