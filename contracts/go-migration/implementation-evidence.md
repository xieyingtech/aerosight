# 实施证据

当前仍处于迁移阶段；默认前端启动入口尚未切换为静态导出。未勾选任务仍需实现和验证。

## 2026-09-06

- 原 `pnpm test:web`：285 项通过。目录迁移后的 `pnpm test:migrations` 重跑通过，覆盖 PostGIS、空库、已有库及重复执行。
- Go 迁移器集成测试：并发迁移锁、原始字节 checksum、历史篡改阻止待执行迁移、已有基线收养、失败 DDL 回滚。
- Go 初始化测试：重复启动保留管理员密码；sqlc 与原始 SQL 共享事务并整体回滚。
- `pnpm db:check`：通过；人为修改生成结果后检查正确失败且没有覆盖改动，恢复后再次通过。
- HTTP 数据库测试：登录 CSRF/Origin 校验、持久会话重启恢复、退出后失效、团队/项目 JSON null 与空数组、跨团队读写拒绝。
- 原 bcryptjs 生成的密码 hash 登录成功；二次登录轮换 token 并使旧 token 失效；Auth.js Cookie 拒绝；数据库会话过期后返回 401。
- 项目 SSE 集成测试：502 条事件触发 snapshot.required；Last-Event-ID 优先于 query cursor；按数值游标续传；等待期间连接池 InUse 为零；退出后 15 秒复查断流。
- 频道 SSE 集成测试：502 条样本触发 backpressure_limit_exceeded；从 501 续传第 502 条；等待期间无连接占用；新增显式 deny 后 5 秒复查断流；再次访问返回 404。
- 项目快照测试：空图层、诊断 nullable deviceId、隐藏 role、撤权 404、显式 deny、离线设备操作禁用、依赖降级；真实 PostGIS PointZ 坐标、双点 LineString 轨迹和在线新鲜度通过。
- 设备/回放测试：设备列表和树读取、循环关系终止、回放类型与 bbox 筛选、非法时间窗与 bbox、2002 条事件截断为 2000 条并标记 truncated，以及四个读取入口撤权后返回 404。
- Prometheus：原业务指标目录、标签拒绝和 gauge 语义测试通过；HTTP 测试验证匿名拒绝、Bearer 访问、路由模板标签及 Go runtime 指标。
- 当前 `pnpm check` 通过：TypeScript 类型检查、285 项 Web 测试、Go 全套测试（新增数据库测试使用真实独立 PostGIS 服务）。`pnpm db:check`、`go mod verify`、OpenSpec strict 校验通过。未把这些阶段性检查作为未迁移业务或生产静态构建的验收。

新集成测试使用独立 PostGIS 测试容器，通过 `AEROSIGHT_MIGRATION_TEST_DATABASE_URL` 指定测试服务，并为每项测试创建和清理随机临时数据库；未设置该变量时会跳过，不代表数据库行为已经验收。
