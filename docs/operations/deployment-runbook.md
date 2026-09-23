# AeroSight MVP 部署与现场运行手册

本手册覆盖单区域试点部署。试点默认关闭设备命令、对象存储和外部算法；每个项目必须由管理员逐项配置。AI 只由智能体聊天、案件 `@copilot`/指派或 Task `copilot.run` 显式调用。部署不等于允许飞行，现场负责人仍须完成法规、空域、设备和人员确认。

## 1. 组件与前置条件

- Web：Node.js 24+、pnpm 10.33，提供 UI、API、认证和同步事务。
- Worker：Go 1.24+ 编译出的 `.build/aerosight-worker`，消费 PostgreSQL outbox/任务并提供 `/healthz`、`/readyz`、`/metrics`。
- 数据库：PostgreSQL 17 + PostGIS 3.5；数据库必须支持扩展、事务、advisory lock 和 `FOR UPDATE SKIP LOCKED`。
- 对象存储：MVP 运行时使用 `DATA_DIR` 指向持久卷；Web 与 Worker 必须挂载同一路径。不要使用容器临时层。S3-compatible 接口已有领域契约，但当前部署入口尚未配置远端 S3 client。
- TLS 入口：Web、callback 和媒体访问都应由受控 HTTPS 入口暴露；数据库、指标与 worker callback 监听不直接暴露公网。

建议为 Web、Worker 和迁移任务使用同一不可变构建版本，为数据库和对象目录分别配置加密备份。

## 2. 环境变量与秘密

从 `.env.example` 生成部署配置，不要提交 `.env.local`。最低配置如下：

| 变量 | 用途 | 要求 |
| --- | --- | --- |
| `DATABASE_URL` | Web/Worker 共用数据库 | 使用专用最小权限账号；生产启用 TLS |
| `HOST` / `PORT` | Go HTTP 服务监听地址与端口 | 默认 `127.0.0.1` / `8080`；容器使用 `0.0.0.0` / `8080` |
| `PUBLIC_ORIGIN` | 应用公开访问 origin | 生产使用 HTTPS，仅协议、主机和可选端口，不含路径 |
| `CSRF_SECRET` | 防跨站请求伪造 Cookie 的签名密钥 | Base64 编码的 32 字节随机密钥，重启保持稳定 |
| `APP_SECRET` | 登录、短时令牌签名及数据库凭据加密主材料 | 至少 32 随机字节；Web、Worker 与轮换命令保持一致 |
| `LOG_LEVEL` | 日志级别 | `info`/`warn`/`error`，排障时短时使用 `debug` |
| `WORKER_NAME` | worker 实例名 | 每实例唯一，便于日志和指标定位 |
| `DATA_DIR` | 媒体与算法原始结果目录 | 持久卷绝对路径；留空会明确降级且媒体内容不可读 |
| `ALGORITHM_ALLOWED_HOSTS` | 算法出站 allowlist | 逗号分隔主机名；不放 URL、IP、通配符或凭据 |
| `CALLBACK_LISTEN_ADDRESS` | worker callback/健康监听 | 内网 `host:port`，默认 `127.0.0.1:8081` |
| `CALLBACK_PUBLIC_BASE_URL` | 外部算法 callback 根地址 | 必须 HTTPS；经入口转发到 worker |
| `DJI_FLIGHTHUB_API_BASE_URL` | 司空公有云 OpenAPI 区域主机 | 中国大陆固定为 `https://es-flight-api-cn.djigate.com`，其他主机启动失败 |
| `DJI_FLIGHTHUB_HTTP_TIMEOUT_MS` | 单次司空请求超时 | 默认 `8000`，范围 500–30000 ms |
| `DJI_FLIGHTHUB_MAX_RETRIES` | 429/5xx 有界重试 | 默认 `2`，范围 0–3；尊重 `Retry-After` |
| `DJI_FLIGHTHUB_MAX_PROJECT_PAGES` | Web 项目发现页数上限 | 默认 `50`；仅 Web 使用 |
| `DJI_FLIGHTHUB_MAX_RESPONSE_BYTES` | 司空响应大小上限 | 默认 4 MiB，Web/Worker 保持一致 |
| `DJI_FLIGHTHUB_POLL_INTERVAL_SECONDS` | Worker 周期目录同步 | 默认 300 秒；仅 Worker 使用并附加抖动 |
| `DJI_FLIGHTHUB_RECONCILE_INTERVAL_SECONDS` | Worker 调度扫描间隔 | 默认 15 秒；仅 Worker 使用 |

