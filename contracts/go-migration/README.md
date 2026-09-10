# Go 迁移接口基线

后续 main 的 33 个新增/修改 HTTP 方法见 [main-route-inventory.json](./main-route-inventory.json)，本轮映射与验收见 [main-adaptation-progress.md](./main-adaptation-progress.md)。下表是原迁移分支基线，不能代替后续 main 增量清单。

本清单在迁移前由 TypeScript AST 提取。entrypoints.json 保存原 Route Handler 与 Server Action，作为状态码、响应字段和错误映射的可审查基线；页面条目列出直接服务依赖。测试完成前不得把迁移清单当作实现完成证据。

新增 API 统一执行 design 中的会话、CSRF、作用域和事务要求。各领域权限/幂等/审计基线由原导入的 service 及现有测试定义；迁移时逐项补充 Go 契约测试。

| 类型 | 原入口 | 目标接口/页面查询 |
| --- | --- | --- |
| page | apps/web/app/(app)/admin/ai-providers/page.tsx | listAIProviders → GET /api/admin/ai-providers |
| page | apps/web/app/(app)/admin/layout.tsx | requireAdmin → GET /api/auth/session |
| page | apps/web/app/(app)/admin/page.tsx | getAdminOverview → GET /api/admin/overview |
| page | apps/web/app/(app)/admin/projects/page.tsx | listAdminProjects → GET /api/admin/projects |
| page | apps/web/app/(app)/admin/teams/page.tsx | listAdminTeams → GET /api/admin/teams |
| page | apps/web/app/(app)/admin/users/page.tsx | listAdminUsers → GET /api/admin/users |
| page | apps/web/app/(app)/layout.tsx | requireUser → GET /api/auth/session<br>listProjects → GET /api/projects |
| page | apps/web/app/(app)/profile/page.tsx | getProfileData → GET /api/profile |
| page | apps/web/app/(app)/projects/[id]/agents/page.tsx | listAgentSessions → GET /api/projects/:id/agent-sessions |
| page | apps/web/app/(app)/projects/[id]/algorithms/page.tsx | requireCurrentProjectPermission → GET /api/projects/:id (permissions; enforcement remains in each protected API)<br>listAlgorithmRuns → GET /api/projects/:id/algorithm-runs<br>listAlgorithmProviders → GET /api/projects/:id/algorithm-providers<br>listAlgorithmCatalog → GET /api/projects/:id/algorithm-definitions |
| page | apps/web/app/(app)/projects/[id]/algorithms/runs/[runId]/page.tsx | readAlgorithmRun → GET /api/projects/:id/algorithm-runs/:runId |
| page | apps/web/app/(app)/projects/[id]/assets/page.tsx | listProjectItems → GET /api/projects/:id/:kind |
| page | apps/web/app/(app)/projects/[id]/connectors/page.tsx | getProject → GET /api/projects/:id<br>listDeviceAdapters → GET /api/projects/:id/device-adapters<br>listFlightHubConnections → GET /api/projects/:id/connectors/dji-flighthub<br>listFlightHubDiscoveryActivity → GET /api/projects/:id/connectors/dji-flighthub/activity<br>parseFlightHubWebConfig → GET /api/projects/:id/features |
| page | apps/web/app/(app)/projects/[id]/devices/page.tsx | readProjectDeviceTree → GET /api/projects/:id/device-tree |
| page | apps/web/app/(app)/projects/[id]/events/[eventId]/page.tsx | readPerceptionEvent → GET /api/projects/:id/events/:eventId |
| page | apps/web/app/(app)/projects/[id]/events/page.tsx | legacyProjectEventListHref → client-only helper |
| page | apps/web/app/(app)/projects/[id]/issues/[issueId]/page.tsx | readIssue → GET /api/projects/:id/issues/:issueId<br>issueEvidenceSummary → client-only helper |
| page | apps/web/app/(app)/projects/[id]/issues/page.tsx | listIssues → GET /api/projects/:id/issues |
| page | apps/web/app/(app)/projects/[id]/layout.tsx | 纯页面/客户端导航 |
| page | apps/web/app/(app)/projects/[id]/page.tsx | getProject → GET /api/projects/:id<br>requireUser → GET /api/auth/session<br>readProjectSituationSnapshot → GET /api/projects/:id/snapshot |
| page | apps/web/app/(app)/projects/[id]/realtime/page.tsx | getProject → GET /api/projects/:id<br>requireUser → GET /api/auth/session<br>readProjectSituationSnapshot → GET /api/projects/:id/snapshot |
| page | apps/web/app/(app)/projects/[id]/settings/page.tsx | getProject → GET /api/projects/:id |
| page | apps/web/app/(app)/projects/[id]/tasks/page.tsx | listProjectItems → GET /api/projects/:id/:kind<br>listMissionRuns → GET /api/projects/:id/task-runs |
| page | apps/web/app/(app)/projects/[id]/tasks/runs/[runId]/page.tsx | readMissionWorkbench → GET /api/projects/:id/task-runs/:runId |
| page | apps/web/app/(app)/projects/new/page.tsx | listManagedTeams → GET /api/teams?scope=managed |
| page | apps/web/app/(app)/projects/page.tsx | listProjects → GET /api/projects |
| page | apps/web/app/(app)/teams/[id]/page.tsx | getTeam → GET /api/teams/:id |
| page | apps/web/app/(app)/teams/page.tsx | listTeams → GET /api/teams |
| action | apps/web/app/actions.ts#loginAction | POST /api/auth/login |
| action | apps/web/app/actions.ts#logoutAction | POST /api/auth/logout |
| action | apps/web/app/actions.ts#createTeamAction | POST /api/teams |
| action | apps/web/app/actions.ts#createProjectAction | POST /api/projects |
| route | apps/web/app/api/admin/ai-providers/[providerId]/route.ts | PATCH /api/admin/ai-providers/:providerId |
| route | apps/web/app/api/admin/ai-providers/[providerId]/route.ts | DELETE /api/admin/ai-providers/:providerId |
| route | apps/web/app/api/admin/ai-providers/[providerId]/test/route.ts | POST /api/admin/ai-providers/:providerId/test |
| route | apps/web/app/api/admin/ai-providers/route.ts | GET /api/admin/ai-providers |
| route | apps/web/app/api/admin/ai-providers/route.ts | POST /api/admin/ai-providers |
| route | apps/web/app/api/auth/[...nextauth]/route.ts | GET /api/auth/* (replaced by explicit auth endpoints) |
| route | apps/web/app/api/auth/[...nextauth]/route.ts | POST /api/auth/* (replaced by explicit auth endpoints) |
| route | apps/web/app/api/media-auth/route.ts | POST /api/media-auth |
| route | apps/web/app/api/projects/[id]/agent-sessions/[sessionId]/messages/route.ts | POST /api/projects/:id/agent-sessions/:sessionId/messages |
| route | apps/web/app/api/projects/[id]/agent-sessions/route.ts | POST /api/projects/:id/agent-sessions |
| route | apps/web/app/api/projects/[id]/algorithm-definitions/[definitionId]/route.ts | PUT /api/projects/:id/algorithm-definitions/:definitionId |
| route | apps/web/app/api/projects/[id]/algorithm-definitions/route.ts | GET /api/projects/:id/algorithm-definitions |
| route | apps/web/app/api/projects/[id]/algorithm-definitions/route.ts | POST /api/projects/:id/algorithm-definitions |
| route | apps/web/app/api/projects/[id]/algorithm-providers/[providerId]/route.ts | PATCH /api/projects/:id/algorithm-providers/:providerId |
| route | apps/web/app/api/projects/[id]/algorithm-providers/[providerId]/test/route.ts | POST /api/projects/:id/algorithm-providers/:providerId/test |
| route | apps/web/app/api/projects/[id]/algorithm-providers/route.ts | GET /api/projects/:id/algorithm-providers |
| route | apps/web/app/api/projects/[id]/algorithm-providers/route.ts | POST /api/projects/:id/algorithm-providers |
| route | apps/web/app/api/projects/[id]/algorithm-runs/[runId]/retry/route.ts | POST /api/projects/:id/algorithm-runs/:runId/retry |
| route | apps/web/app/api/projects/[id]/algorithm-runs/route.ts | POST /api/projects/:id/algorithm-runs |
| route | apps/web/app/api/projects/[id]/assets/[assetId]/access/route.ts | GET /api/projects/:id/assets/:assetId/access |
| route | apps/web/app/api/projects/[id]/assets/[assetId]/content/route.ts | GET /api/projects/:id/assets/:assetId/content |
| route | apps/web/app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/route.ts | DELETE /api/projects/:id/connectors/dji-flighthub/:connectorId |
| route | apps/web/app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/sync/route.ts | POST /api/projects/:id/connectors/dji-flighthub/:connectorId/sync |
| route | apps/web/app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/token/route.ts | PUT /api/projects/:id/connectors/dji-flighthub/:connectorId/token |
| route | apps/web/app/api/projects/[id]/connectors/dji-flighthub/projects/route.ts | POST /api/projects/:id/connectors/dji-flighthub/projects |
| route | apps/web/app/api/projects/[id]/connectors/dji-flighthub/route.ts | GET /api/projects/:id/connectors/dji-flighthub |
| route | apps/web/app/api/projects/[id]/connectors/dji-flighthub/route.ts | POST /api/projects/:id/connectors/dji-flighthub |
| route | apps/web/app/api/projects/[id]/device-adapters/[adapterId]/route.ts | PATCH /api/projects/:id/device-adapters/:adapterId |
| route | apps/web/app/api/projects/[id]/device-adapters/[adapterId]/test/route.ts | POST /api/projects/:id/device-adapters/:adapterId/test |
| route | apps/web/app/api/projects/[id]/device-adapters/discoveries/[identityId]/bind/route.ts | POST /api/projects/:id/device-adapters/discoveries/:identityId/bind |
| route | apps/web/app/api/projects/[id]/device-adapters/dji-setup/route.ts | POST /api/projects/:id/device-adapters/dji-setup |
| route | apps/web/app/api/projects/[id]/device-adapters/route.ts | GET /api/projects/:id/device-adapters |
| route | apps/web/app/api/projects/[id]/device-adapters/route.ts | POST /api/projects/:id/device-adapters |
| route | apps/web/app/api/projects/[id]/devices/[deviceId]/commands/route.ts | POST /api/projects/:id/devices/:deviceId/commands |
| route | apps/web/app/api/projects/[id]/devices/[deviceId]/live-streams/route.ts | POST /api/projects/:id/devices/:deviceId/live-streams |
| route | apps/web/app/api/projects/[id]/events/[eventId]/actions/route.ts | POST /api/projects/:id/events/:eventId/actions |
| route | apps/web/app/api/projects/[id]/events/[eventId]/agent-drafts/route.ts | POST /api/projects/:id/events/:eventId/agent-drafts |
| route | apps/web/app/api/projects/[id]/events/route.ts | GET /api/projects/:id/events |
| route | apps/web/app/api/projects/[id]/issues/[issueId]/actions/route.ts | POST /api/projects/:id/issues/:issueId/actions |
| route | apps/web/app/api/projects/[id]/live-streams/[streamId]/playback/route.ts | GET /api/projects/:id/live-streams/:streamId/playback |
| route | apps/web/app/api/projects/[id]/live-streams/[streamId]/stop/route.ts | POST /api/projects/:id/live-streams/:streamId/stop |
| route | apps/web/app/api/projects/[id]/realtime-channels/[channelId]/events/route.ts | GET /api/projects/:id/realtime-channels/:channelId/events |
| route | apps/web/app/api/projects/[id]/replay/route.ts | GET /api/projects/:id/replay |
| route | apps/web/app/api/projects/[id]/reports/[reportId]/export/route.ts | GET /api/projects/:id/reports/:reportId/export |
| route | apps/web/app/api/projects/[id]/reports/[reportId]/publish/route.ts | POST /api/projects/:id/reports/:reportId/publish |
| route | apps/web/app/api/projects/[id]/snapshot/route.ts | GET /api/projects/:id/snapshot |
| route | apps/web/app/api/projects/[id]/task-runs/[runId]/audit-trace/route.ts | GET /api/projects/:id/task-runs/:runId/audit-trace |
| route | apps/web/app/api/projects/[id]/task-runs/[runId]/control/route.ts | POST /api/projects/:id/task-runs/:runId/control |
| route | apps/web/app/api/projects/[id]/task-runs/[runId]/emergency-stop-drill/route.ts | POST /api/projects/:id/task-runs/:runId/emergency-stop-drill |
| route | apps/web/app/api/projects/[id]/task-runs/[runId]/reports/route.ts | POST /api/projects/:id/task-runs/:runId/reports |
| page | apps/web/app/layout.tsx | cn → client-only helper |
| page | apps/web/app/login/page.tsx | auth → GET /api/auth/session |
| page | apps/web/app/page.tsx | 纯页面/客户端导航 |
