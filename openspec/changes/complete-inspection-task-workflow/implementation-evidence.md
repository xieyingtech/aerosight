# 实施与验收证据

## 2026-09-11：任务 1.1 接入核验（尚未完成）

仅通过本地 `.env.local` 对已配置数据库做只读核验；未打印密钥、远端 ID、坐标或签名 URL，未执行司空飞行操作。

| 项目 | 本次证据 | 状态 |
| --- | --- | --- |
| 本地数据库 | 连接成功 | 可用 |
| 本地项目 1 / 连接器 1 | 状态 connected；远端缓存含 13 条 wayline、2 个 model | 仅本地缓存，非当前上游实测 |
| 飞行/告警缓存 | 项目 1 / 连接器 1 未查到对应资源 | 不能验证既有飞行链路 |
| 项目 1 资产 | 2 条 available 资产 | 未确定航拍来源和样本授权，不能当作真实验收样本 |
| AI Provider | `ai_providers` 无配置 | blocked |
| 项目 1 算法 Provider | 无配置 | blocked |
| 凭据解密环境 | 当前环境及本地配置未设置 APP_SECRET；演示环境文件也未设置 | blocked，未尝试替换已有密钥 |
| 司空公开文档 | 航线、飞行、媒体、AI 告警 API | 参见 design；文档可用不代表账号实测通过 |

待补：用户指定参赛项目、恢复对应 APP_SECRET、配置适用识别与 AI 服务、登记真实图片位置/授权。不要在聊天或本文粘贴密钥。任务 1.1 不勾选。

## 2026-09-11：任务 1.2 最小契约

代码：`apps/server/internal/inspection/contracts.go` 与 `contracts_test.go`。

- RunRef 明确业务 Run/step；FlightRef 独立保留 connector+flight 与投影 Run。
- AssetRef 保存资产版本和原始 Run，只读验证不覆盖原归属；Observation 验证项目/团队、飞行归属与重复资产。
- MediaStatus 区分缺失计数、真实零、上传缺失、访问失败和列表截断；必须飞行成功且计数同口径才能给出完整。
- EvidenceSet 保存明确识别来源、模型版本、证据范围和目标算法是否确认；坐标区分 target/capture/unknown。
- Assessment 校验候选与证据引用、同项目/业务 Run、允许更新的案件，未知算法/局部范围不支持 no_issue；needs_review 可以阻止后续写案。
- 来源键不含 Task 版本或分析 Run；相同来源重试稳定，不同项目/资产版本隔离。

验证：`cd apps/server && go test ./internal/inspection` 通过，无外部依赖跳过。此项只完成共享数据契约及契约测试；尚未接入数据库、HTTP 或正式 Task handler，不代表端到端能力已交付。

## 2026-09-11：任务 1.3 增量持久化

新增 `0073_inspection_workflow_contracts.sql` 并同步 `db/schema.sql` 与 sqlc 模型：作者版本、委托者、触发记录、观察/资产绑定、证据集、研判修订、案件来源以及连接器告警管理策略。旧 Task/案件/远端投影没有回写。资产版本、项目/团队、观察—证据—研判的业务 Run 关联由复合外键约束。

验证在自动创建、结束后删除的临时数据库进行，未迁移本地应用数据库：

- `go test ./internal/migrations ./internal/inspection -count=1`，配置真实 PostgreSQL 测试连接：通过，无跳过。
- 覆盖旧版已发布定义保留、重复迁移、同资产被两个 Run 引用、跨项目/跨 Run 绑定拒绝、引用资产版本不可覆盖及 occurrence 唯一约束。
- `pnpm db:generate`、`pnpm db:check`：通过。
- 原 schema 比较在 PostgreSQL 18 中因分区继承 NOT NULL 的自动命名差异失败；比较器现通过已有列 `attnotnull` 比较非空行为，排除重复的 `contype=n` 名称比较。完整 schema 比较重新通过，其他约束仍严格比较。

## 2026-09-11：任务 3.2 任务级并发和重放

修改 scheduler 与 HTTP/sqlc 查询，以 `project+task` 统计活动 Run 和查找来源幂等键；不再因发布新 Task 版本绕过并发或重放。调度发现后在事务内重新锁定 Task/当前版本，并重检启停和版本，阻止过时候选提交。

验证：

- `go test ./internal/tasktrigger -count=1`，真实临时数据库：通过。覆盖同 occurrence 双实例争用、跨版本重放、旧版本活动 Run 限制新版、发布后过时候选、停用后过时候选及不同 occurrence 并发竞争。
- SQL 查询测试同时覆盖 HTTP 使用的 `CountActiveTriggeredRuns` / `ReadTriggeredRun`。
- `go test -tags dev ./internal/httpapi -run 'Test(ProjectAssetListContract|UserTask|TaskDraftMainRoundTrip)' -count=1`，真实临时数据库：通过。
- 曾扩大到完整 HTTP 包：唯一失败为已有 `TestProjectAssetListContract`，其 timestamptz 夹具写入 timestamp 列导致当前服务器时区下偏移 8 小时。已将夹具改为显式 timestamp，失败用例及 Task 相关用例重跑通过；修复后未再重复整个 HTTP 包。
- `git diff --check`、OpenSpec strict 校验、`pnpm db:check` 通过。尚未运行完整 `pnpm check` / `pnpm build`，本轮不作为整体交付验收。

当前完成 1.2、1.3、3.2，共 3/32。Task YAML/表单、观察 handler、真实检测、无案件 AI 研判与定时远端飞行仍未交付。1.1 的真实接入依赖仍待配置；现有改动未启用新飞行入口，也未把业务 Run 标为已成功。


## 2026-09-11：Task 作者入口与调度续作

已完成任务 2.2：Task 本体与 disabled 首稿在同一事务内创建；相同幂等键与请求重试复用结果、变更请求冲突；v2 保存/发布要求 expectedRevision；发布快照不可修改，复制草稿保留 YAML 原文并从新 revision 开始。发布和启用绑定明确用户。数据库 HTTP 测试通过，包括撤销团队成员资格后禁止启用、停用不取消已运行 Run。

作者界面已接 YAML/JSON 原始编辑、模板基础表单、两种巡检模板、新建和启停入口。浏览器在独立临时数据库中验证原始文本载入、表单改名称和 Cron、格式往返、嵌套自定义字段保留、非法文本保留、正式 API 保存重开、从模板表单创建巡检草稿。最新浏览器证据：`.build/aerosight_test_author_0d6b3d82ce2bf73b/result.json` 与 `authoring.png`。未调用外部模型或飞行。

浏览器测试发现旧 `task_steps_uses_valid` 不接受 inspection 步骤，已用 `0074_inspection_task_steps.sql` 增量放行 observe/detect 作者存储，同步 schema；新增真实数据库创建巡检草稿回归用例。应用数据库未迁移。

v2 作者校验已有：安全 YAML/JSON AST、默认值规范化、能力版本冻结、未知能力/版本、前序依赖与递归引用检查、项目资源检查、Cron/时区、JSON Schema 编译（禁止网络或文件 schema 加载）。当前仅 report.generate 通过 v2 发布能力门控，其余能力可保存草稿，待各 handler 验收后逐项开放。任务 2.1、2.3、2.4 尚未全量勾选：能力输入输出契约与完整研判参数、真实巡检试跑仍需后续集成。

调度接入共享 JSON Schema 校验库，按 schema 顶层默认值 → trigger.inputs → invocation.inputs 合并并递归校验、拒绝类型强制转换。v2 schedule 支持手动试跑且不改变计划水位。所有 schedule（包括历史版本）需明确委托者，原有无委托计划需要重新启用，不回退项目创建者；其他 v1 用户触发保持旧契约。调度事务重检并锁定委托权限，Run 记录执行用户；候选异常独立记 error，重启错过的区间记 missed、不批量补跑，接受/并发跳过按 occurrence 固化。

针对性真实数据库测试已通过：输入优先级、嵌套对象/数组类型、未知字段、无委托者、撤权、坏 Cron 隔离、重复扫描、并发跳过不重试、两小时重启窗口，以及恰好 60 秒可执行 / 超出 60 秒不补跑。HTTP 测试验证 schedule 的手动运行与重放、错误类型拒绝及水位不变。旧跨版本并发测试继续验证。

新增 JSON Schema 依赖的 `go mod tidy` 首次因默认 Go 代理下载测试依赖超时；切换公开 Go 镜像后成功，未修改全局 Go 配置。


本轮任务 3.1 已完成，当前累计 1.2、1.3、2.2、3.1、3.2，共 **5/32**。后续重点为能力契约和正式 runtime 接入（3.3、4–6），不能将模板可编辑等同于巡检闭环已执行。

本轮最终检查：

- `pnpm check` 通过（日志 `.build/inspection-check.log`）。配置 `AEROSIGHT_MIGRATION_TEST_DATABASE_URL`，使用 testdb 的 HTTP、迁移、调度集成用例实际执行；单独要求 `AEROSIGHT_TEST_DATABASE_URL` 的其他历史集成用例未配置，仍按原有条件跳过，不能把这些跳过项算作集成通过。
- `pnpm build` 通过；最后调度修改与 Go 依赖整理后补做 `pnpm build:server`。
- 浏览器验收通过，使用系统 Chrome；创建临时数据库和独立开发模式服务，结束后清理；未写现有项目或调用飞行动作。
- `go mod tidy` 通过；`git diff --check` 和 OpenSpec strict 校验通过。


## 2026-09-11：任务 4.2 告警接管和旁路隔离

实现 `inspection/alert_policy.go`、`flighthub/inspection_alert_evidence.go`、正式告警策略 HTTP API 和连接器管理页入口。`0075_inspection_alert_evidence.sql` 记录告警—架次的私有来源关系及结构化原始证据；公开摘要不暴露远端标识或临时 URL。目标 `target_value` 原值保留为 targetValue，不作为 Task 证据置信度；模型版本和位置来源未知时显式登记。

- 策略变更与 `ApplyFlightAlerts` 共用连接器行锁；开启后未知架次登记 pending，只保存证据，不创建感知事件或案件。
- 已接管或 pending 架次在关闭连接器默认策略后仍受保护；只有明确的 confirm-legacy 操作释放 pending 架次，task 架次不能通过该入口释放。
- 新建、更新/重开、missing-alert 三条路径均检查归属。上游缺失只改变受保护资源的可用状态，不修改案件和感知事件。
- 既有案件及其原 Run 关联保持原状；不会因为选择/接管架次而自动转为业务 Run 案件。重复分析保留最初的归属标记，不覆盖远端投影。
- 私有来源映射禁止同一告警在不同架次间重新绑定；项目与连接器复合外键阻止误绑。
- HTTP 接口提供策略读取、幂等开启/关闭和按本地证据 ID 确认 legacy；撤权、跨项目和变更请求复用幂等键被拒绝。前端展示暂存/接管、来源缺失及关联运行，列表超出 100 条明确提示。

验证：

- `TestTaskManagedAlertNewUpdateMissingAndLegacyRelease`，真实临时数据库：早到与重复不建案、旧闭案不重开、missing 不关闭受保护案件、关闭默认开关不释放架次、明确释放后只恢复该架次 legacy、新旧来源不串绑、旧案不隐式关联业务 Run，均通过。
- 原 `TestSQLFlightAlertProjectorKeepsLifecycleLinksAndSecretsIdempotent` 在真实临时数据库通过；为原测试增加 testdb 回退，避免配置了迁移测试库却将此关键回归跳过。
- `TestInspectionAlertPolicyHTTPAuthorizationAndReplay`：读取/切换/释放、幂等冲突、跨项目、撤权和远端身份不泄露通过。
- 全部迁移测试通过（空库、升级、快照、重复等）；修正升级测试的旧库选择为 0073 之前，避免后续新增迁移误被算入旧版本。
- `pnpm build`、`pnpm db:check`、`git diff --check`、OpenSpec strict 校验通过。
- 浏览器测试通过：`.build/aerosight_test_author_da67b96f0962a7ae/result.json`、`alert-policy.png`。从正式连接器页面开启、刷新重开、确认释放、关闭均验证。连接器为临时数据库内的协议夹具，没有调用真实司空或飞行动作。

本轮新增完成 4.2，累计 **6/32**。本次未重复整个 `pnpm check`；上一轮全量检查与本轮相关检查分别记录。观察 handler、正式识别、无案件 AI、复核与业务写案尚需接入，Task v2 发布门控仍未放开这些未部署能力。

## 2026-09-11：运行边界、图片观察及原生告警识别

本轮完成任务 **4.3**，累计 **7/32**；3.3 和 4.1 仍为部分实现，未勾选。新增原生告警 detect handler 已注册后台事件，并开放该 source 的发布能力；existing-flight observe 和 external detect 尚未开放，完整模板仍不能作为闭环通过。

