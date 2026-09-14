# 巡检 Task 部署、操作与参赛演示

适用变更：`complete-inspection-task-workflow`。本文描述当前代码和实际验收边界；是否完成以该变更的 `tasks.md` 与 `implementation-evidence.md` 为准。

## 当前交付范围

| 层级 | 要证明的行为 | 当前可用证据 |
| --- | --- | --- |
| P0 | 既有图片或已完成飞行 → 识别 → AI 研判 → 复核 → 案件/不建案 → 报告 | 已有正式 API、后台执行器及浏览器协议样本验证；真实图片与真实 Provider 效果验收尚未完成 |
| P1 | AeroSight 定时触发一次司空 immediate 飞行，并接回 P0 | 本轮尚未完整接通，不能把模板草稿可保存当作可以执行 |
| F | 实际设备定时飞行、回传、真实识别、研判与报告 | 未完成，需单独现场授权与验收 |

任务语法是 `aerosight/v2` 的顺序步骤 DSL。借鉴 GitHub Actions 的 YAML 编辑习惯，但不支持其完整 jobs、matrix、shell run 或第三方 Action 执行环境。定时是 trigger 的一种；同一份草稿可在 YAML 和参数表单间切换。

## 平台分工

- 司空：设备接入与飞行管理、预设航线、飞行状态、媒体目录/下载及账号实际支持的原生 AI 告警。司空已有区域/巡逻航线直接复用，本轮不做本地覆盖航线算法。
- 外部算法 Provider：对明确图片集合执行适用的检测服务；必须确认类别、输入限制及模型版本。
- 已有 AI Provider：依据冻结的识别证据研判，不能调用写案或设备控制工具。
- AeroSight：Task 定义、触发、范围冻结、执行状态、人工复核、稳定来源防重、案件与报告导航。

司空账号能在 UI 操作某功能，不等于当前套餐/项目的 OpenAPI 能访问该功能。目录检查和协议测试不能证明实机成功。

## 启动当前统一服务

当前仓库使用 Go 1.26.1（见 `apps/server/go.mod`）、Node.js 24+ 与锁定的 pnpm。构建后的 `.build/aerosight serve` 同时运行 API、静态页面、后台任务与回调入口；不要为同一部署另外启动一套旧版 Web API 和 Worker。需要独立 Worker 的运维场景另行规划。

1. 准备 PostgreSQL/PostGIS 和持久化 `DATA_DIR`，为数据库及媒体分别建立备份。
2. 参照 `.env.example` 配置 `.env.local` 或服务环境；不要提交密钥。核心配置包括 `DATABASE_URL`、稳定的 `APP_SECRET`、`CSRF_SECRET`、`PUBLIC_ORIGIN`、`HOST`、`PORT` 和 `DATA_DIR`。
3. 运行 `pnpm install --frozen-lockfile`、`pnpm build`。
4. 确认部署目标和备份后运行 `pnpm start`。当前 serve 会自动应用嵌入迁移并执行数据库 bootstrap，启动不是只读操作。
5. 使用 HTTPS 公开入口。外部算法读取签名图片需要可访问的 HTTPS `CALLBACK_PUBLIC_BASE_URL`；统一服务未设置时回退到 `PUBLIC_ORIGIN`。仅本机 HTTP 的页面可浏览，并不证明外部 Provider 可以读取图片。

`APP_SECRET` 同时涉及已保存凭据的解密与签名，已有数据库必须使用原值；缺失时恢复原密钥或遵循凭据轮换流程，不能随手生成新值覆盖。算法出站主机受 `ALGORITHM_ALLOWED_HOSTS` 等已有策略约束。数据库凭据、Provider Key 和司空 Token 都不应写进 Task YAML。

## 演示前准备

1. 明确参赛使用的 AeroSight 项目与对应司空项目。登记图片授权、原始来源、观察时间及允许分析的范围。
2. 在项目数据资产中确认待分析图片已存在且可用；图片被其他 Run 使用不改变它的原始归属。不要用 SQL 修改任务成功状态来准备真实演示。
3. 外部检测路径：配置 active http-json Provider 和已发布 detection 算法版本，记录实际版本 ID、支持类别与每次图片上限。在独立验收记录中证明调用有效。
4. 原生告警路径：连接司空并同步已完成飞行；确认目标架次、完整媒体和实际算法类别。开启显式 Task 告警管理策略，防止旧告警自动建案绕过研判。待归属告警需核实处理，不能批量假定为 legacy。
5. 管理员配置启用的默认 OpenAI 兼容 AI Provider；项目有 active Copilot。先核对密钥能解密、模型支持协议及出站可用。
6. 执行账号需要任务操作、智能体使用与案件处置权限。任务详情显示当前执行委托者；启用绑定当前账号，运行前会重新检查权限。

