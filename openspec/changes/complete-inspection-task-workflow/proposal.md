## Why

AeroSight 已有 Task、司空连接器、算法、Copilot 和案件模块，但缺少可重复演示的业务闭环。根据 2026-09-11 的司空 2 中国公有云 OpenAPI V2 与官方产品手册调研，本次以最快完成 MSUP 参赛闭环为目标，优先使用司空航线、飞行、媒体和适用的 AI 告警，集中开发跨步骤编排、证据研判与 issue。官方接口存在、仓库客户端实现和当前账号实测成功是三种不同状态，不混称已交付。

## What Changes

- Task 仍是类似 GitHub Actions 的自定义声明式顺序工作流，定时只是触发器。首期支持 YAML/JSON 和同一草稿的模板参数表单；完整步骤可视化设计器后续增强。
- 司空负责区域/巡逻航线编辑及设备执行。AeroSight 选择司空已配置航线，通过已有连接器提交一次 immediate 飞行并跟踪；本次取消自研 polygon 覆盖算法、飞控和航线 Agent。
- 首先打通已有飞行或导入真实图片的只读分析路径；随后交付 AeroSight 定时触发一次司空飞行的路径。前者不能冒充新发起实机闭环。
- 使用司空 AI 告警作为一种识别结果；不适用的业务场景使用已有算法 Provider 接一个真实识别服务，已有 AI Provider 负责业务研判。算法来源枚举不等于任意算法可通过司空 API 调用。
- 新增轻量 observation 证据清单，关联业务 Run、远端飞行、媒体和检测；不覆盖原资产归属。Task 接管的告警不再绕过研判自动建案。
- 增加不依赖 issue 的结构化研判、人工复核、幂等建案及无案件报告；复用现有案件和报告模块。
- 交付拆为 P0 业务闭环 24 项、P1 定时远端飞行接通 6 项、F 现场验收 2 项，共 32 项。P0 是先行可演示成果，原始定时航拍目标需要 P0 + P1 + F，不以缩小验收口径声称全部完成。

### Goals and Non-Goals

目标：先验证「已有数据 → 真实识别结果 → 真实 AI → issue/不建案/复核 → 报告」，再验证「定时 Task → 司空预设航线执行 → 回传 → 同一业务闭环」。疑似违建保留为业务示例，但不得将人车船识别宣称为违建识别；缺少历史或合法性资料只能给出疑似线索或待复核。

本变更不包含：GitHub Actions 完整兼容、shell/container runner、多 Job/矩阵/DAG、六类触发器全面重构、通用拖拽设计器、自研覆盖航线/避障/飞控、动态选机、自动精确目标投影与跨图空间对象库、训练视觉模型、全量司空管理接口扩展、三维重建引擎、执法认定。司空原有计划自动订阅触发列为后续，P0 可以手动选择其已完成飞行。

## Capabilities

### New Capabilities

- `task-workflow-authoring`：Task 创建、版本化 YAML/JSON、模板参数表单双向同步、发布启停与兼容。
- `task-trigger-runtime`：首期 manual/schedule 的输入、授权、幂等、任务并发和单一调度归属。
- `task-step-execution`：本次能力的顺序依赖、输出引用、恢复与副作用约束。
- `inspection-flight-planning`：复用司空规划，选择航线、关联既有飞行或执行一次远端飞行；名称保留但不再要求自研规划算法。
- `inspection-perception-pipeline`：观察清单、司空告警/外部识别、完整性与位置来源。
- `task-agent-assessment`：建案前真实 AI 研判、证据校验、复核。
- `inspection-issue-lifecycle`：由 Task 决策写案件、来源防重、旧同步兼容和证据报告。
- `inspection-workflow-acceptance`：P0/P1/F 分层验收、产品路径、真实模型证据及交付材料。

### Modified Capabilities

无。本变更补充新规格，保留现有主规格的权限与兼容约束，不改写其他尚未归档 change。

## Impact

复用 `flighthub` 的 client/action/projector/asset access、Task runtime、algorithm、agent、issue、report 和现有 UI；仅补业务连接。Go 单服务、静态导出前端、PostgreSQL/sqlc、现有 outbox 和 Provider 配置不变。

增量迁移补作者版本、触发记录、业务 Run 与远端资源绑定、观察清单和研判记录。项目/团队隔离、执行授权重检、不可变版本和敏感凭据边界继续有效。现场账号、航线、样本与 Provider 在首项核验登记；没有实测的接口不计入实机通过。
