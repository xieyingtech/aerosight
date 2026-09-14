共 32 项：P0 第 1–7 组 24 项，P1 第 8 组 6 项，F 第 9 组 2 项。以下均是待实现任务，不因客户端已有代码或历史勾选自动算完成。后续增强见 design，不是本次归档的隐含必做项。

## 1. P0 接入核验与最小数据契约（3 项）

- [ ] 1.1 核验当前司空项目、可读航线/已完成飞行/媒体/AI 告警和实际算法类别，登记真实图片授权与现有算法/AI Provider；交付不含密钥的可用性表，逐项区分文档能力、代码已有、当前只读实测及未验证动作，确定一条适用的真实识别演示配置。（flight-planning、perception、acceptance）
- [x] 1.2 定义 observation/evidenceSet/assessment 及业务 Run—远端资源关联的最小契约，登记来源范围、稳定身份、媒体版本与权限；用项目隔离、同资产多 Run 和远端投影 Run 不被覆盖的契约案例验证。（flight-planning、perception、step-execution）
- [x] 1.3 增量迁移所需作者 revision/DSL 版本、调度记录与委托者、观察/研判及来源防重约束，优先复用旧资源表；同步 schema/sqlc，验证空库、旧库升级、重复迁移与 `pnpm db:check`，历史版本/旧案不重写。（全部规格数据约束）

## 2. P0 Task 作者入口（4 项）

- [x] 2.1 实现 v2 YAML/JSON 安全解析、规范化和本轮能力/引用发布校验；验证同义输入等价、非法 YAML、后序引用、未部署能力和跨项目资源均被拒绝，v1 兼容。（task-workflow-authoring）
- [x] 2.2 补 Task 本体与 disabled 首稿原子创建、无副作用校验、草稿 expectedRevision 和发布快照；经真实数据库 HTTP 测试验证创建重试、修订冲突及旧版本不可变。（task-workflow-authoring）
- [x] 2.3 实现 YAML 编辑与模板基础参数表单共用草稿，涵盖输入资源、manual/schedule、识别来源及研判参数；浏览器验证双向修改/保存重开，非法文本保留、合法自定义字段不被表单丢弃。（task-workflow-authoring）
- [x] 2.4 提供 existing-flight/assets 两条模板、新建/启停/发布/手动运行入口及明确委托者；从空项目经正式 API 发布并试跑，停用不取消活动 Run，无权启用被拒绝。（task-workflow-authoring、task-trigger-runtime）

## 3. P0 本轮运行时补齐（3 项）

- [x] 3.1 为 manual/schedule 接好 inputs 合并、递归校验、Cron/时区、明确委托者重检及 missed 记录；用可控时钟验证 60 秒窗口、坏计划隔离、重启不补飞、撤权及手动试跑不改下次计划，旧触发来源回归。（task-trigger-runtime）
- [x] 3.2 实现 occurrence 防重及 Task 跨版本并发为 1，在创建时重检启停；真实数据库并发验证重复扫描同 Run、旧版本运行时新版本不能突破并发、停用竞争不生成新运行。（task-trigger-runtime）
- [ ] 3.3 为本次步骤接入依赖/条件/引用/输出验证、非设备输入、幂等恢复及取消/迟到结果边界；验证无设备图片路径、错误输出不推进、重启不重复副作用，远端投影成功不提前结束业务 Run。（task-step-execution）

## 4. P0 观察与识别连接（4 项）

- [x] 4.1 实现 inspection.observe 的 existing-flight/assets 输入及不可变清单，复用媒体查询和授权签名刷新；验证远端计数/列表对账、缺失/截断为 partial、人工确认有限范围、跨 flight 拒绝、重复绑定不改原资产归属。（inspection-flight-planning、inspection-perception-pipeline）
- [x] 4.2 增加连接器显式 Task 管理策略与归属待定告警暂存，调整新建、更新及 missing-alert 路径仅写证据；验证早到/重放告警和资源缺失均不绕过研判改案，非管理资源保持 legacy 行为，旧案只能明确确认关联。（inspection-issue-lifecycle）
- [x] 4.3 实现 inspection.detect 的 flighthub-ai 映射，保留来源、算法类别、位置语义和范围；验证 0 告警且算法启用未知不等于 no_issue，target_value 不假定为置信度，照片位置不当目标坐标。（inspection-perception-pipeline）
- [ ] 4.4 实现 inspection.detect 的 external 薄适配，复用现有 algorithm_run 与真实 Provider，支持有限图片集合/配置上限、子项幂等及结果汇总；验证真实图片调用、零结果、单图失败、超限拒绝及原 asset.task_run_id 不变。（inspection-perception-pipeline）

## 5. P0 真实 AI 与人工复核（3 项）

