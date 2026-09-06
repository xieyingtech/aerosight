# 实施证据

当前仍处于迁移阶段；默认前端启动入口尚未切换为静态导出。未勾选任务仍需实现和验证。

## 2026-09-06

- 5.7 第二批迁移：算法定义 POST/PUT 与动态目录 GET 接入 Go/sqlc。配置保留通用 JSON Schema、configuration/version 输入兼容、默认映射/显示元数据、阈值校验和 UTF-16 文本长度；事务锁 provider/definition，分配递增配置版本并原子退役旧快照、发布新快照、更新当前指针及审计。保留创建返回 bigint 字符串、更新返回数值 definitionId 的原端点差异。真实 PostGIS 测试覆盖目录空数组/null、禁用 provider 可见性、4 并发保存版本连续且仅一个 published、插入快照失败回滚创建/改名/退役/审计、跨项目及撤权拒绝，并重跑算法运行集成测试。对应 TS 3 项测试、Go 非数据库全包测试、sqlc 漂移和 OpenSpec strict 通过。Provider 管理与测试接口仍待迁移，5.7 保持未勾选。

- 5.7 第一批迁移：算法运行创建、列表、详情/attempt 和失败重试接入 Go/sqlc。创建在事务中锁定当前发布配置、启用 provider 和可用资产，固定 SHA-256/版本/参数，并原子提交运行、审计和请求事件/outbox；重试保留源配置与参数，只允许 failed/timed_out。修复旧 TS 重试复制旧 runId 导致 Go processor 拒绝快照的问题：重试使用新 runId/请求上下文，清除旧 callback 和资产签名，worker 再签发。原 SSR 页面只展示安全诊断；新浏览器 API 不返回私有 inputSnapshot，保留安全 view 和标准化结果。真实 PostGIS 测试覆盖创建、worker Input 解码及范围一致性、详情/attempt/null、输入别名、跨项目、禁用 provider、缺 checksum、撤权、重试及 outbox 故障全回滚；对应 TS 4 项测试、Go 非数据库全包测试、sqlc 漂移及 OpenSpec strict 通过。Provider/definition 配置管理尚待迁移，5.7 保持未勾选。

- 完成 5.6 当前入口清单：案件 actions 的 comment/status/labels/assign/unassign 已迁入 Go/sqlc，保留 clientKey 重放、版本冲突、负责人 no-op、UTF-16 长度限制与可见 Copilot 提及规则。事务内重新读取受锁保护的权限，作用域校验负责人，并原子提交案件版本/活动/负责人/Copilot 会话和任务/审计/项目事件；issue.updated 不加入 outbox。真实 PostGIS 测试覆盖开关状态、标签去重、负责人及 Copilot、无 agent:use 时普通评论、权限别名、撤权后重试拒绝、跨项目、4 请求同键只执行一次及末端事件失败整体回滚；全量 Go（真实 DB）、原 TS 9 项对应测试、sqlc 漂移与 OpenSpec strict 检查通过。复核 app/API/组件调用：旧事件 actions/agent-drafts 原本固定 410，generateOnDemandAlertDraft 和 AgentEventDraftButton 没有应用调用；案件草稿由现有 Go issue_copilot 后台链路生成并由已迁移详情读取，不新增未暴露的 TS 草稿工具端点。前端切换及端到端验收仍由第 6/8 节完成。

- 5.6 第二批迁移：案件列表和完整详情读取，包含活动/链接/检测/资产/有效负责人/项目成员/活动 Agent/关联草稿。sqlc 保留原查询排序、资产筛选、非法数字检测链接处理及 bigint 字符串，在同一只读快照中完成。真实 PostGIS 测试覆盖空关联集合、系统活动、直接关联资产、负责人及草稿、无地图坐标、跨项目拒绝和 event:handle → issue:handle 权限别名（不扩展指派/Agent 权限）；sqlc 和 Go 非数据库测试通过。案件协作写入与草稿操作尚待迁移，5.6 保持未勾选。

