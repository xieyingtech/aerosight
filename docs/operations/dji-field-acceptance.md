# DJI Cloud API 兼容验证与实机验收边界

本文记录 `dji.cloud-api` 的当前交付证据和未来实机验收清单。当前团队没有 Dock 2/3、Matrice 3D/3TD/4D/4TD 实机，因此 AeroSight **不开放 DJI Cloud API 直连接器的新建入口，也不声明物理控制或真实媒体链路已验收**。连接器页面目前只开放已验证的只读 `dji.flighthub2` 类型；历史直连实例和审计记录继续保留。

## 当前已完成的非实机验证

以下结论来自版本化 fixture、协议模拟器、数据库约束和自动化测试，可以用于回归兼容性，但不能代替现场签字：

- Dock 2 + Matrice 3D/3TD、Dock 3 + Matrice 4D/4TD 的产品枚举、父子拓扑、DeviceType、能力与实时通道投影。
- 未知产品或未知固件只读降级，不获得控制能力；跨连接器外部身份冲突进入 `conflicted`，不自动改写下行路由。
- `automatic` 只接受唯一、精确、无冲突的 DeviceType 匹配；`review`、`observe-only`、未知、多候选和已忽略对象不会自动创建 Device。
- direct/gateway/inherited 显式绑定、主备优先级、连接器切换和双主失败关闭；下行控制发布前必须解析出唯一活动主路由。
- ACK、NACK、超时、断连和迟到回复写入同一命令账本；模拟结果带 fixture/simulator 边界，不转换为物理成功。
- LAN/Public profile 策略、Topic ACL 模板、短期媒体权限、能力 RBAC、直播租约清理和历史证据保留由自动化检查覆盖。

回归入口：

```sh
pnpm check
pnpm test:migrations
pnpm test:security
pnpm build
pnpm exec openspec validate integrate-dji-cloud-device-platform --strict
```

## 可用性门槛

只有同时满足以下条件，后续独立 OpenSpec 变更才可以重新开放 `dji.cloud-api` 新建类型：

1. 有明确型号、固件基线和可安全操作的真实 Dock/飞行器。
2. LAN 或 Public profile 在设备侧完成 MQTT、API、WebSocket、NTP、媒体摄取与播放的真实连通验证。
3. 每项目、每连接器的独立身份和最小 Topic ACL 已部署；浏览器、日志、审计和普通数据库字段不出现秘密原文。
4. 现场负责人批准测试空域、地理围栏、天气、应急联系人、物理急停和人工接管方案。
5. 只读遥测稳定后再逐级开放低风险维护命令、媒体、任务和返航；任一断线、NACK、超时或未知固件立即停止扩大范围。

fixture 或模拟器通过只能证明协议处理和失败语义，不能满足上述门槛。

## 后续真实设备验收清单

### 网络与身份

- [ ] 为目标项目和连接器签发独立 MQTT/媒体身份，关闭匿名访问，只允许该网关的 Topic 范围。
- [ ] 从设备网段验证 MQTT、AeroSight API、WebSocket、NTP、RTMP/RTMPS；从用户网段验证 HLS/WebRTC。
- [ ] 保存服务端自检和设备侧握手证据；只有两侧均成功才关闭 `deviceVerification=pending`。
- [ ] 核对 TLS 证书、时间同步、DNS、固件、机场与飞行器配对及序列号。

### 只读与拓扑

- [ ] 验证机场、飞行器、摄像头和传感器的稳定 Device ID、DeviceType、Driver、父子关系、在线状态和数据新鲜度。
- [ ] 断开并恢复连接器，确认外部身份不重复创建设备，历史记录不丢失。
- [ ] 切换主备连接器，确认 Device ID 不变且任一时刻只有一个有效主路由。

### 媒体与控制

- [ ] 实际媒体到达后才显示 `live`；错误发布/播放身份、随机路径外推流和停止后的继续读取都必须失败。
- [ ] 在制造商允许的维护状态验证低风险命令，保存 command ID、请求人、审批、协议关联、ACK/NACK 和最终遥测。
- [ ] 在批准空域验证任务开始/取消和返航；UI 成功只能来自匹配回复与后续设备事实，不能来自发送成功。
- [ ] 验证断连、NACK、超时、迟到 ACK、Worker 重启和直播租约过期均失败关闭并可清理。

### 证据归档

- [ ] 归档脱敏配置摘要、网络自检、Topic ACL、固件矩阵、现场签字、截图/媒体、命令账本、审计事件和异常处置。
- [ ] 在验收记录中区分 `fixture`、`simulator` 与 `physical-device`，不得混用结论。
