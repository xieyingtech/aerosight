# DJI 平台安全验收记录（2026-08-27）

环境：本地隔离 PostGIS、Mosquitto MQTT 5、MediaMTX、AeroSight Web/Worker 与协议级 DJI 模拟器。测试凭据只从忽略提交的 `infra/demo/.env` 读取，输出与本记录均不包含秘密值。

## 结果

| 验收面 | 结果 | 证据 |
| --- | --- | --- |
| 租户/项目隔离 | 通过 | snapshot、replay、asset、provider 与命令测试拒绝跨项目 ID；未授权资源不可区分地失败 |
| capability RBAC | 通过 | DeviceType/Device scope、wildcard 与 explicit deny；member 无 Adapter 管理权；高风险命令要求精确二次确认 |
| MQTT 认证与恢复 | 通过 | 真实 Broker 上 MQTT 5 认证、QoS 1、断线重连和订阅恢复；错误密码进入 degraded |
| Topic 方向 ACL | 通过 | device 身份发布 `services` 与 platform 身份发布 `services_reply` 均收到 MQTT 5 `Not authorized` / PUBACK RC 135 |
| 播放鉴权 | 通过 | 跨项目 stream、过期 token、篡改 path/protocol 均拒绝；WebRTC 优先、HLS fallback 有测试覆盖 |
| 秘密脱敏 | 通过 | Adapter/Provider public view 仅返回 `hasSecret`；配置摘要仅显示 `[SECRET_REF]`；嵌套 inline secret 被拒绝 |
| 重复命令/副作用 | 通过 | idempotency、精确 command/ACK 关联、未知 ACK 隔离、直播重复停止与并发冲突测试通过 |
| 超时与恢复 | 通过 | command timeout/NACK/disconnect 不显示物理成功；直播租约恢复和孤儿清理测试通过 |
| SSRF/回调/证据 | 通过 | restricted address、redirect 再校验、callback replay、checksum、不可变证据版本与 retention 测试通过 |

执行结果：

- `pnpm test:security`：52/52 Node 安全验收通过；Go agent、DJI、mission、observability 包通过。
- 设备 capability、命令、实时权限、直播状态机与播放 token 定向测试：17/17 通过。
- DJI MQTT Broker 集成：认证重连与错误认证 2/2 通过。
- Mosquitto 方向 ACL：2/2 未授权发布被 Broker 拒绝。

本记录只覆盖无需真实硬件即可验证的安全边界，不替代 Dock 2/3 实机与 LAN/Public 现场验收。
