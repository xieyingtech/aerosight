# 实施证据

当前仍处于迁移阶段；默认前端启动入口尚未切换为静态导出。未勾选任务仍需实现和验证。

## 2026-09-06


- 实时前端接线：设备命令/直播启动、频道启动、直播停止、播放授权/下载、快照刷新和回放统一使用 Go API transport，保留原 JSON/取消信号和同源媒体行为；网络/解析失败恢复操作 busy 并给出状态提示，不自动重放写请求。实时选择更新保留 projectId 和其他筛选，只替换 deviceId/streamId，新增重复参数及清空选择回归。历史媒体按项目/资源 key 隔离，避免切换时复用旧 URL；刷新失败明确提示仍显示上次快照。删除无调用方的旧事件操作/草案按钮，当前历史事件页继续只读，Go 对应兼容 API 保留。TypeScript 和全部 293 项 Web 单元测试通过；实际 SSE/代理/Range/生产浏览器联调仍在第 6/8 节，未以单元测试替代。


- 前端工作台迁移：算法与连接器页采用固定路径，移除最后两个 projects/[id] 页面及布局。算法页从项目角色/显式权限决定是否读取服务配置，目录使用 Go definitions envelope，保留普通成员查看运行/目录；服务配置写后重新加载，算法启动跳转固定详情。连接器从 Go 聚合读取连接/发现身份/同步记录，功能开关来自 Go features；DJI 与 FlightHub 写请求使用 CSRF transport，保留结构化错误和向导本地状态，DJI 网络/解析异常恢复 busy。更新旧目录断言为固定路径，导航测试从真实链接验证页面存在及 projectId，连接器测试保留成员不渲染管理工作台边界。Next typegen、TypeScript 和全部 292 项 Web 单元测试通过。尚有实时组件 transport、全站旧链接、旧 API/服务端依赖移除与生产构建/浏览器验收，第 6 节继续未勾选。


- 前端详情迁移：任务运行、案件、算法运行和历史事件详情采用固定 detail 路径 + projectId/资源 ID，统一 Suspense、参数验证和 Go API 加载。算法 ID/历史事件 ID 按 UUID 校验，数据库日期按 JSON 字符串显示，算法详情类型明确不包含私有 inputSnapshot。任务控制与案件协作通过 CSRF 客户端写入后重新读取数据，保留 expectedVersion、独立权限和错误提示；算法重试跳转固定新运行地址。证据预览改用可取消的共享 API hook，避免切换资源时展示上一个资源的 URL；旧事件列表入口客户端转至案件列表并保留查询参数。Next 路由类型生成、TypeScript 及 API/任务权限/算法诊断/案件证据 13 项测试通过。剩余算法/连接器工作台、全站旧链接兼容与浏览器生产验收未完成，第 6 节保持未勾选。


- 前端接线补齐素材列表遗漏：GET /api/projects/:id/assets 使用 sqlc 显式投影，保持旧 listProjectItems 的 available 筛选、创建时间倒序、数值 ID、nullable MIME/采集时间及 ISO 时间；不返回存储路径。真实 PostGIS TestProjectAssetListContract 验证空数组、筛选/排序/DTO、项目隔离和撤权 404。Go 非数据库全包、db:check 和 OpenSpec strict 通过。任务、案件、素材库和设置页面改为静态查询参数入口，保留表格和管理权限提示，任务模板与运行列表分别加载；Next 路由类型生成和 TypeScript 检查通过。详情链接已使用目标固定路径，详情页和浏览器联调仍待完成，第 6 节保持未勾选。


- 前端静态化第一批：登录/退出、公共会话布局、团队/项目列表与创建、团队/项目概览、平台管理和个人资料改为浏览器加载 Go API；统一加载、错误重试和访问拒绝状态。团队/项目详情及设备、智能体、实时作业采用固定路径和查询参数，查询参数读取包在 Suspense 中，侧栏项目切换与高亮适配固定路径。智能体和 AI provider 写操作使用 CSRF 客户端并在成功后重新请求数据。Next 开发 phase 添加 beforeFiles API/算法资产 rewrites，生产 export 尚未启用。next typegen、TypeScript 和 11 项 API/导航测试通过；清理了引用已删除页面的旧 .next/dev/types/validator.ts 生成缓存。剩余项目工作台、详情、旧地址兼容、浏览器联调和生产构建仍待完成，第 6 节保持未勾选，不能视为完整静态站已可部署。

