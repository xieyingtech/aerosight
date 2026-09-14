## Context

动机与交付范围见 [proposal.md](proposal.md)。本次只重新规划，未修改业务代码。调研日期 2026-09-11，适用司空 2 中国公有云 OpenAPI V2；私有云、其他地区和其他 DJI API 不自动类推。

### 外部能力与职责依据

| 能力 | 已核实公开证据 | 本次决策 |
| --- | --- | --- |
| 区域覆盖、巡逻航线 | 官方 UI 支持区域规划、参数编辑及人车船巡逻识别 [S2][S3] | 在司空配置，AeroSight 只选择引用；未找到 polygon 到航线的公开生成 API |
| 自然语言航线 Agent | 司空 UI 已提供自然语言生成和编辑 [S4] | 不重复开发；未找到可供本平台直接调用的公开 Agent API |
| 航线资产 | API 查询/下载，STS 上传后 finish-upload [S5] | 复用现有目录；首期无需新建上传链路 |
| 飞行任务 | 创建支持 immediate/timed/recurring/continuous [S6] | AeroSight cron 只调用 immediate；不同时建立司空周期计划 |
| 飞行预检与状态 | dispatch-checks 返回提示但不阻止创建；状态更新接口限定计划类型 [S7][S8] | 接现有动作策略/审批；不把计划停用当作在飞飞机停止 |
| 飞行媒体 | 查询指定飞行媒体，最多返回前 10000 个资源 [S9] | 按 flight UUID 对账，复用临时访问刷新，不以首张图代表完成 |
| AI 告警 | 记录可按 flight_id 筛选；来源含官方、AI-LINK、第三方、LLM [S10] | 复用已有识别结果；来源枚举不能证明任意算法配置/推理 API 可用 |
| 定位 | 告警 location 可能是机载或照片坐标 [S10] | 明确来源和质量，不能当作精确目标坐标 |
| 三维模型 | 有模型创建、查询、下载能力 [S11] | 将来有需求再接，本轮无重建引擎任务 |
| 业务 Task / 研判 / issue | 本次公开目录未发现本项目所需的跨步骤业务契约 | AeroSight 实现差异化闭环，复用已有 AI 和案件模块 |

证据分级：公开接口目录与仓库 `contracts/dji-flighthub/v2/endpoints.tsv` 的 89 个 endpoint ID 一致；这说明接口覆盖清单一致，不能证明账号权限和端到端成功。2026-09-02 历史验收有 13 条航线、0 条可访问飞行；这是历史记录，不代表当前账号。当前轮次未调用带凭据的飞行写接口。产品界面能力与 OpenAPI 能力分开记录。

### 本地复用与缺口

| 入口 | 复用与必要调整 |
| --- | --- |
| `httpapi/task_drafts.go` | 已有版本编辑，补 Task 本体创建、YAML 和发布契约 |
| `tasktrigger/scheduler.go` | 复用 cron，补真实 inputs、明确委托者和跨版本任务并发 |
| `mission/processor.go`、`scheduler.go` | 复用顺序引擎，只补本次步骤的依赖/输入输出/恢复；非设备路径解除选机前置条件 |
| `flighthub/flight_client.go`、`flight_action.go` | 已有飞行创建、预检和作业对账，不再另写 DJI SDK/任务队列 |
| `flighthub/flight_projector.go` | 已有远端任务投影；必须区别于业务 Run，远端成功只结束采集步骤 |
| `flighthub/flight_alert_projector.go` | 当前 `projectAIAlert` 直接 `createAlertIssue`，更新及缺失同步还会改案件；Task 接管路径必须全部隔离这些写入 |
| `flighthub/flight_asset_access.go` | 复用签名刷新和授权下载，长期记录只存稳定资源引用 |
| `mission/collection.go` | 旧首个 available 资产完成语义不适合整架次，新路径使用 observation 清单 |
| `algorithm/trigger.go` | 复用真实单图执行，补有限多图汇总和授权引用，避免改写原 asset.task_run_id |
| `agent/task_step.go`、`issue/task_step.go` | 补无 issue 研判与跨 Task 版本来源防重，保留旧模式 |

## Goals / Non-Goals

**Goals:** 最少新增概念：一个业务 Run、一个观察清单、一次正式研判、一组确定性案件写入；现有异步账本贯通。P0 不依赖在线飞机，P1 不依赖另建调度系统，F 如实验证实际飞行。

**Non-Goals:** 本轮不建立通用供应商框架、空间对象平台或另一套工作流引擎。模板表单属于基础可视化编辑，完整可视化步骤设计器仍是后续产品方向，不以本轮表单宣称已经完成。

## Decisions

### 1. 固定业务契约，缩小作者界面

