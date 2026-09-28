# 项目智能体巡检工具与验证状态

更新：2026-09-28。工具存在、API 接受、后台执行完成、真实设备完成是不同证据。

## 工具入口

文字与实时语音共用写工具目录。全部写工具只生成绑定当前项目、账号、会话和参数的待授权记录，用户须在界面点击授权或拒绝。模型不能提交授权决定，用户文字中的“同意”不能替代点击。

| 工具 | 行为 | 执行权限 |
|---|---|---|
| create_inspection_task | 新建停用任务和草稿 | mission:operate |
| create_task_draft | 为已有任务创建或复用草稿 | mission:operate |
| save_task_draft / publish_task | 保存或发布指定版本，检查 expectedRevision | mission:operate |
| set_task_state | 启停任务，检查 expectedVersionId；停用不取消活动 Run | mission:operate |
| run_task | 手动运行指定已发布版本，检查 expectedVersionId | mission:operate |
| control_task_run | 审批、暂停、恢复、取消、紧急停止；检查 expectedVersion | approve 为 mission:approve，其余 mission:operate |
| sync_flight_resources | 排队同步司空飞行及媒体目录，不启动飞行 | device:configure，沿用连接器 owner/admin 限制 |
| submit_flight | 排队提交一次 immediate 司空飞行 | mission:operate，并沿用全部飞行门控 |
| control_flight | 司空任务状态修改或恢复 | mission:operate，并沿用全部飞行门控 |
| run_algorithm | 将已有 asset 提交到发布的算法配置 | algorithm:manage |
| review_inspection | 提交用户确认的整批研判决定 | issue:handle |
| generate_report | 为终态 Run 生成报告草稿，不发布 | mission:operate |
| mutate_issue | 案件评论、状态、标签、指派 | 相应案件操作权限 |

所有操作继续要求 agent:use。执行时再次检查当前 RBAC 和业务资源条件。工具不接收用户、项目、会话、任意 URL、任意 HTTP 路径或 SQL。执行复用平台原业务 handler，不绕过资源隔离、审计、版本检查、功能开关或飞行安全审批。

## 只读查询与照片

`query_inspection` 的 resource：

- readiness：配置目录是否存在，不代表远程服务或设备验证成功。
- task_templates：与任务页面对应的 assets、existing-flight、flighthub-flight 模板，占位资源必须替换。
- validate_task：提供 definition 校验发布条件，无业务副作用。
- task：提供 taskId，获取工作台和版本。
- task_run / task_run_audit：提供 taskRunId，读取运行、研判摘要或已有预检、审批、命令审计记录；不会主动执行起飞预检。
- flight_operations / flight_plan_options：读取本项目司空目录。
- flight_plan：提供 connectorId、deviceId、waylineResourceId，只预览目录航线版本。
- flight_job：提供 connectorId、jobId，查询实际提交作业状态。
- observation / evidence_set / assessment / report / algorithm_run：提供相应 ID，查询封闭照片清单、识别证据、研判、报告或算法运行。
- algorithms：读取已配置算法目录。
- photo：提供 observationId、assetId。沿用原图 API 的 media 权限、封闭清单、版本和 SHA256 校验；文字模型收到最长边 1024 像素的 JPEG 预览。原图不变，算法仍读取原图。模型必须支持图片输入；实时语音模型不接收图片，明确提示改用文字。支持 JPEG/PNG/GIF 解码，其他格式明确失败。

## 授权结果和恢复

新写工具授权后先原子占用记录为 executing，再调用原 API。并发和重复点击不能再次执行同一授权。操作参数、资源版本和审批 ID 被固定；幂等入口使用服务端生成的授权 ID。

API 的 202 或作业 ID 表示已接受/入队，不能宣称起飞、识别或报告完成。页面显示返回的作业或 Run ID，需继续查询实际状态。失败保留平台错误。进程中断或结果不确定时保留 executing，提示核对平台后处理，不自动重试物理动作。

用户授权工具不等于飞行安全审批。submit_flight 仍要求已存在、未过期、范围匹配的 approvalRequestId、预检、安全策略、设备身份、航线及已验证能力开关。工具不会凭空生成这些记录。

## 可用性与证据

| 能力 | 已有证据 | 尚未证明 |
|---|---|---|
| 本次工具授权机制 | 真实 PostgreSQL 集成测试：授权前无业务写入、拒绝、过期、跨项目、撤权、修订冲突、并发重复点击；飞行缺条件不产生作业；报告草稿入口 | 实机执行效果 |
| 图片到算法、AI、复核、报告 | 2026-09-14 独立演示库：真实历史 RGB 图片、YOLO11n、真实 StepFun、演示复核、工单与报告草稿 | 整架次照片、准确率矩阵、完整现场闭环 |
| 司空资源和媒体 | 历史只读验证；2026-09-14 下载链取得 DAT 字节，详见媒体审计 | 完整真实航拍照片回传 |
| AeroSight 手动启动无人机 | 有 API、worker、治理和协议测试代码 | 没有平台手动启动并起飞成功的实机验收证据 |
| 新飞行巡检模板 | 页面和目录版本预览、草稿配置 | 新飞行模板仍受未部署能力发布门控；不承诺可运行 |
| 定时飞行到报告 | 部分实现和协议测试 | P1、现场 F 验收未完成 |

参考：[历史图片演示](../demo/local-demo-runbook.md)、[司空媒体审计](flighthub-media-readonly-audit-2026-09-14.md)、[巡检工作流待完成项](../../openspec/changes/complete-inspection-task-workflow/tasks.md)。

下一项实机验证应先通过平台页面完成单次预检、授权提交、飞行状态对账及照片回传，再验证智能体入口复用同一流程。本次实现和自动测试不发起真实飞行。

## 本次验证记录

- 配置 `AEROSIGHT_MIGRATION_TEST_DATABASE_URL` 指向本机 5432，测试夹具创建并清理隔离库。相关 HTTP API、聊天、语音、授权、作者、照片权限回归通过（51.075 秒），不是跳过数据库测试后获得的通过。
- `pnpm db:check`、`pnpm typecheck`、`pnpm build:server` 通过。0082 迁移已应用到原有本机业务库；继续使用 5432，无 Docker。前端旧生成类型文件损坏已重新生成。
- 本机默认真实提供商 `step-5-preview` 查询 readiness 并调用 create_inspection_task，生成会话 #5 的 pending 授权；授权前后任务数量均为 1。未点击授权，未创建该任务，也没有飞行动作。
- 当前本机项目 readiness：无司空连接器、无可用照片、无已发布 detection 配置。服务还未配置 DATA_DIR。工具接口可生成授权，不表示这些业务依赖已就绪。
- 照片预览和封闭清单权限通过合成图片集成测试；本次未用真实图片调用模型视觉能力或真实算法服务。报告入口通过隔离数据库夹具测试。
- 本次浏览器检查未完成：浏览器页面连接超时。授权记录和无提前写入已由正式 API 核对，前端仅完成类型检查，不能算浏览器交互验收。