DJI 连接器、算法 Provider 和平台 AI Provider 的凭据由 Web 使用 AES-256-GCM envelope 加密后存入数据库。密钥通过 HKDF-SHA-256 从 `APP_SECRET` 派生；普通读取、审计摘要、日志和智能体上下文都不能得到原文或“是否已配置”标记。编辑表单的敏感 input 始终为空，留空保留旧值，填写非空值才覆盖。

### DJI 司空 2 公有云连接器

中国大陆部署只允许访问 `https://es-flight-api-cn.djigate.com:443`。Web 的 Token 验证/项目发现和 Worker 的设备目录同步都需要到该主机的 HTTPS 出站；防火墙、NAT、DNS 和 TLS 检查代理不得改写主机名或响应。Go Worker 使用标准传输并可遵循 `HTTPS_PROXY`/`NO_PROXY`；Web 当前未注入自定义代理 agent，如部署必须经过显式代理，应在平台网络层提供透明 HTTPS 出口并先完成项目发现验收。

组织管理员在司空 2 的“我的组织 → 组织设置 → OpenAPI → 复制密钥”取得组织 Token。它不是 OAuth 授权码，当前没有由 DJI 托管的授权跳转/回调流程。Token 应只授予所需组织与项目访问，不能放入 URL、工单、日志或源码；轮换时在 AeroSight 连接器卡片选择“更新 Token”，系统会先验证所选项目仍可访问，成功后原子替换加密 envelope 并排队重同步，验证失败则保留旧凭据。撤销 Token 或项目权限后连接器进入失败/降级，不删除历史设备和审计。

启用顺序：先为 Web 和 Worker 同时配置上述连接参数并确认 HTTPS 出站，再在项目中创建或重新连接对应连接器实例。连接器默认每 300 秒同步一次目录；429 按 `Retry-After` 与指数退避处理。设备目录达到官方 1000 条上限时无法证明完整性，系统会失败关闭，不推进游标、不部分提交，也不会把未返回设备标记为 missing。

选择接入方式时：已有机场在 DJI 司空 2 公有云且只需要目录/状态同步，使用“DJI 司空 2”；需要任务下发、返航、机场调试或直播控制，使用“DJI Cloud API 直连”并配置 AeroSight 自有 MQTT/API/媒体端点。不要为同一设备自动切换下行来源；跨来源同 SN 会进入人工冲突确认。

## 3. 全新部署

1. 创建 PostgreSQL/PostGIS 数据库、专用账号和持久对象目录；确认备份与恢复策略已启用。
2. 安装锁定依赖并构建：`pnpm install --frozen-lockfile && pnpm build`。
3. 在仅迁移任务可写 schema 的维护窗口运行 `pnpm db:migrate`。迁移使用 advisory lock；失败时不会登记该迁移，修复原因后重跑，不要手工跳号。
4. 启动 Web 与 Worker：`pnpm start:web` 和 `pnpm start:worker`，或由编排平台分别托管。不要依赖单个 `pnpm start` 进程做生产级进程监管。
5. 检查 Web 登录页、Worker `/healthz`、`/readyz` 与 `/metrics`。关键数据库不可用时 readiness 必须失败；对象存储、算法、模型或 adapter 不可用时项目应显示明确降级，历史数据仍可访问。
6. 创建团队和项目，确认全部项目功能开关保持默认关闭，再按下面的启用顺序进行试点。

可在候选构建上执行 `pnpm drill:fresh-environment`。它验证环境变量样例与秘密占位符，在全新 PostGIS 上跑空库/当前/旧版/重复迁移契约，并构建 Web 和 Worker；任一步失败均以非零状态退出。

## 4. Provider 与存储配置

### 对象存储

