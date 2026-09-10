# 迁移入口验收矩阵

基于冻结入口逐项映射到实际 HTTP/PostGIS 测试。测试文件名对应 apps/server/internal/httpapi；每个文件的全部具名测试须在真实数据库回归中通过。该矩阵是覆盖索引，业务断言与运行证据仍以测试正文和回归日志为准。

页面行为另由 scripts/browser-page-states.mjs、browser-project-workspaces.mjs、browser-project-details.mjs 与 test-production-browser.mjs 验证加载/失败/拒绝、旧链接及构建后新增资源。Auth.js 两个方法按设计替换为显式 auth API；历史事件写方法保留 410。

| 原入口 | 目标 | 验证范围 | 测试文件 |
| --- | --- | --- | --- |
| apps/web/app/(app)/admin/ai-providers/page.tsx | GET /api/admin/ai-providers | 管理、凭据、默认项、上游与撤权 | ai_providers_test.go、ai_upstream_test.go |
| apps/web/app/(app)/admin/layout.tsx | GET /api/auth/session | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/(app)/admin/page.tsx | GET /api/admin/overview | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/admin/projects/page.tsx | GET /api/admin/projects | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/admin/teams/page.tsx | GET /api/admin/teams | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/admin/users/page.tsx | GET /api/admin/users | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/layout.tsx | GET /api/auth/session | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/(app)/layout.tsx | GET /api/projects | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/profile/page.tsx | GET /api/profile | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/projects/[id]/agents/page.tsx | GET /api/projects/:id/agent-sessions | 会话与消息读取、权限和输入 | agent_sessions_test.go |
| apps/web/app/(app)/projects/[id]/algorithms/page.tsx | GET /api/projects/:id (permissions; enforcement remains in each protected API) | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/projects/[id]/algorithms/page.tsx | GET /api/projects/:id/algorithm-runs | 创建、详情、重试、事务与作用域 | algorithm_runs_test.go |
| apps/web/app/(app)/projects/[id]/algorithms/page.tsx | GET /api/projects/:id/algorithm-providers | 配置、凭据、脱敏、探测与权限 | algorithm_providers_test.go |
| apps/web/app/(app)/projects/[id]/algorithms/page.tsx | GET /api/projects/:id/algorithm-definitions | 定义与不可变快照 | algorithm_definitions_test.go |
| apps/web/app/(app)/projects/[id]/algorithms/runs/[runId]/page.tsx | GET /api/projects/:id/algorithm-runs/:runId | 创建、详情、重试、事务与作用域 | algorithm_runs_test.go |
| apps/web/app/(app)/projects/[id]/assets/page.tsx | GET /api/projects/:id/:kind | 原 listProjectItems 的资产与任务列表 | assets_list_test.go、mission_reads_test.go |
| apps/web/app/(app)/projects/[id]/connectors/page.tsx | GET /api/projects/:id | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/projects/[id]/connectors/page.tsx | GET /api/projects/:id/device-adapters | 适配器读写、凭据与原子创建 | device_adapters_test.go |
| apps/web/app/(app)/projects/[id]/connectors/page.tsx | GET /api/projects/:id/connectors/dji-flighthub | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/(app)/projects/[id]/connectors/page.tsx | GET /api/projects/:id/connectors/dji-flighthub/activity | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/(app)/projects/[id]/connectors/page.tsx | GET /api/projects/:id/features | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/(app)/projects/[id]/devices/page.tsx | GET /api/projects/:id/device-tree | PostGIS、设备树、回放筛选与租户 | snapshot_test.go、project_reads_test.go |
| apps/web/app/(app)/projects/[id]/events/[eventId]/page.tsx | GET /api/projects/:id/events/:eventId | 旧事件详情及废弃写入口 410 | legacy_events_test.go |
| apps/web/app/(app)/projects/[id]/events/page.tsx | client-only helper | 静态导航、链接与页面边界（另有生产浏览器） | page_redirects_test.go、static_pages_test.go |
| apps/web/app/(app)/projects/[id]/issues/[issueId]/page.tsx | GET /api/projects/:id/issues/:issueId | 案件详情、关联证据与权限 | issue_reads_test.go |
| apps/web/app/(app)/projects/[id]/issues/[issueId]/page.tsx | client-only helper | 静态导航、链接与页面边界（另有生产浏览器） | page_redirects_test.go、static_pages_test.go |
| apps/web/app/(app)/projects/[id]/issues/page.tsx | GET /api/projects/:id/issues | 案件详情、关联证据与权限 | issue_reads_test.go |
| apps/web/app/(app)/projects/[id]/layout.tsx | static layout | 静态导航、链接与页面边界（另有生产浏览器） | page_redirects_test.go、static_pages_test.go |
| apps/web/app/(app)/projects/[id]/page.tsx | GET /api/projects/:id | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/projects/[id]/page.tsx | GET /api/auth/session | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/(app)/projects/[id]/page.tsx | GET /api/projects/:id/snapshot | PostGIS、设备树、回放筛选与租户 | snapshot_test.go、project_reads_test.go |
| apps/web/app/(app)/projects/[id]/realtime/page.tsx | GET /api/projects/:id | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/projects/[id]/realtime/page.tsx | GET /api/auth/session | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/(app)/projects/[id]/realtime/page.tsx | GET /api/projects/:id/snapshot | PostGIS、设备树、回放筛选与租户 | snapshot_test.go、project_reads_test.go |
| apps/web/app/(app)/projects/[id]/settings/page.tsx | GET /api/projects/:id | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/projects/[id]/tasks/page.tsx | GET /api/projects/:id/:kind | 原 listProjectItems 的资产与任务列表 | assets_list_test.go、mission_reads_test.go |
| apps/web/app/(app)/projects/[id]/tasks/page.tsx | GET /api/projects/:id/task-runs | 任务定义和工作台读取 | mission_reads_test.go |
| apps/web/app/(app)/projects/[id]/tasks/runs/[runId]/page.tsx | GET /api/projects/:id/task-runs/:runId | 任务定义和工作台读取 | mission_reads_test.go |
| apps/web/app/(app)/projects/new/page.tsx | GET /api/teams?scope=managed | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/projects/page.tsx | GET /api/projects | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/teams/[id]/page.tsx | GET /api/teams/:id | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/(app)/teams/page.tsx | GET /api/teams | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/actions.ts#loginAction | POST /api/auth/login | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/actions.ts#logoutAction | POST /api/auth/logout | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/actions.ts#createTeamAction | POST /api/teams | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/actions.ts#createProjectAction | POST /api/projects | 目录、表单、管理、空数组与租户 | directory_test.go、auth_test.go |
| apps/web/app/api/admin/ai-providers/[providerId]/route.ts | PATCH /api/admin/ai-providers/:providerId | 管理、凭据、默认项、上游与撤权 | ai_providers_test.go、ai_upstream_test.go |
| apps/web/app/api/admin/ai-providers/[providerId]/route.ts | DELETE /api/admin/ai-providers/:providerId | 管理、凭据、默认项、上游与撤权 | ai_providers_test.go、ai_upstream_test.go |
| apps/web/app/api/admin/ai-providers/[providerId]/test/route.ts | POST /api/admin/ai-providers/:providerId/test | 管理、凭据、默认项、上游与撤权 | ai_providers_test.go、ai_upstream_test.go |
| apps/web/app/api/admin/ai-providers/route.ts | GET /api/admin/ai-providers | 管理、凭据、默认项、上游与撤权 | ai_providers_test.go、ai_upstream_test.go |
| apps/web/app/api/admin/ai-providers/route.ts | POST /api/admin/ai-providers | 管理、凭据、默认项、上游与撤权 | ai_providers_test.go、ai_upstream_test.go |
| apps/web/app/api/auth/[...nextauth]/route.ts | GET /api/auth/* (replaced by explicit auth endpoints) | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/api/auth/[...nextauth]/route.ts | POST /api/auth/* (replaced by explicit auth endpoints) | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/api/media-auth/route.ts | POST /api/media-auth | 媒体机器鉴权 | media_auth_test.go |
| apps/web/app/api/projects/[id]/agent-sessions/[sessionId]/messages/route.ts | POST /api/projects/:id/agent-sessions/:sessionId/messages | 工具循环、历史、限制、故障与作用域 | agent_chat_test.go、agent_read_tools_test.go |
| apps/web/app/api/projects/[id]/agent-sessions/route.ts | POST /api/projects/:id/agent-sessions | 会话与消息读取、权限和输入 | agent_sessions_test.go |
| apps/web/app/api/projects/[id]/algorithm-definitions/[definitionId]/route.ts | PUT /api/projects/:id/algorithm-definitions/:definitionId | 定义与不可变快照 | algorithm_definitions_test.go |
| apps/web/app/api/projects/[id]/algorithm-definitions/route.ts | GET /api/projects/:id/algorithm-definitions | 定义与不可变快照 | algorithm_definitions_test.go |
| apps/web/app/api/projects/[id]/algorithm-definitions/route.ts | POST /api/projects/:id/algorithm-definitions | 定义与不可变快照 | algorithm_definitions_test.go |
| apps/web/app/api/projects/[id]/algorithm-providers/[providerId]/route.ts | PATCH /api/projects/:id/algorithm-providers/:providerId | 配置、凭据、脱敏、探测与权限 | algorithm_providers_test.go |
| apps/web/app/api/projects/[id]/algorithm-providers/[providerId]/test/route.ts | POST /api/projects/:id/algorithm-providers/:providerId/test | 配置、凭据、脱敏、探测与权限 | algorithm_providers_test.go |
| apps/web/app/api/projects/[id]/algorithm-providers/route.ts | GET /api/projects/:id/algorithm-providers | 配置、凭据、脱敏、探测与权限 | algorithm_providers_test.go |
| apps/web/app/api/projects/[id]/algorithm-providers/route.ts | POST /api/projects/:id/algorithm-providers | 配置、凭据、脱敏、探测与权限 | algorithm_providers_test.go |
| apps/web/app/api/projects/[id]/algorithm-runs/[runId]/retry/route.ts | POST /api/projects/:id/algorithm-runs/:runId/retry | 创建、详情、重试、事务与作用域 | algorithm_runs_test.go |
| apps/web/app/api/projects/[id]/algorithm-runs/route.ts | POST /api/projects/:id/algorithm-runs | 创建、详情、重试、事务与作用域 | algorithm_runs_test.go |
| apps/web/app/api/projects/[id]/assets/[assetId]/access/route.ts | GET /api/projects/:id/assets/:assetId/access | 签名、Range/HEAD、期限与租户 | media_access_test.go |
| apps/web/app/api/projects/[id]/assets/[assetId]/content/route.ts | GET /api/projects/:id/assets/:assetId/content | 签名、Range/HEAD、期限与租户 | media_access_test.go |
| apps/web/app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/route.ts | DELETE /api/projects/:id/connectors/dji-flighthub/:connectorId | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/sync/route.ts | POST /api/projects/:id/connectors/dji-flighthub/:connectorId/sync | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/token/route.ts | PUT /api/projects/:id/connectors/dji-flighthub/:connectorId/token | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/api/projects/[id]/connectors/dji-flighthub/projects/route.ts | POST /api/projects/:id/connectors/dji-flighthub/projects | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/api/projects/[id]/connectors/dji-flighthub/route.ts | GET /api/projects/:id/connectors/dji-flighthub | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/api/projects/[id]/connectors/dji-flighthub/route.ts | POST /api/projects/:id/connectors/dji-flighthub | 连接生命周期、错误、历史、加密与权限 | flighthub_test.go |
| apps/web/app/api/projects/[id]/device-adapters/[adapterId]/route.ts | PATCH /api/projects/:id/device-adapters/:adapterId | 适配器读写、凭据与原子创建 | device_adapters_test.go |
| apps/web/app/api/projects/[id]/device-adapters/[adapterId]/test/route.ts | POST /api/projects/:id/device-adapters/:adapterId/test | 网络探测、配置、回滚与撤权 | device_network_test.go |
| apps/web/app/api/projects/[id]/device-adapters/discoveries/[identityId]/bind/route.ts | POST /api/projects/:id/device-adapters/discoveries/:identityId/bind | 并发绑定、作用域与回滚 | device_binding_test.go |
| apps/web/app/api/projects/[id]/device-adapters/dji-setup/route.ts | POST /api/projects/:id/device-adapters/dji-setup | 网络探测、配置、回滚与撤权 | device_network_test.go |
| apps/web/app/api/projects/[id]/device-adapters/route.ts | GET /api/projects/:id/device-adapters | 适配器读写、凭据与原子创建 | device_adapters_test.go |
| apps/web/app/api/projects/[id]/device-adapters/route.ts | POST /api/projects/:id/device-adapters | 适配器读写、凭据与原子创建 | device_adapters_test.go |
| apps/web/app/api/projects/[id]/devices/[deviceId]/commands/route.ts | POST /api/projects/:id/devices/:deviceId/commands | 能力、安全、幂等、并发、审计与回滚 | device_commands_test.go |
| apps/web/app/api/projects/[id]/devices/[deviceId]/live-streams/route.ts | POST /api/projects/:id/devices/:deviceId/live-streams | 直播创建、并发与原子分发 | live_start_test.go |
| apps/web/app/api/projects/[id]/events/[eventId]/actions/route.ts | POST /api/projects/:id/events/:eventId/actions | 旧事件详情及废弃写入口 410 | legacy_events_test.go |
| apps/web/app/api/projects/[id]/events/[eventId]/agent-drafts/route.ts | POST /api/projects/:id/events/:eventId/agent-drafts | 旧事件详情及废弃写入口 410 | legacy_events_test.go |
| apps/web/app/api/projects/[id]/events/route.ts | GET /api/projects/:id/events | SSE 游标、溢出、撤权与断流 | streams_test.go |
| apps/web/app/api/projects/[id]/issues/[issueId]/actions/route.ts | POST /api/projects/:id/issues/:issueId/actions | 协作、状态版本、审计与并发 | issue_writes_test.go |
| apps/web/app/api/projects/[id]/live-streams/[streamId]/playback/route.ts | GET /api/projects/:id/live-streams/:streamId/playback | 播放授权与令牌 | live_playback_test.go |
| apps/web/app/api/projects/[id]/live-streams/[streamId]/stop/route.ts | POST /api/projects/:id/live-streams/:streamId/stop | 停止、并发与回滚 | live_stop_test.go |
| apps/web/app/api/projects/[id]/realtime-channels/[channelId]/events/route.ts | GET /api/projects/:id/realtime-channels/:channelId/events | SSE 游标、溢出、撤权与断流 | streams_test.go |
| apps/web/app/api/projects/[id]/replay/route.ts | GET /api/projects/:id/replay | PostGIS、设备树、回放筛选与租户 | snapshot_test.go、project_reads_test.go |
| apps/web/app/api/projects/[id]/reports/[reportId]/export/route.ts | GET /api/projects/:id/reports/:reportId/export | 报告发布、保留与导出 | reports_test.go |
| apps/web/app/api/projects/[id]/reports/[reportId]/publish/route.ts | POST /api/projects/:id/reports/:reportId/publish | 报告发布、保留与导出 | reports_test.go |
| apps/web/app/api/projects/[id]/snapshot/route.ts | GET /api/projects/:id/snapshot | PostGIS、设备树、回放筛选与租户 | snapshot_test.go、project_reads_test.go |
| apps/web/app/api/projects/[id]/task-runs/[runId]/audit-trace/route.ts | GET /api/projects/:id/task-runs/:runId/audit-trace | 审计关联与无设备副作用演练 | mission_audit_test.go |
| apps/web/app/api/projects/[id]/task-runs/[runId]/control/route.ts | POST /api/projects/:id/task-runs/:runId/control | 状态机、审批、并发与事务 | mission_control_test.go |
| apps/web/app/api/projects/[id]/task-runs/[runId]/emergency-stop-drill/route.ts | POST /api/projects/:id/task-runs/:runId/emergency-stop-drill | 审计关联与无设备副作用演练 | mission_audit_test.go |
| apps/web/app/api/projects/[id]/task-runs/[runId]/reports/route.ts | POST /api/projects/:id/task-runs/:runId/reports | 报告草稿、聚合与回滚 | report_drafts_test.go |
| apps/web/app/layout.tsx | client-only helper | 静态导航、链接与页面边界（另有生产浏览器） | page_redirects_test.go、static_pages_test.go |
| apps/web/app/login/page.tsx | GET /api/auth/session | 认证替换、会话与 CSRF | auth_test.go、csrf_boundary_test.go、session_response_test.go |
| apps/web/app/page.tsx | static layout | 静态导航、链接与页面边界（另有生产浏览器） | page_redirects_test.go、static_pages_test.go |

共 85 个入口记录、99 个方法/页面查询映射，关联 73 个 Go 顶层测试。

复核命令：node scripts/check-migration-coverage.mjs --results .build/final-go-postgis.jsonl。先执行真实 PostGIS 全包回归，再核对结果；不接受默认未配置数据库时的 skip 作为通过。
