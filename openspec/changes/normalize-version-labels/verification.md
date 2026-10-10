# 验证记录

- `pnpm typecheck` 通过。
- `pnpm build` 通过，包括前端静态导出、sqlc 一致性和 Go 打包；本地测试服务已重启使用新构建。
- `go test ./internal/httpapi -run '^TestObjectQueryFilterAndInput$' -count=1` 通过，覆盖内置 Skill 加载与输入约束。
- `node --check scripts/test-inspection-authoring-browser.mjs` 通过，研判链接文案断言已同步；未执行浏览器测试。
- `git diff --check` 通过。
- change 与主规范严格验证通过，主规范已同步；AGENTS.md 已记录名称版本展示约定。
- 全局文本搜索的剩余结果为 79 个文件、162 个匹配行、227 个中点。完整文件位置、片段及建议位于忽略的 `.build/middle-dot-audit.md`，不将盘点辅助脚本加入 Git。
- 本次没有数据迁移、外部资源重命名或非版本中点批量替换；未归档，未运行全量测试。