- 第 6 节第一批：新增浏览器同源 API transport、typed JSON 错误、CSRF 合并获取/失效、可选认证与受保护 401 通知、登录/退出/会话方法；禁止外部/归一化后越出 /api 的路径，不自动重试业务写入，保留 Response 用于 Range/流式消费。新增 useAPI 和统一加载/失败显示，切换路径或刷新时不展示旧项目数据，卸载取消请求。7 项 transport 测试验证并发 CSRF、Header/凭据、401/403、取消隔离、不重放写入、路径限制及 HTML 错误；全项目 typecheck 通过。Next.js 静态导出限制已用 Context7 /vercel/next.js 核对。此批为页面迁移基础，尚未把旧登录/SSR 页面切到 Go 会话，也未启用 export 或完成双入口验收，6.1/6.2 保持未勾选。

- 完成 5.1 当前入口清单：复核原 4 个 Server Actions（登录/退出已由 4.4 接管，业务写入为创建团队/项目），个人资料及平台用户/团队/项目页面只有只读入口，不新增不存在的编辑功能。现有 Go/sqlc 目录 API 已覆盖全部目标；修正创建名称为 trim 后 UTF-16 最大 100，并接受项目表单的字符串 teamId。真实 PostGIS 验证空列表、团队创建/owner、项目创建和自动 Copilot、资料及管理员投影不泄露密码、非法输入、owner 插入失败时团队回滚、owner/admin 允许建项目/member 和已移除成员拒绝、平台 admin 不绕过团队管理权限、scope 列表及团队/项目/平台权限拒绝。创建继续保留原无业务审计契约，项目写入前在同一事务内锁定 manager 成员记录。Go 非数据库全包、sqlc 和 OpenSpec strict 通过。Server Action 的 Go 替代入口已就绪；第 6 节负责前端调用切换和删除原 actions，尚未宣称静态前端构建完成。

- 完成 5.11：复核 cmd/aerosight 已把 runtime.Callbacks 接入 Gin 同一端口，并给算法回调/资产组补 context deadline，继续使用原独立机器认证（不经过 SCS/CSRF）。修复回调 LimitReader 静默截断，实际正文超过 16 MiB 明确返回 413；算法资产查询排除 deleted_at 非空记录。真实 PostGIS 的统一 HTTP 集成测试通过无 Cookie/CSRF 外部签名回调、processing→completed、重复回调只存一次原始结果、同 callback ID 不同正文 409、过期/跨项目 401、超大正文 413；资产签名有效读取、篡改项目/过期 403、签名有效但项目或版本不匹配 404、软删除 404，未授权请求不触发存储读取。已有算法签名/状态机单元测试、Go 非数据库全包、sqlc 和 OpenSpec strict 通过。部署公共地址切换与生产端到端仍由第 7/8 节完成。

- 完成 5.9 当前入口清单：聊天 POST 接入官方 openai-go/v3 Responses API；保留配置模型/base URL/数据库密钥、中文系统提示、最近 20 条历史、最多 8 次模型调用及六个只读工具。无 Node sidecar；请求关闭自动重试和上游 store，逐轮带回 output message phase、加密 reasoning 与 function_call_output，每次模型/工具调用前复查 agent:use 和 open 会话，工具内再校验项目范围；最终只落库脱敏回答/精简证据。独立 AI_REQUEST_TIMEOUT 默认 120s，错误返回安全代码，响应正文上限 4 MiB，失败只保留原先已提交的用户消息，不伪造助手回复；达到八步无文本时保留原存储 fallback 和空 content 返回行为。真实 PostGIS + SDK 假上游覆盖完整两轮六工具 Responses 请求与回传/20 条历史/推理续传/模型 ID/最终证据、八步、500/超大正文/超时、非法输入/无 provider/关闭会话/越权参数/未知写工具/中途撤权。全量 Go（真实 DB）、13 项原 TS 相关测试、sqlc 和 OpenSpec strict 通过。第 6 节仍负责客户端静态化及 TS 服务端依赖移除，第 8 节负责生产单二进制端到端。

- 5.9 第四批迁移：六个聊天只读工具的 sqlc 查询、严格输入校验、递归 scope 注入拒绝、只读快照权限复查及结果格式接入 Go。保留设备/任务/案件/可用资产/轨迹/地图计数与质量字段、数据新鲜度、100 条/64 KiB 上限；证据链接直接采用设计中的固定页面查询参数，数值 ID 引用避免科学计数法。修复旧 query_issues 的 limit 与底层 query_events schema 不一致问题，设备 ID 筛选和任务/案件 limit 在查询中生效；检测关联字符串拒绝超出 bigint 范围，避免坏链接破坏案件查询。真实 PostGIS 覆盖六工具空值/两项目隔离/撤权拒绝、设备筛选、案件 limit、仅 available 资产且不泄露 storage_key、两点 LineString 和项目计数；格式测试覆盖嵌套注入、写工具拒绝、引用 URL 编码、记录/字节上限及新鲜度。原 TS 3 项、会话集成回归、Go 非数据库全包、sqlc 和 OpenSpec strict 通过。Responses 编排尚待接入，5.9 保持未勾选。

