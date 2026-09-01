# AeroSight 通用平台复用清单

司空 2 连接器只增加新的 Connector Definition 和 runtime，不建立平行的
连接、设备、同步或事件领域模型。

| 关注点 | 复用实现 | 司空用途 |
| --- | --- | --- |
| Connector Definition | `connector_definitions` | 注册 `dji.flighthub2@1.0.0` 的协议、发现模式和兼容 Driver |
| Connector Instance | `device_adapters` / `connector_instances` | 保存项目作用域、状态、同步游标与加密凭据 |
| 同步运行 | `connector_sync_runs` | 记录首次、手动和周期 poll 的结果与脱敏错误 |
| 外部设备身份 | `device_external_identities` | 保存司空项目中的 Dock/飞行器稳定外部身份和发现状态 |
| 设备绑定 | `device_connector_bindings` | 将经管理员确认的外部身份接入统一设备目录 |
| 凭据 | `credential_envelope_json` 与 Web/Worker AES-GCM 实现 | 加密组织 Token，使用项目和 Adapter AAD |
| 异步调度 | `outbox_events` 与 Worker outbox consumer | 在事务提交后调度首次和手动同步 |
| Connector runtime | `apps/worker/internal/connector` | 复用 registry、scope filter、完整快照和同步游标 |
| DeviceType/Driver | `apps/worker/internal/driver` 与 `internal/dji` 产品矩阵 | 识别已知 Dock 2/3 和配套飞行器，仅投影只读能力 |
| 租户边界 | `project_id` + `team_id` 复合外键和项目权限 | 所有查询、写入、同步及绑定保持项目隔离 |

禁止新增 `flighthub_connections`、`flighthub_devices`、
`flighthub_sync_runs`、`flighthub_events` 或同义专用表。后续能力继续通过上述
通用模型扩展。