- 5.6 第一批迁移：旧 perception event 详情只读 API，保留检测证据投影、位置说明、历史反馈、ID/时间序列化及原有说明文字。核对实际 Route Handler 后保留 actions/agent-drafts 的 410 LEGACY_EVENT_READ_ONLY；没有复活未被调用的 handlePerceptionEvent 写入服务。真实 PostGIS 测试验证详情空证据/反馈、group bigint 字符串、跨项目/撤权拒绝，两个废弃写接口不改变事件状态或写审计。原 TS 证据显示两项测试、sqlc 检查和 Go 非数据库测试通过。案件读取/协作/草稿仍待迁移，5.6 保持未勾选。

- 完成 5.5 当前入口清单：新增任务模板列表/详情 Go 查询，测试空数组、nullable description、触发类型、跨项目及成员删除后拒绝。核对 app 页面/Route Handler/Server Actions：任务定义只有读取入口；task-versions.ts 的 createTaskDraft/publishTaskDraft/listTaskVersions 无应用调用，没有把未暴露的 TS 工具函数新增为公开 API。现有任务运行控制、工作台、审计、演练与报告入口已迁移，后台调度/执行继续复用 Go。全量 Go 真实 PostGIS 回归、sqlc 和 OpenSpec strict 通过。前端调用切换及旧 TS 清理继续由第 6 节完成，完整端到端验收仍由第 8 节覆盖。

- 5.5 第五批迁移：报告草稿聚合创建。单 SQL 快照读取任务/版本/设备、步骤、窗口内轨迹、关联事件与反馈、有效资产，生成缺口和可追溯结论；新草稿时间值及时间证据版本沿用 Go ISO UTC 规范，已有报告版本不改写。事务内复核权限及任务状态版本、锁报告记录分配递增版本、退役旧草稿，并原子写入证据/审计。真实 PostGIS 测试覆盖非终态拒绝、缺口/资产证据、证据写入故障回滚旧草稿退役与新版本、重复生成递增版本、跨项目拒绝；聚合测试覆盖全部八类证据及人工结论。sqlc 检查和 Go 非数据库测试通过；完整正向轨迹/事件数据集的端到端验收仍归入 8.1。任务定义尚待迁移，5.5 保持未勾选。

- 5.5 第四批迁移：报告发布与 JSON 附件导出接入 Go/sqlc。发布锁定报告及草稿，事务内校验 mission:operate、完整性、版本切换、证据资产去重保留及审计；导出在只读快照内校验独立 report:export，仅导出当前发布版本，保留文件名/类型/private no-store。真实 PostGIS 测试覆盖草稿不可导出、不完整确认、failed 禁止发布、资产保留失败全回滚、重复发布、旧版本退役与最新版本导出、跨项目及独立导出权限；sqlc 检查、原 TS 四项报告测试和 Go 非数据库测试通过。报告草稿聚合创建及任务定义尚待迁移，5.5 保持未勾选。

- 5.5 第三批迁移：GET task-runs 列表和单次运行工作台，使用 sqlc 显式 JSON 投影及共享只读快照事务。保留时间/ID/null/空步骤、最新命令及操作顺序，分别计算 mission:operate 与 mission:approve。真实 PostGIS 测试覆盖运行列表、无关联设备、无步骤、关联版本/设备/步骤、最新命令、成员无控制按钮、独立审批权限与跨项目拒绝；sqlc 检查和存量 Go 非数据库测试通过。任务定义与报告尚待迁移，5.5 保持未勾选。