保留 `apiVersion: aerosight/v2`、`trigger/steps/uses/with` 和规范化 JSON。无版本历史定义按 v1 处理，不重写历史快照。YAML 只接受 JSON 兼容类型，拒绝重复键、别名/锚点、标签、多文档和超限输入。source、normalizedDefinition、hash、revision 一并保存；发布展开能力版本、默认值和资源引用并冻结。能力不存在、资源越界或引用后序输出时拒绝发布。

新增 Task 与 disabled 草稿须原子幂等；保留既有版本/Run 路径，新增创建和无副作用校验入口。草稿更新使用 expectedRevision。基础表单编辑输入资源、触发时间、识别来源和研判参数，与 YAML 共用草稿；不能表达的合法自定义步骤仍保留 YAML，不静默覆盖。无效 YAML 保留文本但禁止发布。暂不建设 schema 全自动表单、步骤拖拽和条件树设计器。

### 2. 两条模板共用后半段

P0 `inspection.observe` 接受 `existing-flight` 或 `assets`，只绑定可授权的既有证据，无飞行写操作。P1 同一能力新增 `flighthub-flight`，提交一次 immediate 飞行后等待证据。这样少一个自研 flight.plan/低层 collect 编排环节，但 UI 必须展示预检、提交、飞行与回传子状态。

`inspection.detect` 接受 observationId 和 source=flighthub-ai/external。这是现有告警同步/算法批次的薄步骤入口，不是新检测平台。输出 evidenceSetId；`copilot.run(mode=assessment)` → `issue.create-or-update` → `report.generate` 共用。旧 flight/device/algorithm 能力不删除，本模板无需依赖它们的通用重构。

两种识别来源必须显式配置，不在失败后静默切换供应商。用于疑似违建的模板需要适用的真实外部识别服务与历史资料；司空人车船演示可先验证通用事件闭环，但必须标明场景差异。提供方选择只改变配置，不改变步骤契约。

### 3. 单一调度所有者

首期改动仅覆盖 manual/schedule 新模板，已有其他来源保持兼容，不要求全面重构。手动运行 schedule Task 不改变下一次计划。schema defaults → trigger.inputs → invocation.inputs 按键合并后递归校验；发布/启用绑定授权用户，运行与动作前重检，禁止回退项目创建者。

occurrence 唯一键为 project+task+UTC 时间，任务级锁跨版本限制并发为 1。首期过期超过 60 秒跳过，不补飞；停机区间记 missed，坏计划独立记录错误。停用只阻止新运行。

P1 schedulerOwner=aerosight，远端只允许 immediate；UI 明确提示同一巡检不可另配司空周期计划。首期不承诺发现所有司空人工计划或全局占机；设备状态及冲突由远端预检和既有动作服务核验。已由司空调度的巡检仅能以 P0 手动选择已完成架次分析；自动订阅其完成事件后续实现，避免首期维护两个调度归属模式。

### 4. 远端飞行与业务 Run 的关联

以 project+connector+remoteFlightUUID 识别远端架次，另存 businessRunId/stepId，不覆盖 connector 投影 Run。观察清单可引用同一个架次供多次只读分析，但同一步提交只允许一个飞行作业。写前登记 Task 管理策略和设备派发意图，以现有作业 ledger 关联远端 ID。

远端请求超时、提交成功但本地未收到 ID 时进入 unknown/paused，复用已有对账，不能自动再飞。为防远端告警早于绑定，本次由明确的项目连接器开关启用 task-managed 告警模式：该模式中新发现且归属未确定的告警暂存，不走自动建案；确认非本次管理飞行后才按 legacy 策略释放。既有已投影案件不自动删除或重新归属；P0 选择旧架次时仅展示旧案为候选，需确认才关联。这样保留 legacy 的同时关闭先到告警绕过研判的竞态。

司空准备航线，发布记录 connector、wayline UUID、远端更新时间/内容哈希（可取得时）和设备。执行前刷新航线身份及版本；检测变化须重新确认发布，取不到可比较版本时须下载校验或阻塞，不能承诺远端原地编辑仍是旧航线。无需自研航线文件或上传。

复用现有预检、动作审批/授权、连接器作业和本地设备提交互斥；有 blocking error 不下发，警告按现有策略处理并展示。无法验证状态不能猜成功。取消业务 Run 不等于停止飞机；暂停、返航等现场操作继续经司空或已验收控制入口处理，业务 UI 显示远端可能仍在飞行。未知状态保留占用/待核实标记。

### 5. 观察清单、完整性与识别

observation 保存来源模式、项目、Run/step、connector/flight 引用、选定媒体和不可变版本、目标业务类别、观测时间范围与 completeness。资产导入路径由用户明确封闭输入集合；不要求设备在线，也不修改原资产归属。

