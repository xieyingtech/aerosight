# DJI 司空 2 OpenAPI 运维与现场验收手册

本手册适用于 AeroSight `dji.flighthub2@1.0.0` 中国大陆公有云连接器。它补充平台级[部署与现场运行手册](./deployment-runbook.md)和[凭据轮换手册](./credential-rotation.md)。接口范围与当前证据见[合同 README](../../contracts/dji-flighthub/v2/README.md)、[能力覆盖表](../../contracts/dji-flighthub/v2/CAPABILITY-COVERAGE.md)和[脱敏验收证据](../../openspec/changes/expand-dji-flighthub-openapi-platform/acceptance-results.md)。

## 1. 上线边界

- 只允许 `https://es-flight-api-cn.djigate.com`，仅 HTTPS，禁止自动重定向和租户自定义 origin。
- Web 负责鉴权、项目 RBAC 和同步数据库事务；Worker 持有最短生命周期明文 Token 并执行上游请求。
- 先部署数据库迁移，再 Worker，最后 Web。应用可回滚，数据库 schema 不回滚。
- 连接器启用不等于写能力启用。所有 action 默认关闭，真实 GET evidence 不能替代 `field-write`。
- 坐标基准未验收时只作带 `unverified` 标签的态势展示，不进入控制预检。
- 不存在 released 直播停止接口；停止意图只撤销本地播放授权，并等待物模型/离线/凭据到期等证据收敛。

## 2. 部署与健康检查

1. Web 与所有 Worker 使用相同的 `DJI_FLIGHTHUB_ENABLED`、固定 API base、超时、重试和响应大小配置。
2. 网络只放行固定官方主机 443；DNS/TLS 代理不得改写主机。响应中的对象存储/媒体 URL 由用途 allowlist 单独判断。
3. 运行 `pnpm db:migrate`，确认迁移 ledger 正常；不要手工跳过迁移。
4. 滚动启动 Worker，再启动 Web；检查 `/healthz`、`/readyz`、`/metrics` 和连接器诊断。
5. 先连接测试项目并运行一次只读 probe。检查各资源流的水位、状态、最后成功时间和退避，不只看总连接状态。
6. 发布前运行 `pnpm check:flighthub-docs`、相关 Go 测试、`pnpm check` 和 `pnpm build`。

生产日志、工单和截图不得出现 Token、credential envelope、完整 SN/UUID、坐标、签名 URL、直播 Token 或上游原始响应。只使用本地项目 ID、endpoint ID、安全错误码和不可逆账号指纹定位。

## 3. 限流与资源流

官方未公布稳定全局配额，不要根据短时成功率推断可提高频率。所有资源流共享连接器级 token bucket 和最大并发，并独立保存水位与退避：

| 资源流 | 目的 | 典型处理 |
| --- | --- | --- |
| `device-state` | 设备位置、遥测、模式和直播状态 | 在线设备优先；失败不刷新旧数据时间，也不被目录流阻塞 |
| `health` | HMS、自动录制和能力证据 | 空结果保持健康；权限失败单独分类 |
| `inventory` | 设备目录和历史拓扑 | 低频完整快照；无法证明完整时不处理 missing |
| `catalogs` | 航线、任务、地图、模型和直播目录 | 分领域分页；单域失败不阻塞其他资源流 |
| `active-operations` | 任务、直播、命令和建模对账 | 只对活动记录高频轮询，到达终态后降频/停止 |

收到 HTTP 429 或业务限流码时尊重 `Retry-After`，随后使用有界抖动退避。不要通过增开 Worker 绕过配额；Connector 租约只允许一个拥有者。分页重复、部分失败、恰好达到无游标上限或 schema 异常时保留上次完整快照。

## 4. 故障排查

按“连接器 → 资源流 → endpoint → 证据”的顺序排查：