- [ ] 5.1 为 copilot.run 增加无 issue 的 assessment 上下文，接已有 Provider 和只读证据工具，保存模型/提示/证据版本；验证空案件库真实调用成功，旧 issue Copilot 回归，证据指令不扩大权限。（task-agent-assessment）
- [ ] 5.2 校验结构化 decisions 与引用，明确 no_issue 的已分析范围、缺资料 needs_review 及模型错误；验证捏造证据/非法结构被拒绝，超时不伪造无异常，单期建筑图不认定新增违法。（task-agent-assessment、inspection-perception-pipeline）
- [x] 5.3 实现整批暂停、复核 revision、确认/调整/驳回 API 与幂等恢复；验证原始输出保留、重复复核不重飞不重识别、取消/过期拒绝以及普通 resume 无法绕过复核。（task-agent-assessment、task-step-execution）

## 6. P0 案件与报告（3 项）

- [x] 6.1 让 issue.create-or-update 消费正式 assessment 并以稳定来源而非 taskVersion 防重；验证跨版本同告警/检测不重复建案/计数/重开，模糊 update 和闭案新观察转复核。（inspection-issue-lifecycle）
- [x] 6.2 将案件、证据关联、审计、幂等与步骤完成原子提交；用并发与故障案例验证事务回滚无半批案件、待复核无写入、no_issue 成功返回空列表。（inspection-issue-lifecycle）
- [x] 6.3 复用报告和案件反馈，补 Run/飞行/原图/识别/研判链接、分析范围与失效证据提示；验证零案件报告、待复核摘要和跨项目拒绝。（inspection-issue-lifecycle、inspection-workflow-acceptance）

## 7. P0 页面与业务交付（4 项）

- [ ] 7.1 完成资源就绪检查、模板入口和 Run 步骤详情，展示既有数据/模拟/真实来源、媒体范围、识别来源及失败原因；浏览器验证从任务进入原图、识别和研判，断线刷新恢复真实状态。（inspection-workflow-acceptance）
- [x] 7.2 接入复核表单及案件/报告导航，保留已有指派反馈；浏览器验证一次确认建案、一次驳回不建案及修订冲突，低质量坐标标签不误导。（task-agent-assessment、inspection-issue-lifecycle）
- [ ] 7.3 经正式 API/runtime 完成 P0 E2E 正常/零检测/复核/重放/重启/取消/跨项目矩阵，并在授权冻结样本上运行真实识别和 AI；交付逐样本标注/预测、误报漏报、模型/参数与耗时，至少各一条异常、不建案、复核链，缺依赖单列 blocked。（inspection-workflow-acceptance）
- [ ] 7.4 补部署/模板操作、司空配置分工与中文路演材料，完成新项目第二次复现；运行 `pnpm check`、`pnpm build` 及相关数据库集成检查，列出跳过项和 P0 逐项证据，材料不得将既有图像分析称为现场航拍。（inspection-workflow-acceptance）

## 8. P1 定时远端飞行接通（6 项，复用 P0 后半段）

- [ ] 8.1 增加司空航线/设备选择、来源预览和可比较版本冻结，扩展 flighthub-flight 模板；验证预览无动作、跨项目拒绝、航线改动/无法验证时执行被阻塞，不实现本地覆盖算法。（inspection-flight-planning、task-workflow-authoring）
- [ ] 8.2 将 observe 接入既有 flight_action 预检/授权/提交作业，固定 schedulerOwner=aerosight 和 task_type=immediate，复用本地提交互斥；验证 blocking error/撤权不提交，同一模板拒绝 recurring/timed 双重调度。（inspection-flight-planning、task-trigger-runtime）
- [ ] 8.3 在现有 ledger 建立业务 Run/step 与远端 flight UUID 绑定，派发前启用告警暂存策略；验证成功但 ACK/ID 丢失时暂停对账、不自动重飞，早到告警不会抢先建案。（task-step-execution、inspection-issue-lifecycle）
- [ ] 8.4 接远端飞行终态与媒体就绪到观察步骤，复用 P0 清单并展示预检/提交/飞行/回传子状态；验证 ACK 不结束、缺图继续等待、超时明确、不同架次不串图。（inspection-flight-planning、inspection-perception-pipeline）
- [ ] 8.5 明确取消/暂停与远端物理状态，提供司空或已验收控制入口并保留未知占用标记；验证业务取消不显示飞机已停，迟到结果不继续建案，计划状态更新不冒充在飞控制。（task-step-execution、inspection-flight-planning）
- [ ] 8.6 用可控时钟和标记清楚的远端协议替身贯通 cron→一次 immediate→飞行终态→媒体→P0 后半段；验证重复扫描/服务重启不重复派发，运行相关检查并单列软件证据，不能计为实机通过。（task-trigger-runtime、inspection-workflow-acceptance）

## 9. F 独立现场验收（2 项）

- [ ] 9.1 核实当前账号套餐/项目、目标设备、司空预设区域或巡逻航线、现场窗口及动作授权；在现有授权机制下验证目标动作的预检、提交/对账和必要的现场控制，保存实际结果，未知或缺条件保持未完成。（inspection-flight-planning、inspection-workflow-acceptance）
- [ ] 9.2 完成一次真实定时触发预设航线的飞行、完整媒体回传、适用真实识别、AI、issue/不建案及报告，保存全链关联与录像；独立发布 P0/P1/F 结论，只有三层通过才宣称原始闭环完成，无定位实测不宣称米级精度。（inspection-workflow-acceptance）