新建/编辑页面的“资源就绪检查”只读目录与配置：图片数、飞行目录数、算法版本数、Copilot/默认模型是否配置。它不验证实际图片授权、密钥解密、远端连通或模型效果。

## P0 模板操作

进入项目“任务”→“新建任务”，选择“已有航拍图片”或“司空已完成飞行”。填写真实资源 ID 后创建草稿。新 Task 默认停用；保存、发布、启用是不同操作。非法文本会保留，未保存更改不能发布。

以下是图片模板示例。`101` 和 `9` 是示意 ID，必须替换成当前项目资源，不能原样用于验收。

```yaml
apiVersion: aerosight/v2
name: 授权图片巡检
trigger:
  type: manual
concurrencyLimit: 1
steps:
  - key: observe
    uses: inspection.observe
    with:
      mode: assets
      assetIds: [101]
  - key: detect
    uses: inspection.detect
    dependsOn: [observe]
    with:
      observationId: steps.observe.outputs.observationId
      source: external
      algorithmDefinitionVersionId: 9
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
```

对于已完成飞行，observe 使用 `mode: existing-flight`、`connectorId` 和 `flightUuid`；detect 可选择 `source: flighthub-ai`。选择 external 时仍需本项目算法版本 ID。请以应用模板的当前内容为准。两种 observe 都只读既有数据，不会起飞。

先手动试跑。需要定时分析同一已有数据源时，将 trigger 改为：

```yaml
trigger:
  type: schedule
  cron: "0 8 * * *"
  timezone: Asia/Shanghai
  enabled: true
```

这表示每天 08:00 分析模板明确引用的数据；不会自动寻找“最新架次”，也不会发起新飞行。任务跨版本并发为 1，计划扫描仅处理允许窗口，重启不会补执行全部错过的计划。手动试跑不推进计划水位。停用只阻止新触发，不取消当前运行。

参数表单与原始编辑共享草稿。复杂嵌套参数、条件和自定义步骤使用 YAML；表单无法表达的内容应保持不变。发布遇到未部署能力或跨项目资源时应修正配置，不修改数据库绕过门控。

## 运行、复核与报告

1. 运行详情查看每步输入/输出、状态、失败原因与来源链接。
2. 观察页面显示封存范围和原图版本。预览验证实际字节哈希；远端签名过期会通过稳定引用刷新，仍失败则明确不可用，不用新文件替代原图。
3. 识别页面显示算法/告警来源和模型版本。单张照片位置不等于目标坐标；零告警不自动等于没有异常。
4. 任一 needs_review 暂停整批案件处理。进入研判页查看模型原文与证据后，填写理由并确认、调整或驳回。普通 resume 不能跳过复核。
5. 两人并发复核时旧 revision 被拒绝，输入保留；刷新后再核对现有决定，不强行覆盖。复核不会重新飞行或识别。
6. 确认后的案件通过稳定来源防重；同来源再次分析不会重复计数或重开闭案。驳回只表示不采纳该线索，不是对未观测区域宣告无异常。
7. 暂停时可查看“巡检进展与待复核摘要”，它不是最终报告。流程完成后打开报告并导航案件、观察、识别和研判。

手动运行遇到响应丢失时，在同一页面、同一版本与输入下重试会复用幂等标识；刷新页面后应先检查已有运行，当前实现不持久化跨刷新请求标识。

## 常见阻塞与处理

| 现象 | 处理 |
| --- | --- |
| 资源目录有图片，但预览失败 | 核对存储、版本与哈希；远端图片还需原飞行/连接器可访问。保留失效提示，不覆盖封存证据 |
| Provider 配置存在但调用失败 | 检查原 APP_SECRET、Provider 凭据、HTTPS 入口、出站主机策略与响应格式 |
| needs_review 长时间暂停 | 有权用户核对缺失资料并复核；过期上下文应重新规划有效分析，不改库强制成功 |
| 发布失败 | 查看服务端资源/能力/引用错误；草稿可保存不代表功能已部署 |
| 业务 Run 已取消 | 不能据此判断飞机已停止；物理状态需通过司空或已验收控制入口确认 |
| 未配置真实样本/服务 | 软件协议测试可继续，真实效果项保持未验收 |

## 软件复现与真实验收分开记录

常规检查：`pnpm check`、`pnpm build`。依赖数据库环境变量的测试跳过时不得计作集成通过。