| 安全类别/现象 | 判断与处置 |
| --- | --- |
| `credential_invalid` | Token 无效或过期；停止快速重试，按第 5 节轮换。不要在日志输出原始业务消息 |
| `scope_forbidden` | 账号/项目/组织权限不足；核对司空侧授权和所选项目，不删除本地历史 |
| `configuration_required` | endpoint 需要 workspace、设备或其他上下文；补齐安全上下文后重试，不误判 Token |
| `empty` | 合法空目录/业务空码；UI 显示空状态，整体连接保持健康 |
| `rate_limited` | 检查 `Retry-After`、attempt 和 backoff；降低轮询，禁止并发扩容规避 |
| `temporarily_degraded` | 5xx/网络暂时故障；观察独立资源流退避，确认位置流仍推进 |
| `schema_incompatible` | 停止应用本批次，保存字段名/类型摘要，比较合同漂移；不得保存原始 payload |
| 水位不推进 | 检查连接器 status、租约、outbox、resource sync state、最后错误类别和 schema 完整性 |
| 地图无位置 | 检查 `device-state` 成功水位、最新 telemetry/pose、采集时间、坐标合法性和 `coordinate_reference` |
| 命令长期未知 | 检查 command attempt、远端 cmd/task status 和新鲜物模型；转人工核查，禁止盲目重发 |
| 直播 `stopping` | 这是可能的正确状态；检查播放凭据是否撤销，以及物模型、设备离线、五分钟无观众等终止证据 |

禁止把 HTTP 200、业务受理、outbox completed 或 Worker 成功消费单独解释为飞行、直播、建模或删除已经最终成功。

## 5. Token 与主密钥轮换

### 组织 Token

1. 在司空创建/取得新 Token，保持旧 Token 可用，进入连接器“更新 Token”。
2. AeroSight 使用新 Token 重新发现项目并确认原项目仍可访问；失败时必须保留旧 envelope。
3. 验证成功后原子替换 envelope、排队同步；确认连接器恢复、只读水位推进且无秘密输出。
4. 在司空撤销旧 Token。若权限也发生变化，重新运行只读 probe；旧的 `field-write` 证据不得自动扩张到新账号指纹。

### `AUTH_SECRET`

按[凭据轮换手册](./credential-rotation.md)执行 dry-run 和事务性 re-encrypt。Web/Worker 必须原子切换同一新值；命令行不得携带密钥参数。轮换失败时保持旧值，成功后若部署切换失败应修复并使用新值，不能让新旧 Worker 混跑。

临时 STS、签名 URL、直播 Token 和模型上传 Token 不做长期轮换：过期即销毁，需要时重新申请，并重新经过用途/主机/有效期校验。

## 6. 应用回滚

1. 立即关闭司空写 feature flags 和新写请求入口；停止创建现场动作，但不要中止证据对账。
2. 盘点 `queued/dispatching/accepted/unknown/stopping` 的命令、作业和直播会话，逐项指定人工负责人。
3. 等待可安全收敛的在途请求；对物理结果未知项不得自动补发、撤销或标记成功。
4. 回滚 Worker 和 Web 到上一已验证构建。不要回滚数据库、删除新表/列、清空水位或删除审计/能力证据。
5. 运行只读 probe 和目录/状态检查；确认历史仍可读、租户边界正常，再决定是否恢复资源流。
6. 重新开放任何 action 前，确认对应 `field-write` 证据仍绑定当前账号、区域、型号、固件且未过期。

重复物理命令、租户隔离失败、秘密泄露、审计断链或错误终态是立即关闭全部司空写能力并回滚应用的条件。

## 7. 坐标基准现场验收模板

执行前需要用户提供已知控制点和同一时段任务轨迹。记录中不得写真实坐标，只保存脱敏样点编号、接口、声明基准、转换版本、误差统计和通过/失败。

| 项目 | 记录 |
| --- | --- |
| 批次/日期/时区 |  |
| 测试项目、账号指纹 |  |
| 数据接口 | device state / task track / element / flight area / model |
| 控制点与轨迹证据引用 | 受限存储引用，不粘贴坐标 |
| 候选坐标基准与转换版本 |  |
| 样点数、最大/平均误差 |  |
| 验收阈值及依据 |  |
| 结论、复核人 |  |

未通过的接口继续保存原值并标记 `unverified`，不得用于高风险控制预检。

## 8. 低风险写入现场验收模板

