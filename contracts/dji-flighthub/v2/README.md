# DJI 司空 2 OpenAPI V2 合同（中国大陆公有云）

本目录保存 AeroSight `dji.flighthub2@1.0.0` 连接器使用的脱敏上游合同。合同覆盖官方 released 目录中的 89 个接口，但“进入合同”和“生产可写”不是同一件事：只读能力通过安全探测启用，所有写操作仍需项目权限、能力证据、功能开关、审批/二次确认和现场验收。

目录不得包含可用 Token、完整 SN/UUID、真实坐标、签名 URL、组织名或项目名。

## 合同入口

- [endpoint manifest](./endpoints.tsv)：89 个 released 接口的 method、path、scope、risk、分页、部署和版本化验证级别。
- [能力覆盖表](./CAPABILITY-COVERAGE.md)：按领域说明本地实现、证据、默认门禁和已知限制。
- [fixture](./fixtures/)：成功、空状态和业务失败的脱敏响应样例。
- [合同锁](./contract-lock.json)：用于检测官方 method/path/schema 漂移。
- [平台复用清单](./REUSE.md)：说明司空资源如何复用 AeroSight 通用模型。
- [司空专项运维手册](../../../docs/operations/dji-flighthub-runbook.md)：部署、限流、排障、轮换、回滚和现场验收步骤。
- [脱敏验收证据](../../../openspec/changes/expand-dji-flighthub-openapi-platform/acceptance-results.md)：真实只读、状态回填、目录对账和故障矩阵结果。

运行 `pnpm check:flighthub-docs` 可检查文档链接、manifest 总数/方法统计、必填字段和领域覆盖，防止出现没有能力归属的孤立接口。

## 当前覆盖基线

合同于 2026-08-30 对照官方 released 目录建立，包含 89 个接口：59 GET、19 POST、6 PUT、5 DELETE。2026-09-02 已使用本地连接执行显式 opt-in 的 59/59 GET 脱敏 smoke，并将账号作用域的结果写入 capability evidence；该结果没有创建任何 `field-write` 证据，也不会打开写能力。

验证证据分四级：

1. `documented`：已进入版本化官方合同。
2. `fixture`：脱敏 fixture 和本地测试通过。
3. `live-read`：固定官方主机上的无副作用真实 GET 已执行，结果只保存类别、计数/字段集合和耗时。
4. `field-write`：在批准窗口、测试资源和观察员条件下逐 action 完成现场写入；证据按账号，必要时还按型号和固件绑定。

证据只能收窄能力，不能越级扩张。`HTTP 200` 只代表请求到达网关；只有业务 `code`、异步任务/命令状态及物模型证据共同满足时，才能声明最终成功。

## 部署目标与主机边界

- 区域：中国大陆公有云。
- 固定 API origin：`https://es-flight-api-cn.djigate.com`。
- 传输：仅 HTTPS；origin 由部署配置持有，不接受浏览器、连接器凭据或上游响应传入任意绝对 URL。
- 自动重定向：禁止。
- 临时上传、下载、直播和模型链接：必须再次通过用途、协议、主机和有效期策略，不得永久明文保存。

未认证调用 `GET /openapi/v2.0/system_status` 时，网关可能以 HTTP 200 返回业务认证错误。这是同时校验 HTTP 状态、业务码和响应 schema 的直接原因。

## 鉴权与项目作用域

`X-User-Token` 是组织级 OpenAPI 密钥，不是 OAuth 授权码。组织管理员在司空 2 的“我的组织 → 组织设置 → OpenAPI”取得并复制密钥；当前集成没有 DJI 托管的跳转/callback 授权流程。

每次请求发送：

- `X-User-Token: <redacted>`
- `X-Request-Id: <每次尝试新建的 UUID>`
- `X-Language: zh`

项目作用域请求还发送 `X-Project-Uuid: <project-uuid>`。Token、项目 UUID、完整设备 SN 和临时凭据不得进入 URL、query、普通日志、审计输入、指标标签、fixture 或源码。

项目发现 `GET /openapi/v2.0/project` 只发送组织 Token，不发送项目 UUID。AeroSight 使用 `usage=complete`、`sort_column=create_time`、`sort_type=desc`、从 1 开始的 `page` 和显式 `page_size=20`；客户端拒绝重复页并执行页数、响应大小上限。设备目录 `GET /openapi/v2.0/project/device` 当前最多返回 1000 个拓扑项；恰好达到上限且无法证明完整时失败关闭，不处理 missing。

## 错误、限流与一致性

| 类别 | 常见信号 | 行为 |
| --- | --- | --- |
| 凭据无效 | HTTP 401、业务认证码 | 停止自动重试，保持历史只读并要求轮换凭据 |
| 作用域禁止 | HTTP 403、权限业务码 | 标记 `scope_forbidden`，不伪装为设备离线 |
| 参数/上下文缺失 | 例如 `200610` | 标记 `configuration_required`/`unverified`，不判定 Token 失效 |
| 合法空状态 | 例如 `231011` 或空列表 | 标记 `empty`，不降低整体连接健康 |
| 限流 | HTTP 429、`Retry-After`、限流业务码 | 尊重 `Retry-After`，执行有界抖动退避 |
| 暂时故障 | HTTP 5xx、网络超时、暂时性业务码 | 有界重试；资源流单独退避，不阻塞位置流 |
| schema 不兼容 | 缺少必需字段、响应超限 | 隔离本批次并保留上次完整快照 |
| 写入结果未知 | 响应丢失或仅同步受理 | 先查询远端状态；不得盲目重发或误报物理成功 |

官方公开资料未给出稳定的全局数值配额。AeroSight 使用保守的连接器级 token bucket、最大并发、分页上限和分资源流水位，并把真实 429/`Retry-After` 视为权威信号。

## 安全启用边界

- GET probe 只能调用 manifest 中声明为无副作用的 released GET。
- 读取能力是“官方合同 ∩ 部署/区域 ∩ 账号响应 ∩ 本地实现/证据”的交集。
- 写能力默认关闭，必须有当前账号的 `field-write`；设备动作还必须匹配当前型号和固件。
- DELETE、设备迁移、RTK、对频、组织权限和物理控制要求独立功能开关、owner/admin、审批/二次确认及现场观察员。
- 连接器禁用或断开后停止新绑定、同步和下行；历史数据保持只读。
- 未完成坐标基准验收的数据保持 `coordinate_reference=unverified`，不得用于控制预检。

## 官方资料

- [司空 2 公有云 OpenAPI V2 用户手册](https://fh.dji.com/user-manual/cn/custom-development/open-api/public-cloud-v2.html)
- [官方 Apifox 合同](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a)
- [官方 OpenAPI V2 示例仓库](https://github.com/dji-sdk/FlightHub-2-OpenAPI-V2-Demo)