为 Web/Worker 挂载同一持久卷，设置 `DATA_DIR`，再开启项目 `object_storage_enabled`。使用一条非敏感测试图片验证上传、checksum、版本、短时读取 URL 和重启后读取。目录不可用时先关闭该开关；不得清空数据库资产记录或把缺失媒体伪装为成功。

### 算法 provider

先配置 `ALGORITHM_ALLOWED_HOSTS`，再由平台管理员进入“系统管理 → 算法服务”，在列表中用“新建算法服务”或“编辑”弹窗配置全平台共享的 Provider：类型、HTTPS base URL、认证凭据、认证方式、允许 header、timeout、并发与速率限制。项目算法定义仍按项目隔离，但可引用任一平台 Provider。运行地址和协议测试，确认 DNS 满足 allowlist，再验证实际调用与 callback 签名，最后开启 `external_algorithms_enabled`。任何 SSRF 拒绝、mapping 漂移或 callback 重放告警都阻断启用。已有项目 Provider 会显示在平台列表中，其历史凭据保持原加密范围；新 Provider 使用平台加密范围。

### AI provider

由平台管理员进入“系统管理 → AI Provider”，在供应商列表点击“新建 Provider”或“编辑”，使用同一个配置弹窗：

- **基础配置**：供应商名称、基础地址、API Key 和启用状态。AI Provider 支持内网、回环地址及 HTTP / HTTPS；地址包含 API 前缀，例如 `http://192.168.1.10:8000/v1`。API Key 加密保存；无认证服务新建时可留空，编辑时留空保留已有 Key。
- **模型配置**：进入分页自动调用当前地址的 `/models` 接口；顶部输入框支持搜索匹配、下拉选择和添加列表以外的模型 ID。模型行只配置协议，能力不再单独声明，默认启用全部能力。协议必须选择 OpenAI Compatible、Responses、Anthropic Messages 或 StepFun Realtime，新模型默认 OpenAI Compatible。
- **默认模型**：在管理页从全部供应商的模型中分别选择默认文字和默认实时模型，不在供应商弹窗里设置。当前运行时接入 Responses 和 StepFun Realtime；OpenAI Compatible 与 Anthropic Messages 目前为协议配置项，调用适配尚未实现，选为默认后不会错误地按 Responses 发送请求。停用或删除默认模型所在的供应商后，需要重新选择对应默认模型。

“测试连接”检查模型列表接口的 HTTP 状态，不代表模型推理或实时语音已经验收。模型获取失败时可保留已有配置并手动添加。已有 Key 不回显，修改未保存的基础地址后获取模型需填写新地址的 Key，或先保存基础配置。

升级时执行迁移 `0078_ai_provider_models.sql`，将已有文字/实时模型转换为模型目录，并保留原来的默认选择；部署新后端和前端后即可使用。之后修改供应商或模型无需重启或 AI 环境变量。没有可用默认文字模型时，智能体文字聊天、案件 Copilot 与 Task `copilot.run` 不可用；没有默认实时模型时实时语音不可用。智能体仍通过既有预检、审批和命令账本访问设备。

## 5. 功能开关与试点启用顺序

项目开关位于 `project_feature_flags`，缺行或空值都按关闭处理。项目 owner/admin 可进入“项目设置 → 项目功能”按树状分组调整，父分组选中表示批量修改当前分组的叶子开关，没有隐式权限继承。保存仅写入有变化的项；遇到其他管理员对同一项的修改时，重新加载再保存。此页面不支持个人授权，也不会生成设备能力验收记录或执行远端操作。司空功能即使开启，实际操作仍受原有角色、连接状态、能力验收和审批条件限制。

基础开关：

1. `operations_overview_enabled`：只读总览和回放。
2. `object_storage_enabled`：完成持久卷验证后开启。
3. `external_algorithms_enabled`：完成 allowlist、加密凭据和 callback 验证后开启。
4. `device_commands_enabled`：完成法规、围栏、预检、审批、现场急停演练后最后开启。

启用前保存审批人、配置版本、验收结果和时间。不要通过直接 SQL 长期开关；紧急情况下如必须操作数据库，应由双人复核并补录审计。

## 6. 发布、回滚与迁移原则