远端飞行需终态成功后对账文件列表与 folder_info 的 expected_file_count/uploaded_file_count（已存在本地契约，实际可用性须联调核验）；数量口径需一致并验证每个选定文件可读。可用数量不足继续等待到步骤截止。若计数缺失/不可靠，只能记录稳定快照为 partial，用户可确认按该有限范围分析，不能据此声明全架次完整。接口 10000 上限或分页截断必须显式报范围不完整，不静默截取。只选告警图片时标记 alert-only。

检测证据记录原始来源、算法类别、模型版本（未知则明示）、证据引用、范围与错误。外部服务复用现有 algorithm_run，首期默认并发 2，最大图片数为发布配置且在执行前校验，超出拒绝并提示缩小明确范围，不引入通用批处理调度器。子项幂等键由 step+assetVersion+modelConfig 决定；所有选定项成功才将这一范围标为检测完成。

0 条告警不代表执行过目标检测；只有确认目标算法启用且处理完整的范围才可给出该范围的 no_issue。算法未启用、媒体缺失、上游超时不能当零检测。location 单独记录 target/capture/unknown 和质量/CRS；照片 GPS 与未知告警坐标仅作上下文，低质量只展示影像位置，不做跨图地物自动匹配。target_value 不直接转换为置信度。

### 6. 本次步骤运行约束

v2 执行边界验证依赖成功、条件、引用、资源权限、输入 schema，结束验证输出；false 条件无副作用。后序引用和无法执行能力在发布时拒绝。只支持本轮顺序模板，默认 onFailure=abort，外部状态未知用 pause，不新增通用 continue 策略。远端/模型调用不持有数据库长事务，复用作业租约/outbox。

已成功步骤恢复时不再提交。取消或暂停后的迟到结果仅记录，不推进建案；整条业务 Run 不能因远端投影成功提前结束。非设备输入无需 selectedDeviceId。重试分类由能力控制，用户 YAML 不能将飞行未知 ACK 改成可自动重试。

### 7. AI、复核与案件

研判根为 evidenceSetId，不要求已有 issue。复用现有 Provider 和只读项目上下文工具；记录 provider/model、提示模板版本、证据版本、原始输出及校验结果。输出 decisions[]，每项为 create/update/no_issue/needs_review，引用系统给定 candidateId/evidenceRefs，含理由、缺失信息和位置质量。模型不能自造对象/issue ID 或执行写工具。资料内容只作证据，不能扩大权限。没有对比历史或合规依据时不得认定“新增违建”。

任一 needs_review 暂停整批案件写入；人工确认/调整/驳回生成新 revision，保留原始输出，以幂等复核 API 恢复到案件步骤，不重跑飞行/识别。普通 resume 不能绕过未完成复核。结构错误、无权、模型超时按失败记录，不伪造 no_issue。

首期案件唯一来源键为 project+connector+alertUUID，或 project+assetVersion+detectionKey+problemType；不含 taskVersion。相同来源重放不重复建案、加计数、重开旧案。不同架次没有可靠业务对象标识时不做自动同地物合并，update 必须引用本项目已有明确关联或人工确认的候选案件。已关闭案件遇到新观察时进入复核，首期不实现自动复发引擎。

Task 管理的告警同步只更新证据，禁止创建、更新、关闭或重开其案件；检查新建、已存在告警更新及 missing-alert 恢复三条路径。未接管资源按旧策略运行。案件和证据关联、审计、幂等记录、步骤完成在同一事务提交。no_issue 不建案，report 从 Run 已提交状态生成，零案件也有效；复核期间显示未完成摘要，确认后生成最终报告。

### 8. 计划中的 YAML 示例

以下是待实现的 v2 作者契约，不是当前可直接运行配置。ID 为示例；引用语法沿用现有受 schema 限制的 `inputs.*` / `steps.*.outputs.*`，不采用 GitHub Actions 表达式执行器。

```yaml
apiVersion: aerosight/v2
name: 每日巡检
trigger:
  type: schedule
  cron: "0 8 * * *"
  timezone: Asia/Shanghai
  inputs: {connectorId: 1, waylineUuid: route-example, deviceId: 2}
concurrencyLimit: 1
inputSchema:
  type: object
  properties:
    connectorId: {type: integer}
    waylineUuid: {type: string}
    deviceId: {type: integer}
  required: [connectorId, waylineUuid, deviceId]
steps:
  - key: observe
    uses: inspection.observe
    timeoutSeconds: 3600
    with:
      mode: flighthub-flight
      schedulerOwner: aerosight
      connectorId: inputs.connectorId
      waylineUuid: inputs.waylineUuid
      deviceId: inputs.deviceId
  - key: detect
    uses: inspection.detect
    dependsOn: [observe]
    with:
      observationId: steps.observe.outputs.observationId
      source: flighthub-ai
  - key: assess
    uses: copilot.run
    dependsOn: [detect]
    with:
      mode: assessment
      evidenceSetId: steps.detect.outputs.evidenceSetId
  - key: issue
    uses: issue.create-or-update
    dependsOn: [assess]
    with:
      assessmentId: steps.assess.outputs.assessmentId
  - key: report
    uses: report.generate
    dependsOn: [issue]
    with: {scope: current-run}
```