浏览器软件复现（会创建并删除临时数据库；需数据库建库权限和 Chrome）：

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' node scripts/test-inspection-authoring-browser.mjs
```

此脚本包含明确标记的协议/种子样本，覆盖真实作者 API、观察读取、正式人工复核、案件与报告、冲突及网络响应丢失。它不证明真实模型质量，也不证明实机飞行。输出位于 `.build/aerosight_test_author_*/`。

真实验收另存一份逐样本表：授权来源、冻结时间、标注、预测、模型/参数、耗时、误报/漏报、Run/observation/evidence/assessment/issue/report ID。至少记录异常建案、明确范围不建案和待复核链路。新项目第二次复现必须通过正式入口，不能用种子 SQL 冒充。F 验收同时参照 `dji-flighthub-field-acceptance.md`，记录现场动作授权与实际结果。

## 三分钟演示讲稿

“司空负责飞行与航线，我们的平台把巡检业务组织成可审计的 Task。今天演示的数据来源是【填：授权既有图片/司空已完成飞行/已验收现场飞行】。”

“这里用 YAML 或表单定义步骤，定时只是触发方式。每次运行先固定观察范围，再调用适用算法和 AI。证据不足会暂停交给人复核，不直接把模型文字变成案件。”

“现在看原图版本、识别来源和研判原文。确认后进入案件，驳回则不建案；重复来源不会产生重复案件。报告保留分析范围与证据链接。”

结束时展示 P0/P1/F 分层状态。尚未完成实飞时说“已验证软件流程，真实模型效果/现场飞行待验收”，不要说“无人值守航拍全链已完成”。


研判步骤支持可选 `temperature`（0–2，默认 0.2），例如 `with: {mode: assessment, evidenceSetId: steps.detect.outputs.evidenceSetId, temperature: 0.2}`。模板参数表单可直接编辑；该值随 Run 入队冻结到模型作业，后续修改草稿不改变已排队请求。模型与 Provider 继续使用已配置的默认 Provider，原有证据约束和复核规则不变。

### 研判运行中取消与恢复

研判作业使用现有 `started_at` 记录两分钟租约；模型请求最多 45 秒，调用期间不持有业务 Run 数据库锁。有效租约不会被其他消费者抢占；进程中断后，过期的未完成作业可重新授权并恢复。已提交成功结果不再次请求模型。若进程在响应提交前中断，恢复可能再次调用只读模型，不能承诺外部模型计费请求恰好一次。

模型返回后重检租约、Run/步骤状态、委托者权限和上下文期限。运行中取消可以在模型响应前完成；迟到输出保存为 canceled 研判记录，不恢复 Run、不建案、不生成报告。暂停期间收到结果同样不推进，步骤明确失败，需核对运行状态后处理。该机制只涉及只读研判，不能用于自动重试未知飞行提交。

若在模型在途时暂停，迟到输出会使该步骤明确失败；随后恢复会让业务 Run 收口为失败，不能用 resume 将这份迟到输出变成成功研判。检查失败原因后创建新的分析运行。失败的非设备步骤不再无限等待结果，也不会自动重新执行。

巡检外部检测也由统一 Go 服务内的独立后台作业执行，复用算法 Run 的两分钟租约。Provider HTTP 调用不占用业务 Run 事务锁，单次处理总期限一分钟；取消后停止尚未发出的图片请求，已发请求的迟到原始结果可以留痕，但不进入成功研判。outbox 唤醒和检测作业都必须运行；标准 `serve` 已装配两者，不要只运行一个自定义 outbox 消费器代替完整服务。进程在结果提交前中断时，检测 Provider 也可能收到重复只读请求。

### 可选真实模型契约检查

`apps/server/internal/agent/assessment_live_test.go` 仅在显式设置 `AEROSIGHT_TEST_LIVE_ASSESSMENT=1` 时调用已配置默认模型。运行时需提供 `DATABASE_URL` 与原 `APP_SECRET`：

```sh
cd apps/server
AEROSIGHT_TEST_LIVE_ASSESSMENT=1 go test -tags dev ./internal/agent -run '^TestInspectionLiveModelContract$' -count=1 -v
```

该命令只读 Provider 配置并产生模型计费请求，不创建业务 Run 或案件。输入是合成协议样本，检查 JSON 契约和保守复核；完整零候选允许 no_issue 或 needs_review。通过不代表真实照片识别或真实不建案效果验收。默认常规测试跳过它。

研判作业按入队时冻结的提示词版本执行。当前新作业使用 `inspection-assessment-v2`，已排队的 v1 保留原文；assessment 与作业的版本不一致，或运行程序不支持该版本时，在模型调用前失败。升级不会静默替换旧作业的提示词。