- v2 同步步骤执行前锁定业务 Run、重检执行委托者权限、前序依赖/条件、输入引用及 schema；false 条件跳过且不执行副作用。同步返回后验证输出，错误输出通过 savepoint 回滚业务写入。取消/暂停、已完成重放不再执行，迟到非控制事件不能恢复业务 Run。异步完成、复核恢复及远端飞行边界仍待实现（3.3）。
- 修复运行派发使用 run-step ID 查询 task-step 重试配置的问题，改为显式关联；真实后台 report-only Run 已成功。
- assets observe 实际读取图片内容、验证已有 checksum、冻结 SHA256/资产版本/原始 Run 来源；同资产跨 Run 分析不改归属。不可读、重复、跨项目、超限和 checksum 不一致均拒绝。当前只有本地可读资产，司空媒体刷新和已有飞行对账仍待实现（4.1）。
- 原生 detect 从当前业务 Run 的封闭 observation 读取确切 connector+flight 告警，锁定连接器并接管该架次；最多 1000 条，超限明确失败，无静默截断。保存独立 evidenceSet 快照，后续告警修改不会改写历史。保留算法类别编号、targets 原始 targetValue、模型 unknown、提供方位置原文；候选位置始终 unknown/unverified，不认定为目标坐标。公开快照不复制存储中的临时 URL/远端 ID 字段。
- 原生告警范围不能证明全区域分析完成：有告警为 alert-only，零告警且目标算法未知为 unavailable，来源失效为 partial。targetAlgorithmConfirmed=false，均不能给出 no_issue。提供 observation/evidence-set 的项目授权 GET 接口；未封闭 observation 不可读取，跨项目或撤权拒绝。

验证：

- 真实临时数据库 `TestNativeDetectFreezesScopeAndDoesNotInferNoIssue`：原生/零告警/来源 missing/其他架次排除/错误 Run/取消/非法输出原子回滚/1001 条上限/重放/快照冻结均通过。使用协议数据夹具，不是账号实测告警或真实算法调用。
- 真实临时数据库 `TestV2StepBoundaryProtectsInputsDependenciesOutputsAndCancellation`、`TestObserveAssetsSealsReadableVersionsWithoutChangingProvenance`、作者 HTTP 回归均通过。
- `TestInspectionSnapshotsAreProjectScopedAndSealed` 通过：未封闭、跨项目、非法 ID、成员撤权均覆盖。
- `pnpm build`（图片路径后、native detect 前）通过。浏览器验收 `.build/aerosight_test_author_64377018e64a9ed2/result.json`：正式 API 创建/发布/启用/触发、真实后台图片 observe→report、PNG 内容 checksum/原始 Run、重复触发均通过。图片为明确标记的测试夹具；不作为真实航拍或识别验收。

未执行真实飞行；未修改应用数据库或已有密钥。真实识别/AI Provider 和现场授权条件仍待配置，但不阻止其余软件实现。

本轮最终回归：新增 native detect 和读取接口后，`pnpm check` 全部通过（`.build/inspection-check-latest.log`），`pnpm build` 通过（`.build/inspection-build-latest.log`）；全量检查配置临时数据库集成入口，仍未配置独立历史 `AEROSIGHT_TEST_DATABASE_URL` 的条件跳过项不计集成通过。最终浏览器复跑通过：`.build/aerosight_test_author_bb029240241d8042/result.json`。OpenSpec strict 校验和 `git diff --check` 通过。工作区保留未提交改动，变更/目标仍在进行中。

## 2026-09-11：已有飞行媒体读取适配续作（4.1 部分）

上轮属于有效进展：原生 detect 已实现并验证，本轮继续已有飞行媒体读取；总体仍为 **7/32**，未缩减任务范围。

- 新增 `flighthub/inspection_observe.go`，在项目/连接器/架次核验后读取真实 GetFlightTask 和媒体列表，对照现有投影资产、原 Run 与 object_version；不改写投影 Run 或原资产归属。读取能力已通过可注入 reader 接入后台 observe，assets 路径继续复用同一封闭事务。
- `FlightTaskFolderInfo` 区分未提供/NULL 计数与明确的 0，拒绝负数和错误类型；完整性比较使用全部文件数量，不用已选图片数量冒充远端总数。范围计数、数据缺口和 remote objectVersion 保存到观察快照。
- 复用授权链接刷新，新增 encrypted locator 的 flight UUID 校验；错误架次、记录文件冒充图片在请求远端前拒绝。新增流式 SHA256 读取，最大 64 MiB，复用允许域名、超时和禁止重定向策略，不向媒体主机附带 API token。空文件、过期链接、超限、读取失败有明确错误；内容读取发现过期时再刷新一次。
- full-flight 清单不完整时失败；仅显式 assetIds + confirmLimitedScope + 范围说明允许 partial，确认用户取自 PreparedStep。不可读/不属架次的显式选定资产不能通过确认绕过。上游列表截断按客户端明确失败，不把截断结果作为完整数据。

验证：临时数据库中的既有投影/签名刷新测试增加 observer 对账、未知计数、有限范围确认、错误选择；与真实 TLS 下载器的大小/重定向/过期/空内容及哈希测试配合通过。观察 handler 经真正步骤边界验证 existing-flight 回调清单落库与重放，assets 回归通过。`go test -tags dev ./internal/inspection ./internal/runtime ./internal/flighthub -count=1` 通过；沿用条件跳过的历史独立数据库测试不计真实集成通过。最后定向复跑亦通过；`pnpm build:server`、OpenSpec strict、`git diff --check` 通过。本轮未重复全量前端检查。

尚未开放 existing-flight 作者发布门控：还需要完整输入 schema、正式 API/runtime 的远端协议替身串联及更多失败/版本变化/撤权矩阵。上述测试是标明的协议夹具，未访问真实设备、未调用实飞动作，不是账号或现场验收。4.1、3.3 和整体目标继续保持未完成。

## 2026-09-11：已有飞行输入契约与签名刷新版本竞态

本轮补齐 existing-flight 的作者 inputSchema：connectorId/flightUuid 必填；assetIds 必须为唯一正整数集合；maxImages 限制为 1–1000；confirmLimitedScope=true 时同时要求非空图片清单及非空白范围说明。增加 schema 测试覆盖正常架次、有效有限范围、缺清单/说明、空白说明、重复 ID、错误类型和超限。

修复观察读取中的版本竞态：此前只比较本地投影与首次媒体列表，签名刷新可能取得更新后的文件。现在刷新授权链接时再次获取实际媒体列表，比较冻结的 objectVersion；不同版本返回 media_version_changed，不读取并绑定新内容到旧版本。保留原有普通资产下载行为，只在巡检专用路径要求版本可验证。数据库测试覆盖错误版本拒绝及相同版本刷新成功，已有完整/partial 对账测试回归通过。

`go test -tags dev ./internal/flighthub ./internal/httpapi -run 'Test(SQLFlightAssets|InspectionMedia|TaskV2|TaskAuthoring)' -count=1`（临时数据库）通过；新增 `TestTaskV2ExistingFlightInputContract` 通过；`pnpm build:server` 和 `git diff --check` 通过。existing-flight 发布门控继续保持关闭，正式 API + 真实 outbox consumer + 远端协议替身串联验证尚未完成。总进度仍为 7/32，目标继续进行。

## 2026-09-11：已有飞行正式 API 与后台消费者串联

新增 `httpapi/inspection_workflow_runtime_test.go`，使用正式 Task 创建/发布/启用/触发 API、真实 PostgreSQL outbox Consumer、生产 mission/observe/detect/report handlers；仅远端 HTTP 响应和 token resolver 为明确标记的协议替身。源飞行和媒体目录是夹具，不手工写入业务 Run 的步骤成功状态。

- 完整媒体：远端 GET → 刷新链接/实际读取夹具内容 → 冻结观察 → 原生 detect → 报告，由消费者推进至 succeeded。
- 缺失计数：观察步骤以 INSPECTION_MEDIA_INCOMPLETE_CONFIRM_FINITE_SCOPE 失败，不产生封闭观察。
- 显式有限范围：资产清单和范围说明确认后形成 partial 快照，继续后续步骤。
- 刷新期间媒体版本变化：以 INSPECTION_MEDIA_VERSION_CHANGED 失败，不绑定变更内容。
- 触发后执行前撤权：后台在接触 Provider 前拒绝，撤权后的触发重放 API 也拒绝。
- 重放复用原 Run，不重复读取媒体；原资产的 source Run 保持原值；SHA256 对应实际经下载器读取的夹具内容。
- 原生零告警的 evidenceSet 始终 unavailable、不能结论 no_issue，即使报告生成成功也不宣称已完整识别。
- 发布时拒绝不存在于该项目/连接器的架次；发布核验仅查询本地资源，无远端动作。

串联测试发现空连接器 credential envelope 的扫描错误，已让观察读取将 NULL 交给现有凭据解析路径按空对象处理；没有添加默认凭据或授权回退。纠正测试夹具的项目 UUID 格式后，全部路径通过。

通过上述验证后，正式开放 existing-flight 的 v2 发布门控。相关作者 HTTP 回归、输入 schema 测试与完整串联用例通过；`pnpm build:server`、`git diff --check` 通过。本轮未重复完整前端检查。

整体仍未完成：4.1 还需统一 assets 模式对司空远端资产的按需读取和版本校验，避免用户直接选择远端 assetId 时只尝试本地文件；3.3 的异步/复核边界及 external/AI/案件步骤仍待完成。当前勾选仍为 7/32，未将这条无真实模型的协议测试链称为完整巡检或现场验收。

## 2026-09-11：4.1 观察输入完成

补齐 assets 模式对司空远端图片的读取：根据服务器加载的资产版本/原 Run 及访问引用定位来源架次，复用加密 locator 检查、签名刷新、远端版本核对、有限大小流式哈希。远端读取失败不能回退到本地文件或伪造可用。保留每张资产的原 FlightRef、sourceRunId、objectVersion、checksum；assets 模式的范围仍是选定集合，不冒充全架次。

为避免与投影写入产生相反锁顺序，assets 观察先按连接器 ID 顺序获取连接器锁，再锁资产，随后读取封闭内容。多 Run 使用同一资产不改变原归属。已有飞行清单对相同版本的重复媒体去重；重复身份但版本冲突拒绝。10,000 条触顶的客户端目录返回明确错误且不返回可用部分列表。

任务 4.1 的证据由本地图片观察测试、实际投影/加密引用/签名刷新测试、完整性与下载器测试，以及正式 API + 真实 outbox Consumer 串联测试共同覆盖：

- 本地/远端 assets 和 existing-flight 成功读取、冻结 checksum 与版本；跨项目/错误 flight、同资产跨 Run/重放、不可读、checksum 不符、超限、取消均覆盖。
- 远端预期/上传/实际列表数量对账、计数缺失不当零、缺图/选定范围为 partial 或失败；明确用户确认有限清单后继续。
- 正式运行覆盖重复媒体、未知计数失败、有限范围成功、已有飞行与 assets 两种方式的版本漂移失败、执行前撤权，以及原生零告警仍不可结论 no_issue。
- `go test -tags dev ./internal/httpapi ./internal/flighthub ./internal/inspection -run 'Test(InspectionExistingFlightFormalAPIAndConsumer|InspectionMedia|ObserveAssets|SQLFlightAssets)' -count=1` 通过，使用可自动清理的临时数据库及明确标记的协议夹具。

现已勾选 4.1，总计 **8/32**。真实账号/图片授权/识别服务与现场飞行验收仍属于未完成任务，不由上述协议测试替代。当前完整 `pnpm check` 已启动，稍后追加最终结果；工作目标保持进行中。

4.1 最终回归：`pnpm check` 通过（`.build/inspection-check-latest.log`，临时数据库集成入口已配置；历史依赖独立测试数据库的条件跳过项仍不计集成验收）；`pnpm build:server` 通过。生产后台运行方式的浏览器回归也通过：`.build/aerosight_test_author_7bdee35ec9e2a11e/result.json`。OpenSpec strict 和 `git diff --check` 通过。下一项为 4.4：现有单图 algorithm trigger 强制 asset.task_run_id 等于分析 Run，且单个 algorithm completion 会结束整个步骤，需要复用调用服务并增加逐图片子项关联及聚合完成，保持原资产归属。

## 2026-09-11：4.4 外部识别子任务基础（部分）

新增 `algorithm.QueueInspectionAsset`：通过当前 v2 Run 的封闭 observation/asset 绑定授权输入，检查资产版本、objectVersion、原来源 Run 和冻结 checksum；复用原算法定义/Provider、Input 协议、algorithm_runs、幂等键和请求 outbox，无需修改 asset.task_run_id。每张图片有独立算法运行，重复入队复用原记录，并保留上层步骤的 Prepared 输入快照。

算法完成路径识别 inspection.detect 父步骤，改为发出幂等 `inspection.algorithm.completed` 事件；单张成功或失败均不直接完成整个父步骤。原 algorithm.run 单图步骤继续原有完成方式。新增临时数据库测试覆盖两张图片、重复入队/完成、来源不改写、错误观察/版本变化拒绝及旧完成行为回归。

`go test -tags dev ./internal/algorithm -count=1` 通过；最后补充仅 v2 入队约束后定向测试再次通过。`pnpm build:server` 在最后的 v2 SQL 约束补充前通过；`git diff --check` 通过。未调用真实 Provider，未开放 external 发布门控。

4.4 尚未完成：还需 detect external 的清单调度、全部子项结果聚合、算法派发前委托/取消边界、冻结媒体访问、真实 Provider 和实际图片验证。不能把已入队或单图成功当整个识别完成。进度保持 8/32，目标继续进行。

## 2026-09-11：4.4 外部识别调度与汇总（部分）

