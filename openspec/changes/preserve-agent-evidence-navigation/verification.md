# 验证记录

2026-10-12：前端 typecheck、pnpm build:web 通过。智能体飞行与实时链接的两个现有测试通过。

本地浏览器使用线上只读会话 11，经本地前端点击设备链接再返回完整会话。文档加载次数 1，window 标记保持，会话回答保留，非 GET 请求数 0；外链保持新窗口、媒体 API 保持普通链接。记录位于忽略目录 .build/agent-navigation/local.json。

主规范已同步。本 change 严格验证通过。全量主规范验证 15 项通过，独立工作中的 overview-map-terrain 失败，不属于本改动。未执行全仓库 pnpm check/pnpm build，因此保留活动 change，不归档。

线上核验待部署后补充。

生产首次验证发现 canonical 页面目录的 index.txt / __next.*.txt 返回 404，修复范围扩展为 Go 导出文件别名。TestProjectExportNavigationData 与 TestStaticPagesGinFallbackBoundary 通过。实际静态导出经 Go pageNotFound + webassets 在本地运行，浏览器文档加载 1 次、会话及原回答保持、业务写请求 0；记录 .build/agent-navigation/export.json。严格主规范失败具体为独立 overview-map-terrain 的 Purpose 少于 50 字符；非严格校验该规范通过。
