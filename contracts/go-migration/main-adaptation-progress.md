# main 适配验收

基线：main / origin/main `3b19aebc5156ea440c107f900e3d92b994ad5c16`。
原迁移历史保存在本地分支 `codex/unify-nextjs-ssg-go-server-before-main`。
本轮在 main 上整合为一个普通提交，不创建 merge commit、不推送远端。

## 运行结构

Next 只产出静态页面；Go Gin/sqlc 承担 API、认证、事务、后台任务及静态资源托管。
main 的 FlightHub、任务触发、报告与维护命令已整合到同一 Go module/runtime。
旧 TS 服务端实现与路由只保留在 contracts/go-migration/legacy-web 供契约比较，不参与生产构建。
main 中没有调用入口的 executeAgentMissionStart 保留为参考；实际任务触发 API 与后台 tasktrigger 已迁移。

## main 增量映射

| main 功能 | Go / 静态端实现 | 回归证据 |
| --- | --- | --- |
| FlightHub 飞行、空间、模型工作台 | flighthub_workspaces / workspace_reads / workspace_present；固定静态页面 | FlightHubMainWorkspaceQueriesExecute、FlightHubMainGovernedWritesPostgres |
| 诊断、能力探测、重连、管理读取、加入码 | flighthub_lifecycle_main、flighthub_join_code、管理工作台 | 真实数据库治理写入及生命周期测试；上游只使用客户端接口 |
| live / geospatial / flight / device-admin / model / management actions | typed sqlc 查询、审批/现场证据门槛、审计、加密请求、幂等及 outbox | 模型删除预览、版本变化、审批撤销、成员预览及幂等数据库测试；输入/策略契约测试 |
| FlightHub 控制会话 | flighthub_control_sessions | acquire/replay、heartbeat 限速、持有人拒绝、release 数据库测试 |
| FlightHub 直播、播放授权 | live_start / live_stop / live_playback | 真实数据库启动、停止、撤权；原有播放与并发测试 |
| 设备发现、重新绑定、离散命令 | device_discoveries_main / flighthub_device_commands | 新建/重放/迁移路由，设备命令并发、权限复查、失败回滚 |
| 任务工作台、版本、手动/API/Webhook 触发 | task_drafts / task_definition / task_workbench / task_triggers | 草稿复用、保存、发布、不可变版本、触发幂等及并发限制 |
| 案件反馈、报告 | issue_feedback / report_drafts | 反馈幂等、版本冲突、事务回滚；案件/评论/反馈进入报告 |
| 遥测坐标、能力门槛、快照 | project_reads / snapshot | 原始未校准坐标、相机可用性、任务步骤及算法运行回归 |
| 静态页面、写请求与旧链接 | 固定路径 + 查询参数，apiFetch CSRF，Go/Next redirects | web boundary、typecheck、静态导出、生产浏览器、真实开发代理 |

所有 73 个 main 迁移原文保留。db/schema.sql 从完整迁移后的 PostgreSQL 17/PostGIS 数据库导出，并由数据库 catalog 比较约束、索引、函数、序列及列定义；sqlc 重新生成。

main-route-inventory.json 保存本轮 main 变更的 23 个路由文件、33 个 HTTP 方法。TestMainChangedRouteInventory 在真实 Server 路由表上验证全部方法均已注册。

## 验证

- pnpm check：通过，含前端边界、类型检查、Web/历史契约及 Go 全包测试。
- pnpm build：通过，73 个迁移与静态前端嵌入 Go 二进制。
- pnpm db:check：通过。
- 设置 AEROSIGHT_MIGRATION_TEST_DATABASE_URL 的 Go 全包数据库回归：通过，HTTP API 包用时 120 秒；最终局部修改另行回归通过。
- pnpm test:migrations：通过，空库、旧库、重复执行。
- pnpm drill:upgrade-rollback：通过，数据库/Go 切换与回滚演练；未提供旧 Node release，因此未执行旧 Node 应用进程演练。
- pnpm test:security：通过。
- pnpm test:production-browser：通过，实际 Chrome + HTTPS Go 服务、hydration、会话、工作台、地图、媒体 iframe 与 CSP。
- pnpm test:dev-proxy：通过，Cookie/CSRF、写入、Range/HEAD、SSE 首条与取消。
- pnpm test:development-browser：通过；迁移后遗留的 Next dev 构建缓存已清理。

生产 CSP 默认允许 OpenFreeMap 与原地图来源；CSP_MEDIA_ORIGINS 配置精确 HTTPS 媒体/RTC 域名，同时派生同主机 WSS 用于信令，不使用通配来源或 unsafe-eval。
浏览器脚本支持 PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH，以复用已安装 Chrome。

未连接真实 DJI/FlightHub 账号、设备或 Volc RTC 供应商；远端命令执行与真实供应商播放仍须按既有现场验收流程验证。测试不会绕过 feature flag、审批或账号绑定的 field-write 证据。