`DetectProcessor` 已加入 external 分支，按当前 Run 的不可变 observation 清单和配置上限创建逐图算法任务。首批创建在步骤事务中完成；后续完成通知重新查询整批真实状态，验证子任务数量、资产身份及定义版本，不依赖通知载荷声称的成功结果。任一子项仍在运行则等待；全部结束后若有失败则步骤失败，不能转零检测。仅全部合法 canonical detection 结果成功时生成独立 evidenceSet，保留每张输入版本、算法运行、定义版本、模型 revision/digest 和规范化结果。未公开的模型版本仍写 unknown。

运行时已注册 `inspection.algorithm.completed` 到同一受步骤边界保护的检测处理器。原生和外部来源共用证据集原子封闭逻辑。明确有限范围的 partial observation 仍保留原始部分范围，外部结果只对该封闭集合声明完整。

新增 `TestExternalDetectWaitsForWholeFrozenImageSet`（临时真实数据库、明确标记的子结果夹具）：双图非零/零结果、首图不推进、单图失败的错误码、非法结果的错误码、取消迟到不写证据、超限不入队、重复通知只封闭一次均通过。`go test -tags dev ./internal/inspection ./internal/algorithm ./internal/runtime -count=1` 通过，`pnpm build:server` 通过；本轮未运行全量前端检查，也未调用真实 Provider。

4.4 保持未完成、external 发布门控仍关闭：还缺 Provider 派发前的委托/取消处理、冻结图片内容的实际访问、正式 API + 真实 Provider 调用与失败矩阵。当前 8/32，目标继续进行。

## 2026-09-11：外部算法派发与暂停恢复边界（3.3/4.4 部分）

算法初次派发现在先按业务 Run→算法子 Run 的顺序取得锁，重检明确委托者及 v2 权限；取消/非活动父运行的排队子项转 canceled，撤权子项失败并通知汇总，均不进入 Provider 请求。暂停的子项保持 queued；任务恢复时按 run state_version 发出幂等唤醒，并通知汇总已完成结果。已结束的算法子项不会在重放时重新调用。

同时修复两个控制问题：纯业务步骤的取消直接取消业务 Run，不向无设备的图片流程发送 device.stop；其状态原因明确为 operator_canceled_business_run，不宣称物理设备已经停止。resume 现在发出运行转换事件以唤醒执行器，避免只把 paused 改为 running 却没有后续事件。

真实临时数据库测试覆盖委托有效、暂停保留、恢复重复唤醒不重复、撤权失败、取消禁止派发及旧单图路径；mission 数据库测试验证 resume 的持久唤醒事件，状态机测试验证无设备取消和旧设备控制路径。测试中修复了恢复事件 state_version 参数的 PostgreSQL text/int 编码问题。`go test -tags dev ./internal/mission ./internal/algorithm ./internal/runtime -count=1`、`pnpm build:server`、`git diff --check` 通过。

尚未完成真实 Provider 端到端访问：下一步为冻结图片访问与正式 API/Provider 协议测试，再进行已授权真实 Provider 验收。外部识别发布门控仍关闭，3.3 的复核/其他异步边界尚未全部完成，总计保持 8/32。

## 2026-09-11：冻结图片网关与 HTTP Provider 派发验证（4.4 部分）

外部识别输入现使用绑定 SHA256 的短期签名 URL，签名域与旧 URL 分离。初次入队及派发刷新均保留冻结摘要；缺少 pinned issuer 时明确失败，不降级。取图入口在返回图片前核对实际字节，变化返回 409，删除/修改 checksum 或混用旧签名返回 403。旧签名的本地资产访问兼容。

司空媒体增加与流式哈希共用校验逻辑的有限字节读取（最多 64 MiB），沿用 HTTPS 域名白名单、过期检查、禁止重定向及不转发 API 凭据。FlightAssetAccessService 的算法取图解析器验证项目/团队/资产版本、活动连接器、原飞行和媒体目录关联，再核对加密 locator 和远端 objectVersion；远端失败不得回退本地存储。运行时已接入网关，Provider 始终经平台核对内容，无法绕过网关直接使用远端临时 URL。取图回调不锁业务 Run，避免同步 Provider 回读图片与派发事务相互等待。

真实临时数据库及实际 HTTP/TLS 测试覆盖：本地/远端成功返回原始字节、同版本内容替换拒绝、签名篡改/降级拒绝、跨项目/版本拒绝、停用连接器不取图、飞行记录不能当作图片；远端下载的空响应、长度超限、流式超限、过期和重定向也同时覆盖字节读取路径。

新增 HTTP Provider 派发场景：真实 Processor 持有业务事务时，由 TLS 协议替身主动下载签名图片，返回零检测或非零检测，保存规范化结果及模型标识并发出汇总事件；图片内容不匹配导致子项失败，重复派发不重复请求，单个完成不结束检测父步骤。这些使用明确标记的图片/Provider 协议夹具和测试 raw store，不是授权真实图片、真实模型或全链正式 API 验收；与完整批次聚合的联合 E2E 仍待补齐。

算法、司空、inspection、runtime 包的临时数据库检查通过；server 构建通过。本轮未运行全量前端检查。external 发布门控仍关闭，4.4 及总计 8/32 状态不变；后续继续完成正式 API→Provider→整批证据联合验证及真实授权 Provider 验收。

## 2026-09-11：external 正式 API 与完整识别批次联合验证（4.4 部分）

新增 external 作者输入契约：observationId、algorithmDefinitionVersionId、maxImages（默认 64、上限 1000）、parameters。即使 observationId 是前序步骤输出引用，也单独校验静态参数与必填项，避免未解析引用掩盖缺失定义或非法上限。发布校验及运行时入队均要求同项目已发布的 detection 定义和活动 http-json Provider；其余协议尚不可执行。未改变旧 algorithm.run 的发布资源查询。

已通过正式 Task 创建、发布、启用、手动运行 API，加真实 outbox Consumer/mission dispatcher/观察及检测处理器、HTTP/TLS Provider 协议替身、签名图片 HTTP 服务，验证双图非零检测、双图零检测、单图失败、配置超限与触发重放。成功时整批才生成一个证据集，失败不生成证据，超限无 Provider 调用，重放不重复请求，原始 asset.task_run_id 不变。同时把 existing-flight 正式 API 场景接到真实加密引用/司空媒体刷新/平台取图网关/HTTP Provider/证据集，验证远端图片路径也能联合执行。这些图片字节和模型响应仍为明确标记的协议夹具，不算真实识别精度或现场验收。

联合测试发现算法完成事件使用 outbox 默认重试次数，未继承步骤发布策略，导致单图失败后整批滞留重试。已修复完成事件及 resume 汇总事件从任务步骤读取 maxAttempts；失败现在按任务配置结束，不靠测试强改 outbox 时间或次数推进。

external 软件执行路径已开放发布，真实 Provider/授权样本验收未完成，因此 4.4 保持未勾选，总进度 8/32。`go test -tags dev ./internal/httpapi ./internal/algorithm ./internal/inspection ./internal/runtime -count=1` 通过（临时数据库，httpapi 全包约 134 秒）；`pnpm build:server`、`git diff --check` 通过。本轮未重复全量前端检查。下一阶段为 copilot.run 无 issue 的 assessment 执行、结构化研判及整批复核。

## 2026-09-11：无案件研判作业与模型协议基础（5.1/5.2 部分）

新增 assessment 入队函数：从同项目、同团队、同业务 Run 的 evidenceSet 与 observation 读取并校验不可变快照，复用 agent_sessions/agent_tool_jobs 创建无 issue 的研判作业，同时保存 inspection_assessments 的证据摘要和提示模板版本。相同步骤重放不会新增 assessment/session/job；错误证据作用域拒绝。函数预期在既有 mission Run 锁及明确委托者授权边界中调用；当前尚未注册到 TaskStepHandler，避免把尚未接好消费与恢复逻辑的能力开放给用户。

新增模型研判适配函数，复用既有加密 AI Provider 配置与 OpenAI completion HTTP 调用；系统策略和证据 JSON 分别置于 system/user 消息，未提供任何写入或设备工具。严格解析 decisions，拒绝未知字段、尾随 JSON、捏造引用和不具备完整检测条件的 no_issue；保留模型原文及 Provider/模型标识供后续持久化，即使结构校验失败也返回原文。提示中明确有限分析范围、缺资料复核、位置语义与单期建筑证据边界。旧案件 Copilot 的 complete 接口继续原有行为，另修复直接调用时 HTTPClient 为空导致崩溃的问题。

`TestInspectionAssessmentQueueAndModelWithoutIssue` 使用临时数据库、真实加密凭据读取和 HTTP 模型协议夹具，验证空案件库、唯一作业/会话、错误证据拒绝、成功零检测研判、partial 禁止 no_issue、非法结构、捏造引用和 needs_review；`go test -tags dev ./internal/agent -count=1`、`git diff --check` 通过。测试中沿用项目自动创建的 Copilot，而不另建重复 Copilot。

这些尚不是可执行的完整 assessment：下一步必须接 JobProcessor 消费、派发前/完成时 Run 与委托重检、模型原文和 revision 原子保存、暂停与恢复、失败/取消边界，再接作者契约和发布。模型 allowedIssues 目前由服务器调用者提供，仍需实现授权候选案件上下文；真实 Provider 验收未完成。5.1/5.2 均未勾选，总计保持 8/32。

## 2026-09-11：assessment 作业消费与原子结果保存（5.1/5.2/5.3 部分）

新增 ProcessAssessmentNext 并接入既有 JobProcessor 循环；旧 issue_copilot 查询排除 assessment 作业，保持旧执行路径。TaskStepHandler 现可在 mission 已准备上下文中派发 assessment，但作者发布门控仍未开放。消费时按 Run→作业→assessment 加锁，重检明确委托者、作业会话授权、当前 Run/步骤状态及证据摘要；暂停保留 queued，非活动 Run 将待执行 assessment 标记 canceled，不调用模型。

使用限时只读模型调用（45 秒，外层事务 60 秒）并在同一事务保存 Provider/模型、原文、首个 model revision、作业结果及步骤输出；错误原文也保留。成功才发出后续步骤事件，needs_review 暂停 Run 和步骤，失败按 abort/pause 策略处理。锁与事务覆盖只读模型请求，崩溃不会留下已提交的 running 作业，已提交结果不会重问模型；取消与该事务串行化，不存在提交后再接受迟到回调的路径。若事务在模型返回后提交前中断，重试可能重新发起只读模型请求，此处没有模型写入工具。

普通 resume 的 runtime 及正式控制 API 都检查是否存在 needs_review，返回 INSPECTION_REVIEW_REQUIRED。另修复 loadSnapshot 读取无设备业务步骤的 nullable capability_code 时扫描失败，空 capability 保留为非设备业务输入。

真实临时数据库测试已调用 ProcessNext，验证唯一成功 revision、原文持久化、重复消费无新工作、暂停保留/恢复消费、取消及撤权无模型请求、非法 JSON 原文保留、needs_review 暂停及 runtime resume 拒绝；旧 agent 与 mission 全包测试通过。最后一次定向检查覆盖 agent/HTTP mission control 并通过；同次 runtime/mission 正则无匹配，未计该次测试覆盖（mission 前述全包已经通过）。API 复核场景联合验证、复核提交 API 与授权候选案件上下文尚待完成，模型/授权图片实测仍缺条件。总计保持 8/32，未勾选第 5 组。

## 2026-09-11：人工复核读取/提交 API 与原子恢复（5.3 部分）

新增项目内 assessment GET，返回原始模型输出、Provider/模型/提示/证据版本、当前状态和全部修订；新增 POST review，以 issue:handle 授权和 AuditedWrite 写入。提交必须包含 expectedRevision、idempotencyKey、完整决策。锁顺序与 worker 保持 Run→assessment；取消、非待复核状态、作业上下文过期、revision 冲突均拒绝。相同请求返回既有 revision，不重复恢复；同键改变决策/操作者/原 revision 返回冲突。原始模型输出保持不变。

人工决策增加仅人工可用的 reject 表达驳回线索，明确不等同于 no_issue；模型的原 Validate 仍拒绝 reject，人工 ValidateReviewed 保留证据/候选/案件范围校验，no_issue 仍要求完整目标检测。人工 update 必须是本项目非 closed 案件，作为显式人工确认对象。需要继续复核的 decisions 不能结束复核。人工修订、assessment 完成、对应 paused 步骤完成、Run 恢复与唯一 continuation 在同一事务提交，零行步骤更新会失败回滚。

`TestInspectionAssessmentQueueAndModelWithoutIssue` 增补原文保留、人工驳回、模型禁止驳回、过期/取消/旧 revision 拒绝、同键不同请求拒绝、唯一恢复事件。`TestInspectionReviewAPIIsScopedAuditedAndIdempotent` 使用真实临时数据库和正式 HTTP API（待复核模型数据为显式夹具），验证 GET 原文、跨项目 404、普通 resume 409、人工提交/重放、修订唯一、审计和取消后拒绝。相关 agent/inspection 检查通过；最终定向 httpapi/agent 复核检查通过，server 构建通过。

尚未完成作者 assessment 契约及正式 API→模型→复核→案件联合链、候选案件模型上下文和复核页面。5.3 暂不勾选，总计保持 8/32；真实模型和现场验收仍单列。

## 2026-09-11：assessment 作者契约、正式联合链与关联案件上下文（第 5 组部分）

