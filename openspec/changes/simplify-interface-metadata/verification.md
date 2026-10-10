# 验证记录

- `pnpm typecheck` 通过；`pnpm --dir apps/web test:unit` 395 项通过。
- 最终 `pnpm build` 通过，包括最新的独立内置/只读标签、前端静态导出、sqlc 一致性及 Go 打包。`pnpm check:web-boundary` 通过；本地测试服务已更新。
- `go test ./internal/flighthub ./internal/dji -count=1` 通过，包括远端写请求响应丢失后的对账测试。本次未额外提供数据库测试连接，不把需要连接而跳过的数据库测试视为已执行。
- `pnpm db:generate` 已同步快照标题查询生成代码；生产构建的 sqlc 一致性检查通过。
- 当前浏览器断言按分组后的字段/链接同步；`node --check` 通过，未运行完整巡检浏览器回归脚本。
- 本地实测设备目录的五个 tab、搜索、关联及详情，MCP 连接/工具数量分组展示；重点页面截图位于 `.build/device-catalog-metadata.png` 和 `.build/platform-metadata.png`。
- 扫描剩余 15 行、14 个文件的中点：前端仅 Enter 快捷键；后端 5 处匹配/对账命名及 1 个兼容测试；历史契约 4 行；历史验证记录 1 行；当前规范 3 行保留 Enter 原文。盘点位于忽略的 `.build/middle-dot-audit.md`，辅助脚本不加入 Git。
- 后端保留的规则名称按 `event_rules.name` 匹配；外部航线、模型与飞行任务按持久化对账名称重试。没有批量重命名历史数据库记录。
- 当前元信息与版本主规范已同步，严格验证通过。本次未归档，未运行全量 `pnpm check`；先前报告聚合链接断言的既有失败未修改。