- 5.9 第三批迁移：会话创建 POST、用于替代 SSR 的会话列表 GET，以及聊天内部消息追加/最近历史查询迁入 Go/sqlc。列表在同一只读快照中校验 agent:use，仅返回当前用户最新 50 个会话和对应消息；追加在审计事务中锁定用户的 open 会话并重新授权；历史限定最新 20 条 user/assistant，再按 ID 正序。复用原最小留存规则：临时 URL/API key/Authorization 脱敏（含 JS Unicode 空白字符）、UTF-16 长度限制、最多 50 工具记录及每条 100 个证据引用，不保留原始参数/结果和临时引用 URL。真实 PostGIS 测试覆盖空数组/null/ID/时间、跨用户/跨项目、关闭/撤权拒绝、列表/历史上限及插入故障下消息和审计整体回滚；原 TS 2 项、Go 非数据库全包、追加脱敏单元测试、sqlc 和 OpenSpec strict 通过。消息追加是供下一批编排调用的内部方法，尚未把未完成的聊天 POST 暴露为成功；Responses 循环和六个工具仍待实现，5.9 保持未勾选。

- 5.9 第二批迁移：AI provider test 使用固定的官方 openai-go/v3 Models.List，关闭自动重试，保留原 GET /models、10 秒上限和 HTTP 状态健康结果；只检查状态并关闭原正文，不缓存或泄露上游错误内容。自定义客户端固定已校验的公网 DNS 地址，禁用代理、保留原主机名 TLS 校验并拒绝重定向；provider 行锁、管理员角色锁与平台审计覆盖健康写入。修正创建缺密钥错误码为原 AI_PROVIDER_API_KEY_REQUIRED。SDK 假上游验证 200/204/401/429/500 单次请求、请求密钥/路径、deadline、取消和不读取正文；真实 TLS 验证固定地址连接、目标变化拒绝和重定向拒绝；真实 PostGIS 验证健康落库、审计、私网拒绝及管理员权限。全量 Go（真实 DB）、sqlc 和 OpenSpec strict 通过。SDK 文档经 Context7 /openai/openai-go 核对。聊天会话、Responses 循环和六个只读工具尚待实现，5.9 保持未勾选。

- 5.9 第一批迁移：平台 AI provider 列表、创建、PATCH、DELETE 接入 Go/sqlc。保留 OpenAI 类型、默认项必须启用、空密钥保留原 envelope、替换密钥重置 untested、平台 AAD 和 write-only 凭据、bigint 字符串/null/时间格式；复用 HTTPS 与全部 DNS 地址校验。注册表事务锁序列化默认切换，用户角色行锁完成写入前再授权，业务和平台审计整体提交。真实 PostGIS 测试覆盖 4 并发默认创建、密文兼容解密、空白更新、加密失败时默认项/审计回滚、删除不存在项及 URL 校验期间撤权；相关算法 provider 回归、Go 非数据库全包测试、原 TS 2 项、sqlc 和 OpenSpec strict 通过。官方 SDK 测试端点、聊天会话和六个只读工具尚待实现，5.9 保持未勾选。

- 完成 5.8 当前入口清单：直播启动迁入 Go/sqlc，锁设备后校验在线/可用能力/显式授权、选择频道、同频道重放和驱动并发额度；DJI 拓扑生成 video_id，验证适配器推流凭据，只把无密码的服务端 RTMP 目标写入命令，原子提交会话/命令/outbox/项目事件/审计。保留现有 Route Handler 不接收 taskRunId 的行为；模拟器直接 live，DJI requested。启动/停止统一 device→session 锁顺序。真实 PostGIS 覆盖 4 并发只建一次、第二频道额度、离线/无能力/缺频道/拓扑/凭据拒绝、同频道重放前再授权、DJI 参数/优先级、outbox 失败全回滚、并发启动/停止及跨项目。全量 Go（真实 DB）、额外并发测试、原 TS 13 项、sqlc 和 OpenSpec strict 通过。媒体 Linux 补验使用本地 Go 交叉编译测试二进制，在 bookworm 容器实际执行 TestMediaProjectFileBoundary，符号链接逃逸分支通过（无 skip）；此前 Windows 权限限制的补验已完成。前端切换与生产端到端仍由第 6/8 节覆盖。