新增 copilot.run assessment 输入契约（mode/evidenceSetId），发布时检查项目活动 Copilot 和默认启用的 OpenAI Provider。研判软件路径现可发布，不代表真实模型验收已通过。沿用已有输出 assessmentId/sessionId/jobId；不开放旧 issue 模式的 v2 发布。

扩展正式 Task API + 双图识别 + outbox/runtime + 模型 HTTP + 复核 API 联合测试：零检测后模型 no_issue 到报告；模型 needs_review 后经正式人工驳回和重复复核到报告；非法模型 JSON 导致失败但保留已成功检测证据。每条只调用一次模型，复核不重新识别。图片、识别和模型响应仍是明确标记的协议夹具，不是业务精度样本或实机验收。

新增 CandidateSourceKeys，为后续案件写入和当前关联案件读取共用：原生告警采用项目/连接器/原告警身份，外部检测采用项目/资产版本/检测键/类别，不使用业务 Run 或算法子 Run。LinkedIssues 只读取 inspection_issue_sources 已存在的明确关联，提供标题、状态及对应 candidateId；closed 案件不可自动 update。worker 将关联上下文保存到作业 args_json，并把可更新范围传给模型；模型 update 还必须匹配该 candidateId 的关联，不能在同项目两个候选间串案。人工明确确认的候选更新仍走复核校验。

新增测试验证算法子 Run 变化不改变来源键、明确关联可更新、跨 candidate 关联拒绝、closed 只作为复核上下文。相关 agent/inspection/httpapi 定向测试通过；最终 agent/httpapi 联合测试再次通过，git diff --check 通过。尚未开始正式案件写入的第 6 组；第 5 组仍缺全部语义/实际模型验收证据，总计保持 8/32。

## 2026-09-11：正式 assessment 案件写入与整批回滚（6.1/6.2 部分）

issue.create-or-update 新增 assessmentId 入口，读取同项目/团队/Run 的正式当前修订并重验决策，模型和人工修订分别使用对应 validator。待复核、非法结构和越界 assessment 均不写案件；no_issue/reject 返回空 issueIds。作者契约及该软件入口已开放，旧 Task issue 路径保持原样。

案件按 CandidateSourceKeys 防重，跨业务 Run/任务版本/算法子运行的同源只关联既有案件，不重复计数或重开闭案。新来源的 update 需人工确认本项目目标；模糊更新或 closed 目标在任何案件写入前将整批送回复核。项目级来源锁与既有 issue-number 锁共同保护分配。新增案件/新来源更新、来源绑定、证据/原图/算法运行/观察/研判/业务 Run 关联、issue/project 事件、审计哈希与步骤输出均在一个事务提交。

正式 API/runtime 联合测试已覆盖双图检测→模型 create→两个案件→报告；同资产检测再次经不同业务 Task/版本/算法子 Run 执行仍只有两个案件且 occurrence_count 保持 1，预先 closed 的案件不重开。此前 no_issue 和人工 reject 路径也已接上案件步骤并正常到报告。第二个案件插入处注入数据库异常，确认第一个案件、issue_links 和 inspection_issue_sources 一并回滚，检测证据保留，步骤失败；未靠截断批次回避失败。

相关 issue/httpapi 定向测试与最终正式联合测试通过；git diff --check 通过。6.1/6.2 暂不勾选：还需补新来源人工 update、模糊/闭案转回复核及并发竞争的专门验收，6.3 报告证据链接与范围呈现尚未完整。当前总计 8/32，真实模型/现场验收不由协议闭环替代。

## 2026-09-11：案件并发/更新验收完成，报告范围快照补齐（6.1/6.2 完成，6.3 部分）

新增 TestInspectionIssueConcurrencyUpdateAndReview，以临时数据库和相同原始资产/检测来源的两个不同 Task/版本/算法子 Run 同时执行，验证只创建一个案件及一次 issue activity。人工确认新来源更新既有案件只增加一次 occurrence；再次通过另一个 Run 提交同一新来源不重复计数。没有既有明确关联的模型 update、已关闭案件的新来源人工 update 均暂停 Run、将 assessment 转 needs_review，案件计数保持不变。结合上一轮正式 API 双图建案、闭案来源重放、no_issue/reject 空列表及第二项写入故障整批回滚测试，6.1 与 6.2 已达到软件验收要求，现勾选，总计 **10/32**。

报告内容新增 inspection 快照：观察范围/时段/原始 FlightRef、证据集、当前研判修订及决策来源，并保留只读 API 链接。报告案件查询纳入本业务 Run 通过 issue_links 关联的既有案件；资产查询纳入观察清单中的复用原图，采用冻结版本和 checksum 而非覆盖 asset.task_run_id。当前目录显示失效/版本变化时增加 dataGap，partial/unavailable 观察或证据以及未完成研判不会显示为完整报告。明确提示结论只适用于列出图片与时段，既有图像不代表本次现场航拍。

正式 API 联合测试增加报告内容断言：两个复用原图、一个观察/证据集、研判快照、有限范围说明均保留；同源第二次运行的报告仍包含关联的两个原案件。最终 issue/report/httpapi 相关检查通过。6.3 尚未勾选：报告 UI/导出里的链接与范围呈现、失效证据和待复核摘要的独立验收仍待完成。所有夹具仍明确属于软件协议验证，真实模型和现场任务未因此完成。

## 2026-09-11：任务步骤研判入口与人工复核页面（7.1/7.2 部分）

新增 `/projects/inspection/assessment/` 页面：展示状态、模型原文、提示/模型版本、完整修订历史、观察/识别证据入口和整批复核表单。按候选保留服务器证据引用，支持确认建案、明确案件 ID 更新、驳回及完整目标检测范围内 no_issue；理由必填，仍有 needs_review 不允许提交。提交固定 expectedRevision，并在不确定响应重试时复用同请求幂等键；成功刷新到服务器状态。只读账号没有提交入口，非 paused/needs_review 不能提交。

assessment GET 增加 runStatus/canReview，后端授权仍为最终判断。任务运行步骤新增研判页面链接、观察/证据只读入口和案件导航。详情页保留刷新按钮及修订冲突错误，不以本地状态冒充已复核。原图目前仍通过观察快照的引用查看，图片预览及完整报告页面尚待补齐。

`pnpm build` 全流程通过（类型检查、Next 静态构建、sqlc check、server 构建）；正式复核 API 临时数据库测试再次通过。浏览器脚本在隔离数据库/真实服务上继续通过原有作者和观察运行验收，并新增明确标记的 UI 协议夹具路由，验证确认 create、reject、提交后服务器状态刷新及只读权限；这部分路由夹具不代替后端复核事务验收。结果：`.build/aerosight_test_author_03f0fd97d741055e/result.json`；截图 `assessment-review-fixture.png` 已查看，无遮挡或布局异常。

7.1/7.2 保持未勾选：尚需完整原图/识别/报告导航、复核修订冲突浏览器案例及更完整资源就绪状态。总体仍为 10/32。

### 报告阅读、观察原图与识别页面（2026-09-12，软件验收）

新增受项目权限约束的 `GET /api/projects/:id/reports/:reportId`，读取最新报告版本；报告页兼容运行时平铺内容、旧手工 `sections` 内容和缺少可选字段的历史报告，并兼容字符串/结构化资料缺口。保留历史 JSON，不改写旧快照。资产引用兼容数值版本、`version:N` 及旧校验和版本；目录缺失/删除/版本或已知校验和变化显示失效。目录匹配标记 `not-revalidated`，不宣称远端字节已验证。运行时报告新生成的 Run/步骤/资产链接修正为现有页面路径，步骤有对应锚点。报告页可进入观察、识别、研判和案件。

新增观察与识别详情页。观察展示冻结清单、时间范围、来源飞行标识、原图版本和 SHA256；识别展示来源/模型、算法覆盖、候选和位置语义，以及可展开的来源结构。Run、研判及报告页面改为这些页面入口。图片位置不等于目标坐标，零告警不直接等于无异常。

原图使用认证的 observation-scoped 内容接口。服务器从已封存清单选择资产，校验项目、团队、资产版本、objectVersion、来源 Run、可用状态和实际字节 SHA256，缺哈希或不匹配拒绝。远端复用运行时司空网关（刷新签名、远端版本/架次核验、有限下载），不暴露临时 URL；远端读取失败不回退到本地。没有远端 reader 时，远端引用返回不可用。读取上限 64 MiB，仅允许经字节检测的 PNG/JPEG/GIF/WebP，私有 no-store 响应。页面加载失败显示原图不可用/版本变化/无法验证，不替换为当前文件。

隔离数据库测试：报告最新版本、跨项目/无权/不存在拒绝、三种旧版本格式、版本变化和已删除资产；观察原图实际 PNG 内容、认证范围、字节替换、资产版本变化、远端失败禁止本地回退、远端成功及远端字节变化。定向运行 httpapi/agent/issue 的巡检与报告测试通过；report 包该名称过滤器没有匹配用例，不能计为该包新增集成覆盖。`pnpm check` 通过，但其默认 Go 测试中缺少专用数据库变量的历史集成用例可能跳过；本节数据库结论来自显式启用 disposable DB 的定向命令。最新 `pnpm build` 通过（TypeScript、37 个静态页面、sqlc 一致性、server 构建）。

5.3 现标记完成：正式 API/worker 数据库用例已覆盖整批暂停、保留模型原文、人工新修订、同键重放、旧 revision/过期/取消拒绝、普通 resume 阻断，以及复核后继续后续步骤。该项不依赖现场飞行或真实模型效果验收。总进度 11/32。

6.3/7.1/7.2 仍未勾选：待复核时的独立报告摘要、完整资源就绪状态和浏览器真实复核到案件的联动仍需补齐；UI 路由夹具的确认/驳回不能冒充真实建案验收。真实 Provider/授权样本与现场验收仍独立未完成。

最终浏览器结果：`.build/aerosight_test_author_a3cea9652547d428/result.json`。在隔离数据库/真实 API 和运行时下验证 PNG 预览解码、字节变更拒绝、零案件运行时报告；UI 协议夹具验证报告三种历史结构/失效提醒，以及复核修订冲突保留输入，未将冲突当成功。已查看观察预览与历史报告截图，布局正常；预览样本是单像素 PNG，仅验证媒体通路，非航拍样本或模型效果证据。

### 待复核进展摘要与报告项收口（2026-09-12）

新增 `GET /api/projects/:id/task-runs/:runId/inspection-summary` 和 `/projects/inspection/summary/`。使用 repeatable-read 只读事务，在同一事务内检查 project:view 并读取 Run 状态版本、待复核数量、封存观察、识别、当前研判修订与资料缺口。复用报告证据聚合；返回 `final:false`，明确为当前进展摘要。Run 中巡检步骤可进入，摘要可刷新并导航原图、识别及人工复核。没有封存观察/研判时明确提示，不能据此认定没有异常。观察页增加司空来源飞行的投影 Run 导航。

未放宽旧手工报告的终态生成限制，摘要 GET 不新建报告/版本、不完成步骤、不派发事件。正式数据库复核用例验证 pending 数量/决策、资料缺口、报告数量为零、复核后状态更新和 pending 清零、跨项目/缺失 Run/撤权拒绝；既有手工报告终态限制回归通过。external 正式 API+真实 Consumer+HTTP 模型协议夹具的 `assess zero` 明确验证报告保留正式 succeeded/no_issue 研判且案件数组为空；该用例不等同真实模型效果。案件反馈与报告组合测试继续通过。

浏览器结果 `.build/aerosight_test_author_cf1ac5490773a18b/result.json`：真实服务观察 Run→摘要导航及刷新；明确标记 UI 协议夹具的待复核提示、资料缺口、研判链接；此前原图、报告兼容、作者/告警策略/复核表单回归同时通过。已查看 `inspection-summary-fixture.png`，布局正常。摘要代码 `pnpm build` 通过（38 个静态页面及 server/sqlc）；随后来源飞行 Link 的小改动通过 typecheck。

6.3 已完成软件行为与规定案例验证，现勾选，总进度 12/32。7.1/7.2 仍需资源就绪检查和浏览器复核到真实案件联动；7.3 的真实样本/Provider 效果验收及 P1/F 不因此提前完成。

### 作者入口资源目录检查（2026-09-12，7.1 部分）

新建与草稿编辑入口新增巡检资源就绪面板及刷新按钮。`GET /api/projects/:id/inspection/readiness` 在 repeatable-read 只读事务内授权 project:view，返回本项目可用未删除图片数、司空连接器数、已成功投影飞行目录数、active http-json detection 已发布版本数、active Copilot 与启用默认 OpenAI 配置是否存在。只返回计数/布尔与 `verification=catalogue-only`，不返回密钥、Provider URL 或模型凭据，不发远端请求。

页面提供图片/连接器/算法/智能体配置入口，并将空项目缺项说清楚。配置存在不表示密钥可解密、样本已授权、远端可读取或模型/实飞验收通过；具体输入仍由发布和运行时校验。没有把“目录检查通过”包装成全链 ready。

隔离数据库用例 `TestInspectionReadinessIsScopedCatalogueOnly` 通过：空项目、同项目可用图片、pending/删除/非图片排除、其他项目资产隔离、停用 Copilot 与撤权拒绝。`pnpm build` 全流程通过。7.1 仍未勾选，来源标记与完整页面联动验收仍待补齐；总体 12/32。

