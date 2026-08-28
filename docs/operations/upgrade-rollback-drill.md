# 数据升级与应用回滚演练

## 结论

2026-08-27 执行 `pnpm drill:upgrade-rollback`，演练通过。数据库 schema 只向前迁移，应用可以回滚到仍遵守兼容性契约的旧构建；回滚过程不降级 schema、不清理新数据。

## 演练步骤与结果

1. 在全新 `postgis/postgis:17-3.5` 中加载 `0001_baseline.sql`，写入旧项目、成员、设备、任务、任务运行和资产快照。
2. 用旧页面 SQL 契约读取项目、设备、任务、运行和资产，确认升级前可用。
3. 运行当前迁移器：baseline 被安全采用，31 个迁移全部登记并完成。
4. 再次执行旧页面 SQL，返回内容与升级前一致。
5. 用新版 schema 写入一项新证据，包含 available asset、published evidence link、`legal_hold=true` 和 active retention hold。
6. 写入旧 Worker 未注册的 `future.evidence.sealed` outbox event，再按回滚 Worker 的已注册事件集合执行 claim。
7. 执行应用回滚后的旧页面查询：旧项目、1 台设备、1 个任务、1 个运行仍可见；旧资产和新资产共 2 项均可由旧查询读取。
8. 核对新版证据仍为 available，published link=1、active hold=1；未知事件保持 pending、attempts=0、locked_by=null、consumptions=0。

Worker 的 outbox claim 现在把已注册事件类型传给数据库，只领取当前二进制能处理的类型。这样回滚构建不会将未来事件重试到 dead-letter，也不会提前写 consumption；新版 Worker 恢复后仍可领取该事件。

## 复跑

```sh
pnpm drill:upgrade-rollback
```

脚本会创建并销毁隔离的临时 PostGIS，输出迁移、旧页面兼容、新证据保全和未知事件状态的 JSON。可用 `ROLLBACK_DRILL_POSTGIS_IMAGE` 覆盖数据库镜像。

## 回滚边界

- 仅回滚 Web/Worker 构建，不回滚数据库迁移。
- 不删除未知表、列、事件、对象或证据。
- 回滚目标必须包含“只 claim 已注册 outbox 类型”的兼容性契约；更老且不满足该契约的构建不得作为回滚目标。
- 发生回滚时先关闭设备命令和外部算法、暂停相关 Tasks，必要时停用默认 AI Provider；核对未 ACK 命令、Copilot Job 与 outbox，再恢复写流量。