- 5.8 第四批迁移：停止直播 POST 接入 Go/sqlc；事务内复查 mission:operate 并锁会话，保留 DJI stopping/45 秒租约/优先级 30 停止命令、模拟器及失败会话直接 stopped、重复停止重放和 replay 模式拒绝。命令冲突返回实际 ID，修复旧逻辑派发不存在 UUID 的边界；命令/outbox 与状态/审计原子提交。真实 PostGIS 验证 4 并发仅一条停止命令/派发事件、已有命令不重复派发、模拟器与 failed 行为、outbox 故障回滚、跨项目/权限拒绝，并重跑播放及媒体鉴权测试；Go 非数据库全包、sqlc 和 OpenSpec strict 通过。直播启动仍待迁移，Linux 符号链接补验仍待执行，5.8 保持未勾选。

- 5.8 第三批迁移：浏览器直播 playback API 接入 Go/sqlc；在事务内锁授权与会话，复核 stream.* 能力显式 deny/allow，签发 60 秒候选并更新 locator 期限。保留 requested/stopped/无 ref/无协议原因、模拟器 locator、WebRTC→HLS 顺序、数值 session ID 与 null 字段；模拟器固定签名与原 TS 字节一致。真实 PostGIS 测试验证过期 locator 刷新、签发 DJI token 通过 media-auth、owner 显式 deny、成员独立能力授权、停止后不可用、跨项目与撤权拒绝；Go 非数据库全包、sqlc 和 OpenSpec strict 通过。直播启动/停止仍待迁移，5.8 保持未勾选。

- 5.8 第二批迁移：media-auth 机器接口与播放 token 签发/验证核心。按原接口区分管理员 API、绑定活动 live_stream 的适配器推流凭据、read/playback 的路径/协议/期限 token；额外 MediaMTX 字段不影响解析，不依赖 Cookie/CSRF，也不创建浏览器会话。保留 MEDIA_ADMIN_USER/PASSWORD 原字符串；加密凭据沿用 device-adapter AAD。固定时间 token 与原 TS 字节一致。真实 PostGIS 测试覆盖管理员凭据、推流状态/路径/错误密码/错误 AAD、query token、协议/路径越界与过期，重跑资产访问集成测试；Go 非数据库全包、原 TS 13 项直播核心测试、sqlc 与 OpenSpec strict 通过。直播启动/停止及浏览器播放地址签发 API 仍待迁移，5.8 保持未勾选。

- 5.8 第一批迁移：媒体 access/content 接入 Go/sqlc。签发与读取分别复核项目和敏感下载权限，已发布 evidence_links 也触发敏感规则；敏感下载签发使用事务审计，签名失败回滚审计。HMAC 与原 TS 固定样本逐字节一致，保留 120 秒 TTL、动作/项目/资产绑定及 NFKC 下载名。内容通过限定项目根的文件句柄流式输出，拒绝路径穿越/反斜杠/目录别名/符号链接逃逸，并支持 Range、HEAD、私有 no-store。真实 PostGIS 测试覆盖全文/部分与后缀范围/416/HEAD、过期和动作篡改、敏感下载/权限别名/审计回滚/撤权后旧签名拒绝/跨项目；原 TS 3 项测试、Go 非数据库全包测试、sqlc 与 OpenSpec strict 通过。Windows 无创建符号链接权限，该分支测试未执行，需在 Linux 生产回归中补验；词法路径拒绝已验证。直播启动/停止/播放及 media-auth 仍待迁移，5.8 保持未勾选。

- 完成 5.7 当前入口清单：provider 列表/创建/PATCH/测试已接入 Go/sqlc，配置读取原 ALGORITHM_ALLOWED_HOSTS。保留 write-only 加密凭据、空白更新保留 envelope、认证类型切换必须提供匹配凭据、默认 disabled、不支持的 adapter 显式拒绝；测试端点按原逻辑只验证适配器与 HTTPS/allowlist/全部 DNS 地址，不虚构远端推理探测。复用 Go 能力表与原凭据 AES-GCM/AAD；写事务在 DNS 完成后再授权。真实 PostGIS 覆盖创建/更新/密文兼容解密/空白保留/认证切换/跨项目/无密钥回滚/校验期间撤权，单元测试覆盖通配主机、混合地址与 IPv4-mapped 私网拒绝；全量 Go（真实 DB）、原 TS 11 项相关测试、sqlc 和 OpenSpec strict 通过。算法 provider/definition/run/retry 业务入口完成；前端调用切换和最终端到端仍由第 6/8 节完成。

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