浏览器结果 `.build/aerosight_test_author_edec40bfe4f5edc2/result.json`：真实空项目资源缺项、刷新、智能体配置链接，以及作者/观察/复核/报告全部已有回归通过。已查看作者页截图，资源面板与编辑器布局正常。仅验证目录检查与 UI，未执行任何外部调用或物理动作。

### 浏览器真实复核到案件与报告（2026-09-12）

新增 `scripts/inspection-review-browser-fixture.mjs`，由已有隔离数据库浏览器脚本调用。通过正式 Task 草稿 API 建立规范化步骤，然后仅在临时数据库封装“已完成观察/识别、待人工复核”的标记协议样本；未宣称该种子来自实际算法或模型调用。人工表单提交、正式复核 API、恢复事件、后台 Consumer、issue.create-or-update、report.generate 及案件/报告页面均使用真实服务，不拦截替换响应。

已通过确认 create 新增 1 个真实案件、reject 不新增案件、两个 Run 均 succeeded 并产生正式报告、模型 needs_review 原文保留和人工 revision=2、报告关联数量正确、报告跳转真实案件详情。对应首次完整结果 `.build/aerosight_test_author_1cef2dc2e71ffa2c/result.json`。随后补充双页面旧 revision 冲突、低质量位置提示和原有协作处置入口的浏览器断言。

联动日志暴露并修复既有 upstream handler 的 JSON 扫描缺陷：`output_snapshot_json` 原先直接 Scan 到 map，在 database/sql 驱动下导致所有 succeeded/failed 上游事件反复重试；现先读取字节再 json.Unmarshal。新增真实临时数据库 `TestUpstreamDatabaseJSONOutputAndReplay` 验证结构化上游输出保持、成功触发下游及同事件重放只有一个 Run，测试通过；server 重建通过。修复后完整浏览器日志不再出现该终态 outbox 错误。日志中的司空假连接器 credential_unavailable 来自告警策略 UI 样本，不是实际接入验收。

最终浏览器结果 `.build/aerosight_test_author_1a7992714979ef4a/result.json` 全部通过，包含两个真实页面同时加载 revision=1、第一页面提交后第二页面旧修订获得正式 API 409 `INSPECTION_REVIEW_REVISION_CONFLICT`、保留输入、案件总数仍仅增加 1，以及位置需核实标签与原有协作处置入口。运行前使用显式 browser context 以支持多页面共享真实登录态。已查看正式 create 报告截图，分析范围、协议来源、人工修订和 1 个案件清楚可见。tasktrigger 整包在临时数据库下回归通过。

7.2 按正式 API/runtime 浏览器证据标记完成，总体 13/32。前置识别/模型阶段依然是明确标记的种子样本，不能代替 7.3 的真实样本与真实 Provider 验收，也未执行飞行。7.1 与其余 P0/P1/F 仍继续保持未完成。

### 手动运行响应丢失与委托者展示（2026-09-12，2.4 部分）

作者入口检查发现手动运行按钮每次点击生成新幂等键；服务器已提交但浏览器响应丢失时，重试可能创建第二个 Run。现按当前发布版本与解析后的 inputs 保留一次请求的 idempotencyKey/occurredAt，网络失败后同页重试复用原请求；成功获得 Run 后才清除，改版本/输入产生新请求。本次不声称跨浏览器刷新持久化重试标识。

浏览器使用真实已发布报告任务：首次请求由 route.fetch 送达实际后端并得到 201，再向浏览器模拟连接失败；再次点击实际发送相同请求，返回 200 与同一个 Run，两个请求体完全相同。结果 `.build/aerosight_test_author_3d27f83c781469e0/result.json` 通过，同时保留已有巡检链路回归。

任务工作台查询增加 authorizedByUserId/authorizedByName，界面显示当前执行委托者及启用会绑定当前账号、定时运行前重检权限的说明。SQL/sqlc 同步且 `pnpm build` 全流程通过。2.4 的完整模板端到端与外部资源验收仍未全部完成，保持未勾选；总进度 13/32。

最终浏览器 `.build/aerosight_test_author_9496c3993474eaff/result.json` 通过：真实启用任务显示有 ID 的委托者，响应丢失重试、真实复核到案件/报告及已有作者/资源/证据回归同时通过。未修改应用数据库或调用真实 Provider/飞行动作。

### 巡检部署操作与中文演示手册（2026-09-12，7.4 部分）

新增 `docs/operations/inspection-workflow-runbook.md`，按当前 Go 1.26.1 统一服务入口说明依赖、构建/自动迁移、持久媒体、稳定 APP_SECRET、外部算法 HTTPS 图片入口与配置分工；避免照旧版分离 Web/Worker 说明重复启动执行器。包含完整 P0 图片 YAML、已有飞行替换方式、schedule 的实际含义、资源目录检查边界、明确委托者、复核/报告/失败处理、软件复现命令、真实逐样本验收记录与中文三分钟讲稿。示例 ID 明确为占位示意；两个 YAML 代码块已用当前 yaml 库解析通过。

作者相关正式 API 临时数据库测试再次通过（`TestTaskV2*`、`TestTaskAuthor*`）。初次合并命令的 taskdefinition 名称过滤未匹配测试，未计入测试通过；随后单独运行该包全部单元测试。本文未修改任何执行行为、没有启动真实项目或执行迁移。真实样本/Provider、第二个真实项目复现、P1/F 仍未完成，7.4 保持未勾选，总体 13/32。

### P1 航线版本契约与只读预览（2026-09-12，8.1 部分）

新增 `flighthub.InspectionWaylineVersion`、FreezeInspectionWayline/VerifyInspectionWayline：复用现有司空目录 `update_time:size` 版本约定，冻结航线身份、更新时间与大小。缺身份/正更新时间/正大小拒绝冻结；航线 ID、更新时间、大小或序列化版本不一致均拒绝比较。此为可比较的远端元数据版本，不是航线文件内容哈希，也不包含 download_url。

新增 `GET /api/projects/:id/inspection/flight-plan?connectorId=...&waylineResourceId=...&deviceId=...`，在 repeatable-read 只读事务内检查 project:view、项目/团队、活动司空连接器、活动 wayline 以及同连接器设备外部身份，读取目录摘要并核对已保存 remote_version 与冻结值。返回 `verification=catalogue-only`、source、immediate 与 schedulerOwner=aerosight，接口没有派发路径。

临时数据库正式 API 测试通过：正确预览、无动作作业、跨项目拒绝、目录版本不一致/缺版本拒绝、missing 航线和撤权拒绝；版本比较单元测试通过。预览本身未访问真实司空，不等于飞行执行前的新鲜校验。

8.1 保持未勾选：选择 UI、Task 发布快照与执行前远端再次核验尚待接通，flighthub-flight 发布门控仍关闭；8.2–8.6 未开始执行动作。总体 13/32。

### P1 航线选择器与 YAML 草稿（2026-09-12，8.1 部分）

新增项目只读 `/inspection/flight-plan-options`，列出活动司空连接器的航线和 managed 设备身份（每类最多 200 条，界面明确目录上限）；返回同连接器关联供选择器过滤。新建模板增加“司空新飞行（仅草稿）”，显式预览后将 connector/device/wayline 及版本写入当前 YAML 的 flighthub-flight observe 步骤，按步骤语义定位而不是假定第 1 行；其他参数保留。更换航线/设备清除旧预览；预览响应不自动改写草稿，只有点击写入才应用，提交期间控件禁用。

默认 schedulerOwner=aerosight、taskType=immediate，模板保留 P0 detect/assessment/issue/report 后续步骤；页面明确仍未开放飞行执行。只读 options/preview 数据库测试通过，`pnpm build` 全流程通过。首次浏览器 `.build/aerosight_test_author_7786d319fdf780d9/result.json` 验证实际隔离目录选择→预览→YAML 版本、换设备清除预览及 connector_action_jobs 为 0；已查看选择器截图。尚需已保存草稿编辑入口、发布/执行前新鲜版本核验和 P1 提交链路，8.1 不勾选。

最终浏览器 `.build/aerosight_test_author_edc4d73fab84bbec/result.json` 全部通过，追加确认正式创建 P1 草稿后 YAML 航线版本原样保存，正式 validate 的 canPublish 仍为 false。预览和草稿保存均未创建飞行动作作业。8.1 部分推进，总进度仍为 13/32。

### P1 草稿冻结选择校验与远端复查基础（2026-09-12，8.1 部分）

为 flighthub-flight 安装明确输入契约：同项目 connector/device/waylineResourceId、冻结 waylineVersion、schedulerOwner=aerosight、taskType=immediate。发布校验使用服务端固定契约，不信任作者自定义弱 inputSchema；冻结选择须为字面量，拒绝动态替换。重新核对活动司空连接器、同团队 managed 设备身份、活动航线以及目录版本/身份/时间/大小。只读预览同步收紧 managed 身份检查。有效选择仍返回 TASK_CAPABILITY_NOT_DEPLOYED，未开启新飞行发布或执行。

新增 RevalidateInspectionWayline，只用已授权调用方提供的 connector Instance 解析项目与 TokenResolver，调用已有 GetWayline 做新鲜比较。单元测试覆盖远端项目/航线参数、下载 URL 变化不改变版本、远端版本变化、远端失败原样返回、伪造快照在读取凭据前拒绝。该函数尚未接入 P1 执行器，因此不宣称执行前校验已经贯通。

已保存 Task 草稿增加同一航线选择器，显式预览/写入编辑器后仍需保存，保存时禁用源编辑器。正式隔离数据库浏览器测试 `.build/aerosight_test_author_46953167b55bfa85/result.json` 通过：合法草稿仍不可发布、目录更新后旧版本验证失败、在保存的草稿中重新选择新版本、保存/刷新后保留，动作作业总数为 0。原有巡检复核/案件/报告等浏览器回归同时通过。

验证：临时数据库 TestInspectionFlightPlanPreviewScopedAndNoActions、航线单元测试、pnpm check、pnpm build（含 sqlc 一致性）及 git diff --check 均通过。pnpm check 的普通测试不代表需额外数据库环境变量的集成项均被执行；上述 flight-plan 项另用隔离数据库显式执行。未修改应用数据库、未访问真实司空或发起飞行。8.1 仍缺运行链路绑定，保持未勾选；总体 13/32。

### P1 现有提交处理器的冻结航线门控（2026-09-14，8.1/8.2 部分）

FlightActionRequest 增加仅内部使用的可选 inspection 契约，随既有请求信封加密保存，不转发给 DJI，既有公开动作 schema 不开放该字段。携带该契约的作业必须为 AeroSight 所有的 immediate create，不允许时间窗口、重复选项或 recurring/continuous 参数；冻结版本身份必须与 ledger 航线相同。

现有 FlightActionHandler 在 BeginAttempt 之前、司空 dispatch-check 之后重新读取远端航线并比较版本。prepared 作业重启/读取重试也重新做 dispatch-check 和航线核验；读失败没有写尝试。reconciling 分支保留在该门控之前：已经提交后航线变化不能阻止既有对账或导致再飞。原来的审批、能力和连接器门控继续生效。

新增处理器替身测试：queued/prepared 版本变化、伪造版本、recurring/隐含第二调度器/错误调度归属被拒绝且 attempt_count=0；读取超时后重试再次预检；写超时后仅对账一次飞行，即使航线此后已变；prepared 的新 warning 和撤销审批不发起创建。flighthub 包测试通过，追加 TestInspection* / TestFlightTask* 回归通过，git diff --check 通过。首次测试文件写入因工作目录错误未创建，已修正并实际执行新测试；不把该次旧测试成功算为新测试证据。

本次仍未接通 observe→动作作业创建、独立飞行 Run 与业务 Run/step 的绑定。已确认 SQLFlightActionStore.Complete 会更新飞行 Run，因此不得将业务 Run 直接用作其 task_run_id；下一步须保留独立飞行投影身份并建立关联。无真实司空请求/飞行，无应用库迁移。8.1–8.3 仍未完全满足，整体保持 13/32。

### P1 独立飞行 Run 与业务步骤关联（2026-09-14，8.3 部分）

新增增量迁移 0076_inspection_flight_bindings，业务 project/step 唯一、action_job 唯一、flight_run 唯一，禁止 business_run=flight_run；组合外键绑定动作作业的项目、团队、连接器、物理飞行 Run 与 create 动作类型，并绑定 business step 的实际 Run。沿用 connector_action_jobs 作为唯一提交账本，不另建派发执行器。schema 与 sqlc 已同步。

SQLFlightActionStore.RecordAccepted/Complete 在同一事务先读取业务绑定并记录 inspection_flight_ownership，再写远端投影/动作结果。远端 canonical_target_id 保留 flight_run_id，告警 ownership 保存 business_run_id。pending 可升级为 task，同一业务重放允许；legacy/其他任务归属不覆盖，事务回滚不产生部分投影。已记录的远端 ID 不允许换成另一个架次。没有 inspection binding 的原有作业保持原行为。业务取消后的晚到结果可以记录归属，不等于继续执行后续步骤。

隔离数据库 TestInspectionFlightBindingRejectsMisassociation 通过：错步骤、相同业务/飞行 Run、错误飞行 Run/连接器/项目、重复复用作业拒绝；实际 SQLFlightActionStore 接受 ACK 和完成对账时，投影指向独立飞行 Run、ownership 指向业务 Run、业务 Run 保持 running；ACK 重放成功，legacy 冲突和变更远端 ID 拒绝。TestInspectionUpgradePreservesHistoryAndEnforcesScope 隔离升级/重复迁移通过，flighthub 包测试通过，pnpm db:check 与 git diff --check 通过。只在临时数据库应用迁移。