适用于直播、画质/录制/分享/转换器、航线上传、任务、标注和建模等。每个 action 单独一行，禁止用一次批准覆盖整个领域。

临时凭据签发可使用以下严格白名单命令。它只调用项目 STS 与开放建模上传 token，不上传对象、不发送 callback、不创建模型/任务，也不控制设备；两个 endpoint 全部成功时才写入 24 小时有效、账号绑定的 `security.temporary-credential` evidence：

```bash
pnpm accept:flighthub-low-risk -- \
  --project-id <local-project-id> \
  --connector-id <connector-instance-id> \
  --confirm-low-risk-write-acceptance \
  --persist-evidence
```

命令会在每个 POST 前重新核对连接器状态、项目作用域、账号指纹和凭据 envelope；任一变化、失败或未知结果都会阻止 evidence。输出仅含 endpoint ID、安全类别、计数/字段集合和耗时，不得据此开放 `flight.execute` 或 `model.write`。

Dock 直播首次现场验收使用单独的严格入口。命令只接受精确的本地 Dock、已投影且可用的视频通道，并在 POST 前重新核对设备类型、在线状态、连接器、账号、凭据和型号。FlightHub 当前 released 设备/状态接口不提供固件版本，因此仅 `live.control` 使用 24 小时有效的“账号 + 精确 Dock 型号 + 通道摘要”短期 evidence；其他设备 action 仍要求型号和固件。若后续能读取固件，既有无固件直播 evidence 自动失配并要求重验。只有真实启动响应能被已登记的直播供应商安全解析后才落证据，并只开启项目的 `live.control` flag；失败、未知结果或不支持的供应商均不落证据。不要对无人机 device id 运行此命令：

```bash
pnpm accept:flighthub-dock-live -- \
  --project-id <local-project-id> \
  --connector-id <connector-instance-id> \
  --device-id <local-dock-device-id> \
  --camera-index <projected-dock-camera-index> \
  --confirm-dock-live-acceptance \
  --persist-evidence \
  --enable-live-control
```

输出不含 SN、URL、Token 或坐标，只含 endpoint ID、安全类别、响应字段名、供应商/协议类型和耗时。验收结束或授权撤回时，应立即从 `project_feature_flags.flighthub_action_flags_json` 删除 `live.control`，撤销本地播放授权并清除短期供应商凭据；由于官方没有 released stop API，不得宣称远端已同步停止，需继续按 `live_status`、设备状态、凭据到期和五分钟无观众规则对账。

| action/endpoint | 测试窗口与资源 | 前置证据 | 预期远端结果 | 回滚/清理 | 审计引用 | 结论 |
| --- | --- | --- | --- | --- | --- | --- |
|  |  | RBAC、flag、`field-write`、幂等键 |  |  |  |  |

要求：使用测试项目/资源，指定回滚负责人；记录 HTTP/业务受理和最终远端结果；响应未知时保持 unknown 并先查询远端。通过后仅为该 action、账号及必要的型号/固件写入有期限 evidence。

## 9. 高风险现场验收模板

返航/取消、暂停/恢复、控制权/云控、相机/镜头、TCA、RTK、对频、迁移、DELETE 和组织权限变更必须另行获得用户批准，并有现场观察员。每次执行前再次确认精确目标和不可逆影响。

| action/endpoint | 精确目标摘要 | 观察员/操作者 | 安全与审批前置 | 逐项执行确认 | 远端/物理证据 | 恢复与结论 |
| --- | --- | --- | --- | --- | --- | --- |
|  | 不含完整 SN/UUID |  | 新鲜状态、区域、账号、型号、固件、flag、审批 |  |  |  |

任何未执行、取消、失败、证据矛盾或结果未知的 action 必须保持生产不可用。固件升级或账号指纹变化后，既有高风险 evidence 自动失效并需要复验。

## 10. 发布记录

每次发布保存：应用 commit、迁移 ledger、manifest/contract-lock 版本、文档检查输出、Go/Web 测试与构建摘要、真实只读 evidence 时间、未开放 action 清单、值班和回滚负责人。只保存安全摘要，不保存上游原始响应。
