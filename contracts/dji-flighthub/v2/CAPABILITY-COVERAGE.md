# DJI 司空 2 OpenAPI 能力覆盖表

本表以 [endpoint manifest](./endpoints.tsv) 为接口级事实源，以 Worker `Capabilities()` 为运行时能力事实源。所有 89 个 released endpoint 都必须归属于下表一个领域；`pnpm check:flighthub-docs` 会验证计数和领域标记，阻止孤立接口进入发布。

“已实现”表示 typed client、共享安全 transport 和本地投影/作业入口存在，不表示当前账号可调用，更不表示现场写入已经批准。账号、项目、设备型号、固件和证据过期都可能进一步收窄运行时能力。

## 证据快照

| 项目 | 当前合同状态 |
| --- | --- |
| released endpoint | 89 |
| 方法 | 59 GET / 19 POST / 6 PUT / 5 DELETE |
| 风险 | 59 low / 3 medium / 15 high / 12 critical |
| manifest 验证字段 | 73 documented / 14 live-read / 1 live-empty / 1 parameter-required |
| 真实只读验收 | 2026-09-02 对当前连接执行 59/59 GET；账号作用域 evidence 已持久化 |
| 现场写入验收 | 尚未执行；所有 action 必须继续要求 `field-write` |
| 坐标基准 | `unverified`；7.7 现场控制点与任务轨迹验收待完成 |

manifest 的 `verification` 是版本化合同注记；数据库中的 capability snapshot 才是账号/型号/固件作用域的运行时证据。两者都不能替代审批、功能开关或现场安全条件。

## 领域覆盖

| 领域 | 接口 | 运行时能力 | 本地接入 | 当前证据与限制 |
| --- | ---: | --- | --- | --- |
| `system` <!-- domain:system endpoints:2 --> | 2 | `health.read` | 系统健康、状态 typed client；连接器健康诊断 | GET 已纳入真实只读；非零业务码仍按失败类别处理 |
| `security` <!-- domain:security endpoints:2 --> | 2 | `security.temporary-credential`、`security.sn.decrypt` | STS 受控上传、SN 解密短期流程 | fixture 已覆盖；两项均为写/敏感流程，默认关闭且无 `field-write` |
| `organization` <!-- domain:organization endpoints:8 --> | 8 | `organization.read`、`organization.project-member.write`、`organization.write` | 用户、角色、权限、成员视图及受控管理作业 | 读取已接入；组织写入/权限变更未现场验收 |
| `project` <!-- domain:project endpoints:4 --> | 4 | `organization.read`、`organization.project-member.write`、`organization.write` | 项目发现、用户/成员/加入码读取与成员写入作业 | 项目发现真实只读通过；成员写入保持关闭 |
| `device` <!-- domain:device endpoints:6 --> | 6 | `inventory.read`、`state.read`、`health.read` | 目录、详情、物模型、HMS、拓扑、自动录制投影 | Dock 2/M3TD 状态已真实回填；坐标仍为 `unverified` |
| `control` <!-- domain:control endpoints:14 --> | 14 | `tca.status.read`、`device.control`、camera/lens、RTK、relay、active-project | 命令账本、状态对账、短期控制会话、维护作业 | GET probe 已执行；全部物理/维护写动作无现场授权，默认关闭 |
| `flight` <!-- domain:flight endpoints:20 --> | 20 | `flight.read`、`flight.execute` | 航线/任务/轨迹/媒体/告警同步，上传和任务长作业 | 13 条航线已对账；当前可访问任务为空；写入未现场验收 |
| `live` <!-- domain:live endpoints:11 --> | 11 | `live.read`、`live.control`、quality/recording/share/converter actions | 多供应商播放、会话对账、录制/分享/转换器目录与作业 | 空目录按健康状态处理；启动及各写 action 未现场验收；没有 released stop API |
| `geospatial` <!-- domain:geospatial endpoints:9 --> | 9 | `geospatial.read`、`geospatial.write`、`geospatial.element.delete` | 标注、飞行区、离线地图、AirSense 投影和受控写入 | 读取已接入；签名 URL 按需刷新；坐标校准和写入现场验收待完成 |
| `model` <!-- domain:model endpoints:13 --> | 13 | `model.read`、`model.write`、`model.delete`、`model.resource.delete` | 模型/open model 目录、产物、上传 token、重建长作业 | 2 个模型已对账；开放模型为空；start/stop/delete 未现场验收 |

## 写能力门禁

所有 action capability 的 `DefaultEnabled` 均为 false。实际调用必须同时满足：

1. 连接器为 `connected`，项目/团队作用域一致。
2. 用户具备对应 RBAC；高风险项为 owner/admin，并具有管理或操作授权。
3. endpoint 存在于 manifest，Driver 也声明所需设备能力。
4. 当前账号存在未过期的 `field-write` capability snapshot；设备动作还需精确匹配型号和固件。
5. 专用 feature flag 已开启，预检、差异预览、审批/二次确认和幂等键有效。
6. Worker 只将 HTTP/业务受理记为已受理，通过 command/task/state evidence 对账最终结果。

关键专用开关包括 `flighthub.live.*`、`flighthub.geospatial.delete`、`flighthub.model.delete`、`flighthub.model-resource.delete`、`flighthub.camera.change`、`flighthub.lens.change`、`flighthub.rtk.calibrate`、`flighthub.relay.pair`、`flighthub.device-migration`、`flighthub.sn-decrypt` 和 `flighthub.organization.project-member`。未列出的通用低风险写入口仍受 `flighthub.actions` 及上述全部门禁约束。

## 已知未完成项

- 坐标：需要已知控制点与同一时段任务轨迹，记录接口坐标基准、转换版本和误差；通过前不用于控制。
- 低风险现场写入：直播、任务/航线上传、建模和标注等需用户批准测试窗口、资源与回滚负责人。
- 高风险现场写入：物理控制、DELETE、迁移和组织权限变更需单独批准、现场观察员和逐 action 确认。
- 契约漂移：官方 method/path/schema 变化会把相关能力降为待验证，高风险能力不会自动继承旧证据。