尚需正式 observe 在同一授权事务创建飞行 Run/动作及 binding、派发前告警策略、业务取消与未知占用处理、媒体终态等待。仅完成关联基础，不勾选 8.3，整体仍 13/32。未实际飞行或访问真实 Provider。

### P1 正式预检/审批生产入口缺口（2026-09-14，设计待确认）

继续接 observe→动作作业时，检查当前 Go HTTP 路由、mission 实现、数据库 queries 与生产源码，未找到生成本地 preflight_snapshot_json、发起 approval_requests、确认审批的正式 API/服务。现有 FHFlightAuthorize 与 FlightActionHandler 读取并强制要求 safety_policy_version_id、preflight.allowed、有效 approved 请求及 approval.context.preflight.allowed；mission-run-workbench 只展示这些状态。flight_action_test.go 的成功前置通过直接 SQL 插入安全策略、预检结果和 approved 请求获得，不能当作正式用户路径已有的证据。

因此 design 第 5 节“复用现有预检、动作审批/授权”的生产入口假设不完整。现有远端 dispatch-check、加密动作 ledger 和对账仍可复用，但需要明确并补齐本地预检/授权产生路径，尤其定时 Task 的授权如何绑定发布版本、设备、冻结航线及有效期。不得自动生成 approved=true 或复制测试种子来绕过此缺口。本次将该设计问题向用户提出；尚未改写规格授权语义、未开启飞行发布。总体仍 13/32，目标未完成。

### P0 动态引用下的固定观察输入契约（2026-09-14，2.1 部分）

P1 授权语义仍待用户确认，本次推进独立的 P0 发布校验。发现 assets/existing-flight 观察步骤使用作者自定义 inputSchema 时，若 with 包含尚未解析的引用，发布阶段整组 ResolveReferences 失败后原有校验被跳过，非法静态 maxImages、缺少有限范围字段和未知参数可能进入已发布版本。

提取两个观察模式的固定输入 schema，解析默认值与发布校验共用。发布时不信任宽松自定义 schema，保留必填、范围和条件约束，仅对已经由解析器验证过的动态引用字段推迟值校验；已知静态字段仍当场检查。运行时原有资源与输入复查保留。

新增真实隔离数据库 HTTP 测试 TestTaskAuthoringCanonicalObservationContractWithDeferredInputs，验证宽松 schema 下合法动态 assets/connector/flight 引用可发布，maxImages=0/1001、缺必填、未知字段、确认有限范围却缺清单/说明、空白说明均返回 TASK_STEP_INPUT_INVALID:observe。单独新测试与所有 TestTaskV2*/TestTaskAuthor* 隔离数据库回归通过（7.856s），git diff --check 通过。普通无数据库环境的先行测试不计作数据库验证。2.1 的完整能力契约审计尚未全部结束，保持未勾选，总体 13/32。P1 设计问题未获回答，不推断已同意新授权机制。

### P0 后半段固定契约与报告模板字段对齐（2026-09-14，2.1/2.4 部分）

将原生检测、外部检测、assessment、案件、报告的静态参数校验统一纳入固定能力契约：作者 schema 只能额外约束，不能因未解析的输入/步骤引用而跳过其他字段约束。复用已有 external/assessment schema，提取 native schema，移除发布函数中重复的外部检测引用处理。

发现实际 inspectionTaskTemplate 的 report.generate 携带 scope: current-run，而 parseTaskV2 原先给 report 默认空对象 schema，导致该模板报告步骤发布失败。新增报告输入契约，仅接受可选 scope=current-run（不填仍保持原行为）；report 运行器本来即按当前 Run 收集事实，不需要改变报告内容语义。未知范围或未支持参数拒绝，不默默忽略。

新增正式隔离数据库 API 测试：合法 native/issue 动态引用允许；未知原生参数、缺 observation、非法 priority、空标题、不支持报告范围/字段拒绝。将原有作者发布测试样例加入真实模板的 report scope。所有 TestTaskV2*/TestTaskAuthor* 以及 TestInspectionExistingFlightFormalAPIAndConsumer / TestInspectionExternalFormalAPIAndHTTPConsumer 通过（13.317s），覆盖正式 API 与后台消费者的协议替身巡检后半段；不是实际 Provider 验收。git diff --check 通过。2.1/2.4 尚有完整字段错误与跨模式/真实配置证据待收口，未新增勾选，总体 13/32。P1 新授权语义仍待用户确认。

### 作者字段诊断与主动草稿校验（2026-09-14，2.1/2.3 部分）

固定输入契约错误保留原 code，同时通过 taskStepInputError 保留校验 cause。validate 接口提供 stepKey 与最多 32 项 fields（JSON Pointer 相对 /with 路径、schema 约束名），不直接暴露 validator 的原始错误消息或输入值。正式 API 测试验证 priority 错误路径 /with/priority 与 enum 等诊断。

新增 TaskSourceValidation，新建和已有草稿编辑入口可主动“校验当前草稿”，复用只读 validate API，无保存/发布/运行副作用。结果绑定 project+format+source；源文本变化即隐藏旧结果，异步响应仅在当前文本仍匹配时显示。无编辑权限的工作台禁用校验按钮。语法/权限错误显示已有安全错误码，固定契约错误显示步骤和字段位置。

pnpm build 全流程通过（77 migration 文件仅构建打包，未迁移应用库）。正式隔离数据库新契约 API 测试通过；浏览器 `.build/aerosight_test_author_9ecb712223c1506c/result.json` 通过：错误报告 scope 显示 /with/scope · const，编辑后旧诊断消失，正确 scope 校验通过，再次编辑隐藏旧成功提示。现有作者/航线草稿/复核/案件/报告回归同时通过。git diff --check 通过。任务整体 13/32，真实 Provider 与 P1 授权语义尚未完成。

### 观察/识别页面刷新与当前资源重查（2026-09-14，7.1 部分）

观察原图与识别证据页面增加显式刷新，复用 useAPI 的重新请求/加载/错误状态，失败时不继续展示旧结果。来源标签改为显式枚举，未知 mode/source 显示未标明，不默认当作司空飞行/原生告警；这不增加未经证实的“真实数据”标签。

pnpm build 完整通过。隔离数据库浏览器 `.build/aerosight_test_author_c7611c2af66f71a6/result.json` 通过：先读取冻结原图，再中断 observation 请求，点击刷新显示失败且隐藏旧原图，解除断网后重试成功并重新加载图片；已有摘要、复核、案件/报告与作者流程回归通过。git diff --check 通过。

另对当前应用数据库执行只读事务聚合查询（未读出密钥、未迁移或写入）：enabled/default AI Provider=0，active algorithm Provider=0，connected/degraded DJI 连接器目录=18，available image 目录=5；本次加载 .env.local 的进程 APP_SECRET 不存在。连接器/图片仅目录数量，不证明实际远端可读或授权样本具备，不能将这些数据计为真实 Provider 验收。P1 授权规则仍未获用户确认；7.1 来源完整标识及真实配置验收仍不充分，整体 13/32。

### 正式 API 取消已排队研判与审批入口纠正（2026-09-14，3.3/7.3 部分）

扩展 TestInspectionExternalFormalAPIAndHTTPConsumer：从正式创建/发布/启用/手动 Run 经后台完成两张图片检测，在 assessment 持久化、模型作业排队后调用 task-runs/:runId/control(cancel,expectedVersion)，再继续消费 outbox 与模型队列。断言 Run=canceled，模型调用为 0，检测调用仍仅 2 次，无案件/报告/物理设备命令，原触发幂等请求重放返回原 Run。测试必须实际到达该取消边界，不能靠默认成功分支通过。

首次新增场景停留 canceling，原因是该局部测试 consumer 未注册 mission.control（生产 runtime 已注册）；补齐正式处理器注册后通过，未修改生产取消行为。该场景证明排队后的取消，不将其表述为模型请求已在远端执行期间的取消验收。

检查该控制接口时纠正之前“没有确认审批正式入口”的判断：mission_control.go 已支持 action=approve 并调用 ApproveMissionControlRun。应复用该入口，缺口限于本地预检产生、飞行审批发起及定时 Task 授权绑定语义；之前的广义缺口描述以本段为准。现有确认操作并不自动解决这些缺口，P1 授权方案仍待用户确认。整体保持 13/32。

### 研判温度参数端到端与作者表单收口（2026-09-14，2.3 完成）

assessment with 新增可选 temperature（0–2，默认 0.2），模板显式包含此值并通过通用参数表单编辑。固定发布契约验证类型/范围；queueInspectionAssessment 再检查并冻结到 agent_tool_jobs.args_json；worker 读取冻结参数并验证，模型请求实际使用该值。旧作业缺字段回退 0.2，原有 issue Copilot 的 completeMessages 默认行为保持 0.2。没有扩大模型权限、改变案件规则或更换 Provider。

正式隔离数据库 HTTP/Consumer 协议测试验证 assess zero 指定 0.7 时模型收到 0.7，其余旧定义仍收到 0.2；assessment 包测试与发布负温度/非法字符串测试通过。pnpm build 全流程通过。浏览器 `.build/aerosight_test_author_b987f1904079a5d9/result.json` 通过：表单 0.7 创建草稿，YAML 改 0.4，切回表单一致，正式保存/刷新后仍为 0.4，既有触发器、资源、源格式、复杂字段保留/非法文本保留回归同时通过。

2.3 所需共享草稿、资源/manual/schedule/识别来源/研判参数基础表单、双向修改保存重开和未编辑内容保留已有实现与浏览器证据，现标记完成。总体 14/32。此为实际软件路径及协议模型验证，不计真实模型效果验收。P1 授权方案仍待确认。

### 两条前端实际模板正式运行验收（2026-09-14，2.4 完成）

新增 productInspectionTemplate 测试辅助：在明确启用数据库集成测试时调用 Node 读取 apps/web/lib/inspection-task-templates.ts 实际导出的 YAML（需要已安装 web 依赖），再用服务器 YAML parser 解析。只替换测试项目的资源 ID；保留模板实际 dependsOn、步骤 key、默认 temperature 和 report.scope，不再依赖平行手写模板证明产品模板可运行。

assets 模板经正式 create/publish/active/manual API→observe→真实 HTTP 协议替身识别→协议模型 assessment→issue→report 成功，完整沿用已有不可变原图/批次/报告断言。existing-flight 模板经同样正式入口→远端媒体协议替身/司空原生证据→模型 needs_review→正式人工 review reject→报告成功，模型只调用一次、产生一份报告、未创建案件。零原生告警仍不能推断 no_issue。两者均保留触发重放同 Run 与不重复读取/识别验证。

合并运行 TestInspectionExistingFlightFormalAPIAndConsumer、TestInspectionExternalFormalAPIAndHTTPConsumer、TestTaskAuthoringEnableDisableAuthorization、TestTaskAuthoringScheduleManualTrialHTTP 的隔离数据库测试通过（6.630s）。已有浏览器证据覆盖新建入口、委托者展示、失联手动重试及禁用/草稿编辑；启停测试覆盖未发布不能启用、绑定委托者、停用不取消已有 Run、撤权不能启用，手动试跑不改调度水位。git diff --check 通过。

2.4 的两条模板、正式创建/发布/启用/试跑及权限/状态路径满足软件交付要求，标记完成，总体 15/32。资源/模型是隔离测试协议配置，不能替代 1.1/4.4/5.1/7.3 的真实 Provider/效果验收，更不能当作现场飞行完成。P1 授权规则仍待用户确认。

### 固定能力输出引用与 v2 作者契约收口（2026-09-14，2.1 完成）

修复作者声明 outputSchema 中不存在的能力输出后让下游引用的漏洞：步骤输出引用现在同时匹配能力固定输出字段及作者 schema；P0 issue assessment 路径仅返回 issueIds，不允许借旧 issueId 字段绕过。自定义 schema 可收紧真实字段，不能创造 handler 不会返回的字段。输入与 condition 两类引用均有断言验证具体 TASK_REFERENCE_CAPABILITY_FIELD_MISSING。

新增规范化 YAML→显式默认值 JSON 再解析哈希一致性，以及不含 apiVersion/capabilityVersion 的完整历史 typed v1 定义兼容测试。初版负例条件结构和“仅去掉 apiVersion”的 v1 样例本身不符合原 schema，已改为正确的 left/right 条件和完整历史定义，确保测试命中实际所需边界；未为错误测试放宽生产 schema。

2.1 要求证据：taskdefinition/source_test.go 覆盖重复键、别名/标签、多文档、超限/深度、异常数字及 JSON/YAML 等价；inputs_test.go 覆盖 schema 编译隔离与递归输入；task_authoring_test.go 覆盖未知版本/能力、前向依赖/引用、固定输入/输出契约、未部署能力、跨项目资源、规范化等价和 v1 兼容；此前浏览器验证字段诊断与校验无保存副作用。两条产品模板正式 API/runtime 验收证明合法输入持续可执行。

