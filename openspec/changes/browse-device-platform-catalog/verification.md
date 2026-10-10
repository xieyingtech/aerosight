# 验证记录

- `TestDevicePlatformCatalog` 在隔离本地 PostgreSQL 通过：类型关联字段、历史对象能力、跨驱动同名能力、schema 字段、通道单位及能力关联、真实操作目录、非管理员拒绝访问。
- `TestDeviceTypeIconValidation` / `TestDeviceTypeIconAPI` 通过：保留兼容 API、图标存储与项目快照投影。
- `pnpm typecheck`、`pnpm build` 和 sqlc 生成一致性检查通过，前端静态导出及 Go 服务打包成功。
- 主规范已同步，change 和主规范严格验证通过。
- 浏览器首次验收曾因原 tab 停在 `data:` 错误页被安全策略拦截，未绕过策略。2026-10-10 tab 已恢复正常 HTTP 页面后，实际验收五个 tab、state.read 跨驱动搜索与定义详情、通道能力关联、cover.open 操作键搜索均通过；页面无图标编辑入口。截图位于 `.build/device-catalog-metadata.png`。
- 本次无需数据迁移，未操作生产数据库；测试辅助脚本和截图位于忽略的 `.build`。
- 本次未归档。前次全量检查的既有 `TestReportAggregateIncludesAllEvidence` 报告链接断言失败仍保留，本次未修改报告实现或该测试。