- 发布前备份数据库和对象目录，记录应用 commit、迁移 ledger 与镜像 digest。
- 数据库迁移只前进，不回滚 schema；新代码必须兼容升级窗口中的旧数据。先迁移，再滚动升级 Worker，最后升级 Web。
- 应用回滚只切回上一个已验证构建，不回退数据库、不删除新表/列、不删除新证据。旧 Worker 必须忽略未知 outbox event，并保留事件等待新版 Worker 处理。
- 回滚触发条件包括 readiness 连续失败、重复物理命令、审计断链、租户隔离失败、证据不可读或延迟持续越过验收线。
- 回滚后关闭 `device_commands_enabled` 和 `external_algorithms_enabled`，必要时停用默认 AI Provider，保留只读总览；核对命令账本、未 ACK 命令、callback、Task/Copilot Job 和 outbox，再决定恢复。

现有数据升级与应用回滚的具体演练证据由 `pnpm drill:upgrade-rollback` 生成，并记录在独立演练报告中。

## 7. 现场安全处置

### 起飞前

确认设备实名登记、运行识别、飞行批准或版本化豁免、操控责任人、起飞确认、天气/电量/链路、项目边界和禁限区域。验证现场人员能访问独立紧急停止入口，并明确物理接管、返航和失联区域。

### 异常与紧急停止

1. 立即使用独立高优先级紧急停止；系统应撤销未执行普通命令并记录请求、策略、尝试与 ACK。
2. 若设备 ACK，现场确认停机/返航状态，冻结相关任务和证据。
3. 若重试耗尽仍失联，将设备与任务视为“安全状态未知”，持续告警；现场按预案建立隔离区、联系操控责任人并采用厂商物理接管，不得在 UI 中推定已停止。
4. 关闭项目设备命令，暂停相关 Tasks；需要时停用默认 AI Provider，保留对象、遥测、审计和案件；创建异常报告并施加 retention hold。
5. 在解除现场风险、完成根因和双人批准前不得重新启用飞行。

### 数据或依赖故障

数据库故障时停止所有写操作与设备调度；对象存储故障时停止新采集但保留元数据；算法/AI 故障时切回人工流程；adapter 故障时标记连接降级并按设备本地安全策略处置。任何审计写入失败都必须失败关闭受保护动作。

## 8. 发布检查单

- 备份可恢复，迁移 ledger 与应用版本已记录。
- `pnpm drill:fresh-environment`、性能基准、安全验收、完整测试、构建和 strict OpenSpec 校验通过。
- Web/Worker readiness 与指标正常，日志无秘密或高基数原文。
- 功能开关按顺序启用，Tasks/Copilot 降级与设备急停演练通过。
- 现场操控责任人、批准引用、失联处置和回滚负责人已确认。

## 9. 全新环境演练记录

2026-09-01（Asia/Shanghai）在候选工作区 `d064286` 加当前 OpenSpec 实现变更上执行 `pnpm drill:fresh-environment`，结果通过：

- `.env.example` 的 19 个数据库、认证、日志、对象存储、算法、callback、媒体和 FlightHub 变量均存在；`AI_PROVIDER`、`AI_MODEL`、`OPENAI_API_KEY` 均不存在，AI Provider 只通过管理后台和数据库配置。
- 19/19 项运行配置、对象存储、算法出站、AI Provider registry 与依赖降级契约测试通过。
- 全新 PostGIS、现有快照、旧版基线和重复执行迁移通过，当前 53 个迁移均由迁移器管理。
- Next.js 生产构建、TypeScript 检查、静态页面生成和 Go Worker 二进制构建通过。

该记录只证明当前候选工作区的全新环境契约。每个后续候选版本仍必须重新执行 `pnpm drill:fresh-environment`、`pnpm test:migrations`、`pnpm check` 和生产构建，并保存实际输出；不得沿用旧迁移数量或旧 AI 环境变量检查结果。

普通 Dock 直播在 `live.control` 开启且成员权限、连接器和设备状态满足要求时可直接尝试启动，不依赖历史 `field-write` 验收记录。是否启动成功由司空接口和后续直播状态确认；其他设备类型及飞行控制沿用原有验收与审批要求。