taskdefinition 与作者纯测试通过；隔离数据库全部 TestTaskV2*/TestTaskAuthor*、TestInspectionExistingFlightFormalAPIAndConsumer、TestInspectionExternalFormalAPIAndHTTPConsumer 合并通过（13.539s）；git diff --check 通过。2.1 标记完成，总体 16/32。P1 未部署能力仍拒绝发布，未将作者契约完成当成飞行执行完成。

### 持久状态上的消费者替换恢复（2026-09-14，3.3 部分）

将外部巡检协议测试的 consumer 注册提取为构造函数，每次创建全新 mission/observe/detect/algorithm/issue/report 处理器。新增 assess create restart 场景，在 assessment 已落库而模型尚未调用时更换 workerID/消费者及模型处理器，再在模型结果已提交、案件尚未消费时再次替换。consumer 名称保持同一订阅语义，依赖数据库保存的步骤与作业进度恢复。

断言两个恢复边界均实际到达，最终成功、检测总调用 2、模型总调用 1、报告保留证据/关联案件，稳定来源案件总数与 occurrence_count 不增加；继续保留 API 触发重放验证。首次第二个边界插入到取消分支导致 restarts=1，测试准确失败；移至模型处理后，两个边界均执行通过。

隔离数据库 TestInspectionExternalFormalAPIAndHTTPConsumer、TestInspectionFlightBindingRejectsMisassociation、TestV2StepBoundaryProtectsInputsDependenciesOutputsAndCancellation 与取消/非设备恢复相关测试通过（httpapi 5.132s，mission 2.242s），git diff --check 通过。此证据是提交边界的全新工作器恢复，不声称操作系统进程强杀或远端请求中断期间的效果已经覆盖；3.3 仍不勾选，整体 16/32。真实依赖及 P1 授权方案状态未改变。

### 实际服务进程强杀与复核重放（2026-09-14，3.3 部分）

浏览器验收脚本新增同一隔离数据库/端口/存储/测试密钥上的服务启动和重启辅助。只向脚本创建的子进程句柄发送 SIGKILL，等待真实 exit 后启动同一构建产物；清理同时检查 exitCode/signalCode，避免已被信号终止的进程被当成存活。应用数据库、应用服务及实际凭据不变。

在 seeded 已提交 needs_review 边界首次强杀，重启后确认 Run 仍 paused、无新增案件，再经真实浏览器复核、正式 API 与后台消费者完成案件和报告。在完成边界第二次强杀，重启后使用原始复核请求体重放，返回 200，全部步骤的 ID/状态/output_snapshot_json 一致，案件数仍只增加 1，assessment revision 仍为 2；随后实际报告页和案件页可访问。协议样本的原始 observation/detection/model 记录是预置数据，未调用真实 Provider 或飞行设备。

验收通过：`.build/aerosight_test_author_93545ee7879e8a73/result.json` 记录两次 SIGKILL 及新旧 PID；浏览器脚本最终退出 0 并删除隔离数据库。`pnpm build` 通过（77 个迁移文件、sqlc 一致），`pnpm check` 通过，`git diff --check` 通过。默认 check 中未配置数据库的可选集成测试存在跳过，不能计为全部数据库集成通过；本次浏览器验收实际使用独立 PostgreSQL 数据库。

该证据覆盖暂停/完成提交边界的真实操作系统进程恢复，尚不覆盖远端模型请求在途被杀及 P1 未知物理提交的整链恢复。3.3 不勾选，总体维持 16/32。真实 Provider/授权样本缺口及 P1 预检发起、定时授权设计问题保持未解决，未改变 spec 范围或宣称闭环完成。

### P0 识别页面导航、断线恢复及未知来源（2026-09-14，7.1 部分）

修复复核表单将所有非 external 来源误标成司空原生告警的问题：只有 flighthub-ai 显示司空原生告警，其余显示未标明识别来源。浏览器协议 fixture 增加 unknown 来源并断言不显示司空标签。

正式复核浏览器 fixture 新增从业务 Run 的 detect 步骤打开实际识别证据页面，检查协议模型版本与未知目标位置；断开该证据 API 后点击刷新，确认错误展示且旧位置结果隐藏；恢复请求并重试后重新读取。再从识别页进入观察页，验证既有图片及协议样本范围说明。随后从 Run 的 assess 步骤点击实际研判链接，继续既有双页面并发复核/案件/报告及两次进程强杀测试。原图真实字节预览仍由同一脚本独立的 PNG 封存场景验证，seeded 复核样本不作为真实图像效果证据。

首次新导航测试对已默认展开的 paused 步骤再次点击导致折叠、等待链接超时；改为仅在 details 未展开时点击，无产品行为放宽。重新运行完整浏览器验收通过：`.build/aerosight_test_author_e9b1722ec10dfdff/result.json`，进程最终退出 0。本轮 `pnpm build` 通过；前轮全量 `pnpm check` 已通过，本轮仅来源文案与验收脚本改动。7.1 的来源真实性仍需完整核对，未仅凭导航证据勾选整项；总进度保持 16/32。

### 研判事务外调用、租约及取消边界（2026-09-14，3.3 部分）

审计发现旧 ProcessAssessmentNext 在 Run/job/assessment 事务锁内调用模型，与 design 第 89 行“远端/模型调用不持有数据库长事务”不符。现复用 agent_tool_jobs.started_at 作为两分钟租约边界：授权、冻结关联案件上下文并提交 claim 后才请求模型（45 秒上限）；响应后按 Run→job 顺序重新加锁，以 started_at 精确匹配当前租约，拒绝旧执行者写回，重检 Run/步骤、授权与上下文到期。过期 running 作业可重检授权后回收，有效租约不重复执行；无新增迁移。

取消/暂停后的模型响应仅保留原文并标记 assessment canceled/job failed，不产生继续事件、案件或报告；未完成步骤明确失败而不永久停留 running。已经提交成功的研判不因后续取消被改写。已完成作业不重复请求模型；在途进程崩溃、尚无持久结果时恢复可能重复只读模型请求，此处不宣称远端请求恰好一次，也不适用于物理飞行重试。

TestInspectionExternalFormalAPIAndHTTPConsumer 新增三种隔离数据库正式路径：assess cancel completed（模型 create 决定已提交、案件事件未消费时取消，原文保留）；assess cancel inflight（HTTP 模型已接收请求但用 channel 暂扣响应，正式取消 API 必须在释放响应前完成，之后消费迟到事件不建案/报告，原文留痕 canceled）；assess create lease（模拟持久 running claim，有效租约 ProcessAssessmentNext 返回未处理、模型调用 0，推进为过期后回收并完成原链）。结合既有模型排队前取消及消费者/进程恢复覆盖不同边界。

隔离 PostgreSQL 的外部全链与 agent assessment 测试通过（httpapi 4.400s、agent 2.236s）。测试中的协议图片/检测/模型不可替代真实 Provider 效果，过期 claim 是显式故障注入，不冒充在途进程强杀。完整构建与检查结果见后续记录。P1 未知飞行提交的全链仍未接通，3.3 保持未完成，总体 16/32。

本轮最终 `pnpm check`、`pnpm build` 均退出 0，日志分别位于 `.build/inspection-lease-check.log` 与 `.build/inspection-lease-build.log`；sqlc 一致、77 个迁移嵌入及服务构建通过，`git diff --check` 通过。默认检查中依赖未设置数据库变量的可选测试仍可能跳过，真实临时数据库结果以上述定向集成运行记录为准。

### 旧租约写回隔离与在途暂停恢复（2026-09-14，3.3 部分）

外部正式链新增 assess create stale lease：用 channel 扣留第一次模型响应，显式注入替换 started_at 租约后释放旧响应。确认 assessment 仍 pending/原文为空、无 revision；将替换 claim 设为过期后由 worker 回收并正常完成报告。断言仅该故障场景模型调用为 2、检测仍为 2，关联原有两案且不增加重复来源计数。此为明确注入的租约更替测试，不冒充实际双进程在途崩溃。

新增 assess pause inflight：模型已接收请求、响应未释放时经正式 control API 暂停，迟到结果留痕 canceled，不建案/报告且 Run 保持 paused。再经正式 API resume 并消费事件。初次失败暴露 scheduler 对非设备 failed 步骤仍返回 awaiting_step_result，导致 Run 长期 running；现 Advance 对非设备已失败步骤明确返回 RunFailed/task_step_failed，不再假装等候结果，也不重派失败步骤。设备命令分支保持原行为。

修复后隔离数据库外部正式全链、任务步骤边界、非设备控制及相关调度测试通过（httpapi 3.233s、mission 1.597s）；git diff --check 通过。测试断言恢复后 Run 确实 failed、模型仅调用 1 次、无案件；取消、正常、复核、旧消费者恢复与租约回收矩阵仍通过。3.3 仍待 P1 关联物理流程等完整验收，总体保持 16/32。

本轮最终 pnpm check 与 pnpm build 均退出 0；日志为 `.build/inspection-resume-check.log`、`.build/inspection-resume-build.log`，sqlc 和 77 个迁移嵌入检查通过。未将默认跳过的数据库测试计入集成结论；定向临时数据库运行见上。

### 外部巡检检测移出 outbox 长事务（2026-09-14，4.4/3.3 部分）

发现 algorithm.Processor.Handler 在 outbox 事务持有业务 Run/algorithm Run 锁期间调用 Provider，不满足 design 的事务外远端调用要求。提取 prepare/finish，保留既有算法请求、凭据解密、签名原图、输出映射、原始结果和汇总实现；runtime 为巡检子作业启用独立 ProcessInspectionNext/RunInspection。配置了 worker 的 outbox handler 对 inspection.detect 子作业只确认持久入口，普通历史 algorithm 作业仍沿原路径。

新巡检 worker 按业务 Run→算法 Run 锁顺序领取 queued/过期 running 子作业，复用 started_at 两分钟租约，在提交后发出最长一分钟的 Provider 请求。返回后重新锁定当前作业并比较租约，只允许当前执行者写回；重新核对业务状态和委托者授权。取消/暂停后的响应保留原始结果并将子作业 canceled，不生成成功识别结论。过期回收 UPDATE 带 running/期限条件，避免覆盖已由回调完成的子作业。请求 attempt 先缓存在内存、随终态事务保存；进程在请求中断时可能重新发出只读检测请求，不承诺 Provider 请求恰好一次。

正式外部 API/runtime 集成 fixture 改为与生产一致的 outbox 唤醒+独立检测 worker，不再用旧事务 handler 替代巡检运行证明。新增 detect cancel inflight：Provider 已收到并读取签名图片，暂扣 HTTP 响应；正式取消 API 必须在响应释放前完成。随后仅留存一个已发请求的原始结果，两个子作业均 canceled，总检测请求 1、模型 0，无研判或报告，证明取消不被远端请求占锁且后续图片不会继续发送。

隔离数据库 TestInspectionExternalFormal 及全部相关 inspection/http-json 检查通过（httpapi 12.634s、algorithm 2.942s），包括正式产品模板、零检测、失败、复核、重放、租约和在途取消链。git diff --check 通过。本次为软件协议验收，无真实 Provider/授权样本效果结论，4.4/3.3 保持未完成，整体 16/32。

本轮完整 pnpm check、pnpm build 均退出 0（`.build/inspection-detection-worker-check.log`、`.build/inspection-detection-worker-build.log`）；无新增迁移，sqlc/77 个迁移嵌入及 git diff --check 通过。默认测试跳过的可选数据库项不计为验收，临时数据库专项测试如上。

### 检测工作器事务错误隔离（2026-09-14）

检查统一 runtime 发现任一后台函数返回错误会取消所有 peer；新 RunInspection 原先将单次 ProcessInspectionNext 事务错误直接上抛，可能把临时数据库失败扩大为整个后台停机。现启动前检查数据库配置与正轮询间隔；循环对可恢复处理错误记录日志并在下一 tick 重试，保留原 queued/租约恢复机制；父 context 取消正常返回，不触发额外故障。

新增 TestInspectionWorkerRetriesTransactionFailureAndStopsOnCancellation，以第一次事务失败、第二次成功并取消的回调验证循环确实继续且退出干净；无效 interval 必须在调用处理器前失败。该变更不改变模型/算法失败结果，不掩盖配置缺失，也不新增数据库迁移。

本轮 algorithm/runtime 包测试通过（0.575s/0.690s），专门错误重试测试通过，pnpm build:server 退出 0，git diff --check 通过。本轮未新增真实 Provider 或数据库效果结论；默认包测试中可选数据库跳过项不算集成验收。整体仍 16/32。

### 检测租约回收正式链验证（2026-09-14，3.3/4.4 部分）

新增 assess create detection lease 场景，在正式 API 创建/发布/启用/触发后，待 observe/detect 生成两个 queued algorithm Run 时显式注入 running claim。有效 started_at 租约下调用 ProcessInspectionNext 返回未处理、远端调用数仍 0；将两项 claim 设为三分钟前后，正常消费原有作业并完成检测→模型→案件→报告。

断言两项算法 Run UUID 在恢复前后完全相同、均 succeeded；既有全链断言保证检测调用 2、模型调用 1、报告关联原案件、防重复来源计数及触发重放不新建 Run。边界是否到达有独立断言，不能因跳过故障注入而误通过。隔离数据库 TestInspectionExternalFormalAPIAndHTTPConsumer 通过（3.194s）。这是持久 claim 故障注入，不能算实际检测请求在途强杀；不替代真实 Provider/效果与现场验收。总体仍 16/32。