- 5.5 第二批迁移：任务 audit-trace 与 emergency-stop-drill 接入 Go/sqlc。审计查询在只读 Repeatable Read 事务内鉴权和读取请求/预检/审批/命令最新尝试，保留 bigint 字符串、null/空数组、缺失项顺序及最后一条高优先级命令的安全状态。演练只允许 dryRun，使用事务审计并保留 ACK/NACK/timeout/disconnected 结果；真实 DB 验证五次演练不生成设备命令/项目事件/outbox、不修改任务状态和版本，拒绝越权；审计集成测试覆盖空轨迹、agent 来源、最新尝试、策略 ID 类型与跨项目拒绝。Go 单元测试覆盖审批未完成、缺少尝试及缺少能力时不虚报确认；原 TS 六项审计演练测试通过。任务定义、工作台与报告仍待迁移，5.5 保持未勾选。

- 5.5 第一批迁移：任务控制 POST 接入 Go/sqlc，保留 pause/resume/cancel/emergency_stop/approve、乐观版本、排队取消、取消中重复急停及原事件类型；写事务内复查独立的 mission:operate/mission:approve 权限。真实 DB 测试覆盖 4 并发同版本仅一次生效、跨项目、撤销操作权限、状态转换拒绝、outbox 故障回滚状态/版本/审计/事件、审批不推进任务版本、不产生队列副作用，以及数据库审批分离/过期/重复审批约束。任务定义、工作台、审计轨迹、急停演练和报告尚待迁移，5.5 保持未勾选。

- 完成 5.3 剩余连接测试和 DJI 设置 API。Go 网络校验保留 LAN/public 策略、全 DNS 结果校验、五类协议、服务端验证与设备待验证区别；实际探测固定已校验 IP、HTTP HEAD 不跟随重定向、TLS 校验证书与主机、5 秒探测上限受请求取消控制。单元测试覆盖混合私网/环回 DNS、IPv6、脱敏、HTTP Host/IP 固定、重定向、503、无效证书及取消释放连接。真实 PostGIS 测试验证 DJI 网络/适配器/凭据原子创建、六个 MQTT topic、兼容 envelope、配置占位符、有效/失败健康状态、模拟器与无网络配置、写失败整体回滚及探测期间撤权。原 TS adapter-policy/network-profile/connection-check 共 16 项测试通过；全量 Go（真实 DB）、sqlc drift 与 OpenSpec strict 通过。设备 API 清单已覆盖；前端移除旧 Route Handler 属于 6.6，尚未执行。

- 设备适配器列表、创建、启停、DJI 凭据更新及发现绑定已迁入 Go/sqlc。保留 ID 字符串/数值的原端点差异、null/空列表、空白凭据不修改、密码原值、重复绑定及能力版本递增语义；权限在写事务内复查，凭据沿用原 AES-GCM/AAD。真实 PostGIS 测试覆盖 4 请求并发绑定仅创建设备一次、未知类型回退、跨项目/非管理成员拒绝、能力写入故障回滚、凭据兼容解密与错误作用域拒绝、嵌套 config 密钥拒绝、无加密密钥时创建及审计整体回滚。适配器连接测试与 DJI 设置仍待迁移，5.3 保持未勾选。

- 设备命令 API 已迁入 Go：保留原命令账本幂等语义、显式 deny 优先、在线与任务冲突检查、高风险确认、返航优先级和队列事件。使用事务内最新成员角色，并锁住设备/能力/授权记录。真实 DB 测试覆盖确认、离线、任务冲突、返航例外、重试返回原命令、重试前撤权、跨项目、回放拒绝、输入校验、outbox 故障整体回滚；4 个并发同键请求只生成一条命令和一条 dispatch 事件。5.3 的适配器配置/发现绑定/连接测试/DJI 设置尚待迁移，该任务保持未勾选。

