# MVP 试点发布门禁记录

## 决策

2026-08-27（Asia/Shanghai）对候选基线 `7ca6c0a` 及仅包含发布脚本/文档的待提交变更执行完整门禁，全部通过，无遗留代码、测试、迁移、构建或 OpenSpec 阻断项。工程门禁允许进入受控试点；实际开启设备命令前仍须完成部署手册中的现场法规、人员、空域、急停和双人审批检查。

## 门禁结果

| 门禁 | 结果 | 证据摘要 |
| --- | --- | --- |
| `pnpm check` | 通过 | TypeScript 无错误；169/169 Web 测试通过；全部 Go 包通过 |
| `pnpm test:all` | 通过 | 重跑 169 项 Web 与全部 Go 包；PostGIS 空库/现有/旧版/重复迁移通过；52/52 安全验收通过 |
| `pnpm benchmark:load` | 通过 | 遥测→地图/时间线 P95 37.12 ms ≤ 2,000 ms；detection→告警 P95 9.39 ms ≤ 5,000 ms；重复副作用为 0 |
| `pnpm drill:fresh-environment` | 通过 | 19 项配置/依赖契约、31 个迁移、Web/Worker 生产构建通过 |
| `pnpm drill:upgrade-rollback` | 通过 | 旧页面三阶段可读；新证据 retained；未知事件 pending/0 attempts/0 consumption |
| `pnpm build` | 通过 | Next.js 生产构建、TypeScript、静态页面生成和 Go Worker 二进制构建通过 |
| `pnpm exec openspec validate build-air-ground-inspection-mvp --strict` | 通过 | `Change 'build-air-ground-inspection-mvp' is valid` |

## 最终性能与完整性数据

- 12 台设备 × 30 tick = 360 条唯一遥测，另有 360 次重复投递；telemetry/observation/pose 均严格为 360。
- 30 条 canonical detection，另有 30 次重复 callback；callback/detection/group/alert 均严格为 30。
- 遥测链路 P50 27.40 ms、P95 37.12 ms、最大 118.57 ms。
- detection 链路 P50 8.00 ms、P95 9.39 ms、最大 12.28 ms。
- 升级回滚演练中的新版证据保持 `available`、`legal_hold=true`、1 个 published link、1 个 active hold。
- 回滚 Worker 不领取未知类型，未来事件保持 `pending`、attempts=0、无锁、无 consumption。

## 试点开启约束

发布时按部署手册记录不可变构建版本、迁移 ledger、数据库/对象目录备份和功能开关审批。先启用只读总览，再依次验证对象存储和算法；设备命令最后开启，AI Provider 与包含 `copilot.run` 的 Task 独立审批。任何租户隔离、审计、重复命令、证据读取、readiness 或现场安全异常都应立即关闭设备命令和外部算法、暂停相关 Tasks，必要时停用默认 AI Provider，并按回滚手册保留数据库与证据。
