# 本地验收记录

日期：2026-10-10。使用本地隔离 PostgreSQL、Qdrant、E5 和 8082 端口 Go 服务，未部署生产。

- 导航与列表标题显示“素材”，算法导航及页面标题显示“算法”。
- 列表顶部一个全宽搜索框，下方表格，无内嵌预览。输入名称 `source-5290.mp4` 后精准匹配先展示，语义补充在后。同一视频多个片段集中一行。
- 输入“林间”查询真实内容，点击素材 3 的 10–18 秒命中区间打开独立详情。DOM 读取播放器 currentTime=10、duration=18.018。
- 返回列表保留关键词；清空搜索恢复全部 3 个素材。排序单测覆盖来源等值优先、名称包含、语义顺序、去重和类型过滤。
- 旧 `assets/?projectId=1&assetId=3&startMs=10000&endMs=18018` 链接自动进入 canonical 详情，仍定位第 10 秒。
- 不存在素材详情显示不可用提示，不回退其他素材。内容请求失败显示独立错误，不影响文字匹配；请求取消与代次检查防止旧结果覆盖。

## 检查

- `pnpm build` 通过（类型检查、静态导出、sqlc 一致性、Go 构建）。修正搜索参数后再次 `pnpm build:web` 和 `pnpm build:server` 通过，并重启本地服务复核。
- `pnpm --dir apps/web test:unit`：395/395 通过。
- `pnpm check:web-boundary` 通过。
- 隔离 PostgreSQL 下 `go test -tags dev ./internal/httpapi -run 'TestProjectPageURL|TestLegacyPageRedirects|TestMediaSearch' -count=1 -v` 通过，证据链接和权限撤销测试实际执行，无数据库跳过。
- OpenSpec change 与全部 7 项主规范严格验证通过。`git diff --check` 通过。
- 本轮未重复执行完整 `pnpm check`；上一轮记录的既有 `TestReportAggregateIncludesAllEvidence` 失败仍保留，不宣称完整检查通过。本 change 保持活动，不归档。

验收脚本、服务配置和截图保存在被忽略的 `.build`，不纳入 Git。