- FlightHub HTTP 迁移：上游项目发现/校验、连接器创建/列表/发现活动、token 更新、同步与断开。复用官方中国区 Go 客户端、原 AES-GCM envelope 和 worker token resolver；配置保留项目翻页上限。真实 DB 测试覆盖重复创建回滚、同步去重、token 解密兼容、断开禁用绑定/取消待执行和运行中同步/保留成功历史、跨项目 ID 拒绝、管理角色限制、上游期间撤权、输入严格校验、上游凭据错误、发现限流和失败审计。审计不包含 token。模拟 HTTP transport 验证官方 origin 与 X-User-Token 请求契约。
- 修复取消 pending 同步的数据库约束冲突：没有 started_at 的待执行记录取消时同时补齐 started_at 和 finished_at；已开始记录保留原 started_at。此路径已在断开集成测试覆盖。

- 业务写边界新增 `AuditedWrite`、`AuditedPlatformWrite`、事务内 `Idempotent` 和 `Publish`；生成查询通过 WithTx 共享原始事务。真实数据库故障注入覆盖业务、项目事件、outbox 插入、幂等完成、审计完成失败时所有记录回滚。8 个并发同键请求仅执行一次；不同请求复用键拒绝；撤权后不返回历史幂等成功。
- `audit-hashes.json` 从原 TS 实现生成，记录采样 Node 的 zh-CN locale。覆盖嵌套/空值、Unicode、HTML 字符、整数属性名、大数字及字面转义。新哈希固定 English 排序；旧幂等哈希匹配支持库提供的语言排序，测试验证中文旧哈希仍能重放相同请求。

- 原 `pnpm test:web`：285 项通过。目录迁移后的 `pnpm test:migrations` 重跑通过，覆盖 PostGIS、空库、已有库及重复执行。
- Go 迁移器集成测试：并发迁移锁、原始字节 checksum、历史篡改阻止待执行迁移、已有基线收养、失败 DDL 回滚。
- Go 初始化测试：重复启动保留管理员密码；sqlc 与原始 SQL 共享事务并整体回滚。
- `pnpm db:check`：通过；人为修改生成结果后检查正确失败且没有覆盖改动，恢复后再次通过。
- HTTP 数据库测试：登录 CSRF/Origin 校验、持久会话重启恢复、退出后失效、团队/项目 JSON null 与空数组、跨团队读写拒绝。
- 原 bcryptjs 生成的密码 hash 登录成功；二次登录轮换 token 并使旧 token 失效；Auth.js Cookie 拒绝；数据库会话过期后返回 401。
- 项目 SSE 集成测试：502 条事件触发 snapshot.required；Last-Event-ID 优先于 query cursor；按数值游标续传；等待期间连接池 InUse 为零；退出后 15 秒复查断流。
- 频道 SSE 集成测试：502 条样本触发 backpressure_limit_exceeded；从 501 续传第 502 条；等待期间无连接占用；新增显式 deny 后 5 秒复查断流；再次访问返回 404。
- 项目快照测试：空图层、诊断 nullable deviceId、隐藏 role、撤权 404、显式 deny、离线设备操作禁用、依赖降级；真实 PostGIS PointZ 坐标、双点 LineString 轨迹和在线新鲜度通过。
- 设备/回放测试：设备列表和树读取、循环关系终止、回放类型与 bbox 筛选、非法时间窗与 bbox、2002 条事件截断为 2000 条并标记 truncated，以及四个读取入口撤权后返回 404。
- Prometheus：原业务指标目录、标签拒绝和 gauge 语义测试通过；HTTP 测试验证匿名拒绝、Bearer 访问、路由模板标签及 Go runtime 指标。
- 当前 `pnpm check` 通过：TypeScript 类型检查、285 项 Web 测试、Go 全套测试（新增数据库测试使用真实独立 PostGIS 服务）。`pnpm db:check`、`go mod verify`、OpenSpec strict 校验通过。未把这些阶段性检查作为未迁移业务或生产静态构建的验收。

新集成测试使用独立 PostGIS 测试容器，通过 `AEROSIGHT_MIGRATION_TEST_DATABASE_URL` 指定测试服务，并为每项测试创建和清理随机临时数据库；未设置该变量时会跳过，不代表数据库行为已经验收。