最新统一服务构建的完整浏览器验收通过并退出 0：`.build/aerosight_test_author_0c59ef46c6a1bfce/result.json`，覆盖作者表单、资源/证据导航、失联重试、正式人工复核/案件/报告及两次实际 SIGKILL 重启。该浏览器复核仍包含明确 seeded 前置阶段，不声称它执行了真实检测或模型；新检测 worker 的实际 HTTP 调用证据来自上述正式数据库协议链。git diff --check 通过。

### 模型响应超时分类及 5.2 验收核对（2026-09-14）

5.2 的结构/证据引用/完整范围校验已有直接代码与测试，但单期建筑图不认定新增违法仍主要是提示词约束，缺少真实授权样本效果验收，不能凭提示词文本勾选整项。

修复 completeMessages 将等待响应头超时归为 MODEL_REQUEST_FAILED、正文读取超时归为 MODEL_RESPONSE_INVALID 的分类缺口。两处使用 errors.Is(context.DeadlineExceeded) 识别超时，并由 failureCodeFor 保留 MODEL_REQUEST_TIMEOUT；不暴露 Provider 响应或凭据，普通错误分类不变。新增正式链 assess timeout/assess timeout body，使用 100ms 客户端超时和不返回头/仅返回头的 HTTP 替身。断言 Run 失败、assessment 明确 timeout、无伪造原文/revision/案件/报告，已完成检测证据保留。

首次替身没有读取请求体，仅等待请求 context 取消，导致 httptest.Server.Close 卡住。确认具体测试子进程后用 SIGQUIT 获取堆栈并终止，堆栈定位替身等待；核验唯一临时数据库中本次 timeout 任务及时间后，删除该遗留隔离库 aerosight_test_77de841639cbbecf。未操作应用数据库或应用进程。替身现先读完请求体，且等待最多一秒。再次运行暴露扩展矩阵超过单测试账号写入限额；仅本 fixture 提高 writeRate 至 1000，生产配置不变，独立限流测试继续覆盖限流规则。

最终临时 PostgreSQL 外部全链、agent assessment 和旧 Copilot 检查通过（httpapi 4.277s、agent 2.213s），测试正常清理。5.2 仍未完成，整体 16/32；未将 HTTP 协议超时测试算作真实模型效果验收。

### 模拟设备关联与未核验来源展示（2026-09-14，7.1 部分）

来源审计确认当前没有统一可信的“已核验真实拍摄”字段，不能从图片可读、文件名、模型版本或司空目录存在推断真实实飞。本轮给 AssetRef 增加可选 sourceDeviceMode（simulator/unverified，旧清单缺字段兼容）。assets observe 只从同项目、同团队的资产关联设备及其 simulator adapter 读取关联信息，随观察清单冻结；不读取作者自行填写的 metadata 声明来升级来源。

观察原图卡片显示“关联模拟设备，不作为实飞证明”；其他及旧清单显示“实拍来源尚未核验”。该字段证明关联设备模式，不认证图像内容真假，也不改变既有图片/已完成飞行的观察模式。无需迁移，不重写历史清单或资产原 Run。

数据库测试新增关联模拟设备场景并核对 sourceDeviceMode，且在之后改变资产设备关联后检查此前两份 unverified 清单保持不变。首次测试设备缺必填 device_type_id 失败，改为正式 legacy.device 类型后通过：inspection 1.557s、httpapi 外部正式链 3.094s。pnpm build 通过（日志 `.build/inspection-source-build.log`）。7.1 仍需完整来源/页面验收，整体 16/32，未虚构真实来源证据。

完整浏览器验收通过并退出 0：`.build/aerosight_test_author_f44f39daee9feb1b/result.json`。隔离 PNG 样本关联正式模拟 adapter/device，实际 observe 输出 sourceDeviceMode=simulator，原图页显示模拟关联提示；既有复核/报告/重启和网络恢复矩阵同时通过。该样本明确是软件协议图片，不是实拍验收。git diff --check 通过。

### 本地配置、司空只读实测与真实模型契约（2026-09-14）

用户授权读取历史记录并明确禁止真实飞行。备份后经用户确认，在本地 `127.0.0.1:5432/aerosight` 执行 5 项待迁移，ledger 共 77 项；已有迁移校验一致。通过正式登录和管理员 API 加密保存默认模型 Provider，启用 `step-3.7-flash`，保存后健康检查为 healthy。密钥不写入本记录。

当前业务项目 1：Copilot active；无图片资产，外部 algorithm Provider 为 0。数据库其他测试项目中的 5 张图片不计作真实素材。使用正式 FlightHub 客户端、已保存 Token 及当前项目范围，只读实测航线 13 条、设备拓扑 1 组、所列设备的飞行历史 0 条、无人机告警总数 0。媒体因无飞行记录尚未验证。每次诊断客户端只允许官方中国 API 域名上的 HTTPS GET，禁用重定向，不启动同步器、调度器或物理动作 Worker；最终一轮 4 次 GET、0 写请求。数量仅代表当前凭据/项目/设备可读结果。

实际空告警页返回 `data: {data:null,page:1,page_size:50,total:0,page_count:0}`，旧客户端误报 SCHEMA_INCOMPATIBLE。现在仅在显式 null 集合和显式零计数均存在时规范化为空数组，原分页检查保留；缺字段、非零数量及分页不匹配仍拒绝。修复后真实 GET 复测成功。新增空页及异常页回归。

新增显式 opt-in 的 `TestInspectionLiveModelContract`：从配置数据库只读加载和解密默认 Provider，使用生产研判提示词、HTTP 调用和结构校验；只传合成证据，不写业务表、不启动设备客户端。必须设置 `AEROSIGHT_TEST_LIVE_ASSESSMENT=1` 才发起计费调用，默认跳过。分别验证完整零候选、不完整范围、单期建筑线索夹带指令文本。早期将零候选强制预期为 no_issue 不合理：规格允许完整范围 no_issue，但不禁止保守复核，且模型明确识别合成数据。改为允许 no_issue/needs_review，并记录实际动作；不能计作真实零案件效果通过。

真实调用另暴露模型将非 update 决策的 issueId 返回为空字符串，严格 JSON 解码拒绝推进。补充提示：仅 update 提供正整数 issueId，其他动作省略；提示版本改为 inspection-assessment-v2。未放宽输出校验或修改模型决策。最终三项真实模型契约调用全部通过，均返回 needs_review，耗时 8.398s / 6.283s / 8.216s，temperature=0.2。这证明本次模型协议与保守复核路径可用，不证明真实图片识别、稳定效果或实际建案完整闭环。

Tasktrigger/inspection/agent/httpapi 相关数据库矩阵通过（4.535s / 1.340s / 3.253s / 13.709s）。完整作者与复核浏览器回归通过：`.build/aerosight_test_author_d32e6783c40ae9d3/result.json`，34 项检查和两次 SIGKILL 重启，physicalActions=false；使用隔离协议样本，不能作为实拍验收。真实识别服务、授权样本与真实异常/不建案链仍缺，整体 16/32，不勾选混合验收项。

修复后的 FlightHub 包测试通过（1.414s），Go 开发构建与 `git diff --check` 通过。扩大 HTTP API 检查时，首次未开启详细进度，运行约 142s 后人工中断用于诊断；后续详细日志证明测试集在持续推进，不能把该等待记成已定位死锁。第二轮总时限设为 90s，在第 84 项开始约 1s 时达到总时限，前 83 项通过；仅对剩余 43 项另设三分钟完成检查，避免重复全部已通过测试。两次中断本身不计作套件通过。

剩余 43 项 HTTP API 测试全部通过（54.535s）；两段日志合并覆盖 126 个不同顶层测试，均至少完成一次，不能宣称单次全量命令退出 0。定位到本次中断遗留的两个临时库，核实测试用户/项目及创建时间（13:32:46、13:34:50），且无连接后，仅清理 aerosight_test_362eadcb4d70f0e3 和 aerosight_test_432e9a12dadc1eb8；未清理业务库。日志：`.build/inspection-httpapi-full.log`、`.build/inspection-httpapi-remaining.log`。本轮不涉及实机控制，真实素材与识别服务缺口未消除。

### 研判提示词版本按入队快照执行（2026-09-14，5.1 部分）

上一轮完成只读实测与修复，属于实质进展。本轮继续审核未完成项时发现：入队已保存 prompt_version，但 worker 总是使用最新提示词。更新为 v2 后，旧 v1 作业恢复会造成版本记录与实际请求不一致。

执行器现在同时读取 assessment.prompt_version 与 job.args_json.promptVersion，要求一致且版本受支持；未知/缺失/不一致明确失败，不发送模型请求。保留独立的 v1 原文常量，新作业继续冻结 v2；真实 HTTP 研判使用已冻结版本选择提示词，不改写历史记录。温度与证据、执行授权和迟到结果检查仍保留。

数据库测试用正式排队入口生成作业，模拟升级前的 v1 快照，实际消费后核对 HTTP system 消息确为 v1；后续 v2 作业核对新提示词。新增版本不一致、未知版本的失败状态/错误码/零模型调用/无伪造原文断言。agent/httpapi 巡检回归通过（2.400s / 11.354s）；补充两个版本失败场景后 agent 定向复测通过（2.043s）。不使用真实模型重复计费请求验证固定字符串分派。5.1 的真实识别证据全链验收仍未完成，不勾选整项，整体保持 16/32。

### 报告来源标签与 P1 缺口复核（2026-09-14，7.1 部分）

上一轮版本追溯修复属于进展。本轮检查来源展示发现报告页仍把所有非 assets 模式写作“司空已完成飞行”，与观察页的显式枚举不一致。报告现区分 assets（既有图片）、existing-flight（司空已完成飞行）、flighthub-flight（司空飞行）和未知（未标明来源），未知模式不再形成错误完成声明。

浏览器使用明确报告协议数据分别验证四种模式，并断言非 existing-flight 不出现“司空已完成飞行”。完整 `pnpm build` 通过（`.build/inspection-report-source-build.log`），完整浏览器验收通过（`.build/aerosight_test_author_2cd6eb1b98a83e67/result.json`），新增来源标签检查；`git diff --check` 通过。无真实飞行调用。7.1 仍保留整体未验收，未把标签修复等同于真实来源认证。

再次核对 P1 生产入口：`httpapi/mission_control.go` 的 approve 分支确实可以确认已绑定的 approval_request，旧记录中“未找到确认审批入口”的表述不准确。尚缺的是预检快照和审批请求的正式生成、定时授权对发布版本/设备/冻结航线/有效期的绑定，不是完全没有确认接口。该澄清不意味着可自动生成批准记录，也不开放实机动作。真实识别服务选择已向用户询问，等待配置/服务方向及真实素材，不擅自假定供应商。

### 历史媒体查询纠正（2026-09-14）

用户要求再次验证是否能获取图片/视频。扩展为 Token 可读项目、机场/无人机分别查询、明确秒级时间窗口，取得项目创建以来 77 条历史；之前无时间条件返回 0 条不足以证明没有历史。现有列表解析器拒绝真实 manual/空航线，最近接口数字枚举另有契约差异。本次直接按官方成功响应的 flight UUID 逐架只读媒体查询：19 空目录、21 目录不存在、32 服务器错误、5 非空列表（仅目录项及一个 DAT，无图片视频）。DAT 签名 URL Range GET 返回 206/1024 字节，证明下载链路可访问，不计作影像验收。详细报告见 docs/operations/flighthub-media-readonly-audit-2026-09-14.md。本轮不改写业务数据或发起实机动作，纠正此前“项目无历史飞行”的过早结论；媒体/算法真实验收仍未完成。


### 历史航拍真实单链 Demo（2026-09-14，4.4/5/7 的部分验收）

用户确认继续完整演示，使用其手动下载的真实航拍 RGB 图。在独立 aerosight_msup_demo 库以正式 API 导入资产、创建并启用算法 Provider、发布 Task，由标准 serve 执行器完成 Run #3 的五步：observe → external YOLO11n → step-3.7-flash assessment → 模拟网页复核 → issue → report。8 个 car 预测框，模型 needs_review，助手代操作演示复核确认 1 条待核实线索并驳回 7 条，生成工单 #1 和报告草稿。没有 SQL 强制步骤成功，也没有真实飞行动作。

补充正式图片导入 API（owner/admin、完整解码、40 MiB、来源/哈希/拍摄时间、审计与写前重新授权）；算法 Provider 显式启停；development 精确 loopback HTTPS 入口及部署 CA 加载；工单巡检证据导航、冻结原图检测框叠加。生产环境不接受开发地址选项，其他出站策略未放宽。

完整 pnpm check/build 通过；真实临时数据库验证导入权限、>2 MiB 图片、非法输入不入库、Provider 启停；本地地址边界与真实 TLS CA 验证通过。独立 YOLO 的六图与异常 API 测试见 earlier local media artifacts。运行详情、截图与复现步骤见 docs/demo/local-demo-runbook.md、.build/msup-demo/run-detail.json、.build/msup-demo/screenshots/。

仍为 16/32：单链成功不等同于 P0 全矩阵。效果标注、重启/取消/重放等真实矩阵、第二项目复现、P1 和 F 未在本次整体验收。模型原文对 allowedIssueIds 的解释及报告汇总模型版本显示仍有局限，保留原始证据，不修饰为更强结论。