案件步骤在 no_issue 时成功返回空列表，报告依赖它无需复杂条件树。P0 模板把 trigger 改成 manual，observe 改为 existing-flight（connectorId+flightUuid）或 assets（assetIds）；同步替换 inputSchema/inputs。外部识别模板把 detect.source 改 external 并选择 algorithmDefinitionVersionId；两条路径都必须接真实 AI 研判。P0 可用 schedule 重分析固定证据验证调度软件，但界面与验收必须标记它未产生新航拍。

### 9. 实施顺序和后续边界

P0 为 tasks 1–7 共 24 项；P1 为第 8 组 6 项；F 为第 9 组 2 项。推荐先 1 → 4 的只读证据适配与 5/6 的研判建案，再接作者/调度 UI；依赖按每组任务描述处理，先跑服务端薄闭环再完善页面。P1 在 P0 共用后半段成立后实施。F 不阻止软件开发，但未通过不能宣布定时实机全闭环完成。

后续增强（不计入本变更待完成清单）：完整可视化步骤编辑器、司空既有计划完成事件触发、更多触发器统一治理、跨架次空间对象关联和精确定位、更多检测服务。航线需求继续优先复用司空；只有经证实外部平台无法覆盖的必要需求才另立自研规划规格。

## Risks / Trade-offs

- [告警来源并不覆盖疑似违建业务] → 第一阶段用代表图片验证，适用性不满足时使用配置化外部识别；业务缺历史则待复核。
- [司空 AI 告警无法证明全区域无异常] → alert-only/partial 明示范围，禁止直接宣称区域无异常。
- [账号、套餐、区域版本及设备动作不一致] → 分别记录文档、客户端、当前账号和现场结果；真实飞行默认门控。
- [临时媒体失效] → 复用按需刷新；仍不可获取则记录证据失效，不把失效链接当验收凭证。
- [Task 管理标记与告警同步竞态] → 项目连接器显式模式下先暂存未归属告警；核实来源后释放 legacy 路径。
- [外部计划争用或原地改航线] → 不承诺全局占机；动作前预检与版本比对，发现冲突暂停。
- [简化定位和去重产生人工复核量] → 首期接受人工确认，不用不可靠 GPS 自动合并问题。

## Migration Plan

1. 增量迁移作者/触发/观察/研判及远端绑定，复用现有资源表；同步 schema/sqlc，空库和旧库验证，历史版本不改写。
2. 部署兼容 v1/v2 的消费者，未部署能力拒绝发布。旧计划无授权委托者需重新启用，不回填项目创建者。
3. 为演示项目显式配置 Task 管理告警模式，列出已有 legacy 案件，只做确认关联，不批量删除/转换。
4. 依次完成 P0、P1 软件验证；F 在已有动作授权机制和现场条件下单独留证，开放范围仅限验收能力。
5. 回滚先停新计划并隔离 v2 队列、作业和迟到事件；保留新表与历史记录。未知远端飞行由现场核实，不让旧二进制重新消费新飞行提交作业。

## Open Questions

部署时登记实际可用的司空项目/设备/航线、套餐和 AI 服务型号、样本授权及现场窗口。它们影响可实测范围而非本次协议和任务结构；缺失须报告 blocked，不以 fixture 替代真实验收。

## References

- [S1 公有云 OpenAPI V2 手册](https://fh.dji.com/user-manual/cn/custom-development/open-api/public-cloud-v2.html) 与 [接口目录](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/llms.txt)
- [S2 区域航线编辑](https://fh.dji.com/user-manual/cn/flight-route-library/edit-area-flight-routes.html)
- [S3 AI 巡逻航线](https://fh.dji.com/user-manual/cn/flight-route-library/ai-patrol-routes.html)
- [S4 司空航线 Agent](https://fh.dji.com/user-manual/cn/copilot-assistant/copilot-wayline-edit.html)
- [S5 航线 API 教程](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/8785456m0)
- [S6 创建飞行任务](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/454273432e0)
- [S7 飞行预检](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/456425797e0)
- [S8 修改计划状态](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/458069502e0)
- [S9 飞行媒体](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/457309246e0)
- [S10 AI 告警明细](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/480437504e0)
- [S11 模型 API](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/8785471m0)
- [仓库接口覆盖](../../../contracts/dji-flighthub/v2/CAPABILITY-COVERAGE.md)、[运维手册](../../../docs/operations/dji-flighthub-runbook.md)、[历史验收](../expand-dji-flighthub-openapi-platform/acceptance-results.md)
