# DJI Dock 2/3 实机接入与现场验收清单

本清单用于 AeroSight 接入 DJI Dock 2 + Matrice 3D/3TD 与 DJI Dock 3 + Matrice 4D/4TD。它不替代 DJI 产品手册、当地空域规则、现场风险评估或持证操作要求。任何控制测试必须由现场负责人授权并具备立即人工接管条件。

## 1. 账号、应用与版本基线

- [ ] 注册 DJI 开发者账号，在开发者中心创建类型为 **Cloud API** 的应用，取得 `app_id`、`app_key`、`app_license`。官方说明要求先完成 License 校验，之后才能使用 Cloud API/JSBridge：[DJI Cloud API 部署步骤](https://developer.dji.com/doc/cloud-api-tutorial/en/quick-start/source-code-deployment-steps.html)。
- [ ] 在连接器向导填写上述三项、MQTT 账号和媒体推流账号。AeroSight 使用 `AUTH_SECRET` 派生的 AES key 加密存库；读取 API、浏览器回显、审计和日志都不得包含原文。
- [ ] 在当天重新核对 [DJI Cloud API release notes](https://developer.dji.com/doc/cloud-api-tutorial/en/) 与 [产品支持矩阵](https://developer.dji.com/doc/cloud-api-tutorial/en/overview/product-support.html)，记录实际固件版本与 Cloud API 版本。
- [ ] 当前功能基线参考 Cloud API v1.16.1（2025-12-17）：Dock 2 与 Matrice 3D/3TD 为 `14.03.07.01`，Dock 3 与 Matrice 4D/4TD 为 `14.03.00.03`。这只是该版本新增功能的最低版本，不应当作永久固定值。
- [ ] 确认机场与配套飞行器型号匹配，飞机已与机场对频，序列号与拟认领的 `gateway_sn`/子设备 SN 完全一致。

## 2. AeroSight 网络与 Adapter 配置

- [ ] 先在“项目设置 → DJI Cloud API 接入”创建 LAN 或 Public profile；公网必须使用 `mqtts`、`https`、`wss`、`rtmps`，关闭匿名 MQTT，并使用受信任证书。
- [ ] 为每个项目/Adapter 签发独立 MQTT 身份和最小 Topic ACL；设备端只允许自己的 `sys/product/{gateway_sn}/#` 与 `thing/product/{gateway_sn}/#` 范围。
- [ ] 确认机场网络可以访问 MQTT、AeroSight API、WebSocket、RTMP/RTMPS、播放网关和 NTP；不要把 `localhost` 或回环地址提供给机场。
- [ ] 运行“连接自检”，保存每个端点的 `server_verified`/失败诊断；`deviceVerification=pending` 必须在机场侧实际联网后再关闭。
- [ ] 导出并双人复核向导生成的脱敏摘要。字段必须与 DJI 协议一致：

  - `gateway_sn`
  - `mqtt_broker.address`
  - `mqtt_broker.client_id`
  - `mqtt_broker.username` / `mqtt_broker.password`（摘要中只能显示 `[ENCRYPTED]`）
  - `mqtt_broker.enable_tls`
  - `config.app_id` / `config.app_key` / `config.app_license`（只能显示 `[ENCRYPTED]`）
  - `config.ntp_server_host` / `config.ntp_server_port`

  DJI Dock 2 的 config 回复字段见 [Dock configuration update](https://developer.dji.com/doc/cloud-api-tutorial/en/api-reference/dock-to-cloud/mqtt/dock/config.html)，Dock 3 还显式支持 `ntp_server_port`，见 [Dock 3 configuration update](https://developer.dji.com/doc/cloud-api-tutorial/en/api-reference/dock-to-cloud/mqtt/dock/dock3/config.html)。

## 3. DJI Pilot 2 / 机场上云

- [ ] 使用与目标机场兼容的遥控器和 DJI Pilot 2；按 Pilot 引导检查急停按钮、网络、飞机在舱状态和对频状态。DJI 官方机场上云流程明确要求这些检查并填写 MQTT 账号密码：[Dock access to cloud](https://developer.dji.com/doc/cloud-api-tutorial/en/feature-set/dock-feature-set/dock-access-to-cloud.html)。
- [ ] 在 Pilot 2 中完成 Cloud API License 校验；若通过 H5/Open Platforms 接入，配置实际可达的 HTTP/HTTPS 入口并验证 token 生命周期。
- [ ] 输入或下发 MQTT Broker 地址、账号密码、TLS 设置、组织 ID/设备绑定码；确认机场和飞行器绑定到预期项目，而不是共享测试组织。
- [ ] 等待 topology、state、osd 与 config 请求进入 AeroSight；未知产品枚举或未知固件必须显示为只读/降级，禁止强行赋予控制能力。

## 4. 现场安全门禁

- [ ] 指定现场负责人、远程操作者、观察员和紧急联系人；确认谁有权按下物理急停、谁有权批准返航/机场调试命令。
- [ ] 核对空域许可、地理围栏、天气、能见度、风速、GNSS/RTK、备用降落点、人员车辆隔离区、障碍物和通信覆盖。
- [ ] 清空桨叶与舱盖活动范围；首次上电、舱盖、充电、声光和重启测试在不装桨或制造商允许的维护状态完成。
- [ ] 在 AeroSight RBAC 中先只授予 `project:view` 和 `stream.*`；控制能力按 DeviceType/Device 范围逐项开放，返航和其他高风险动作保留二次确认与独立审批。
- [ ] 遥测稳定至少 15 分钟且时间同步正确后，才从只读进入受控命令阶段；任何断线、NACK、超时、固件不兼容或媒体未到流都停止升级测试范围。

## 5. 分阶段证据与验收顺序

1. 只读发现：记录机场、飞行器、摄像头、传感器的 Device ID、DeviceType 版本、Driver 版本、父子关系、固件、在线/新鲜度与有效能力。
2. 实时数据：保存遥测/传感 channel ID、schema、单位、样本时间和权限撤销结果。
3. 视频：分别启动两路相机，确认实际媒体到达后才显示 `live`；验证 WebRTC/HLS 授权、停止命令和摄取会话清理。
4. 低风险机场命令：在维护状态验证声光或允许的调试项，保存 command ID、`tid`/`bid`、请求人、审批、ACK/NACK 和设备最终状态。
5. 飞行命令：在批准空域内依次验证任务下发/开始/取消与安全返航；任何结果只以匹配的 DJI reply 和后续遥测为准。
6. 归档证据：保存脱敏配置摘要、网络自检、Topic ACL、固件矩阵、现场签字、截图/媒体、命令账本、审计事件和异常处置记录。

未接入真实硬件时，不得勾选 OpenSpec 10.2–10.4；协议模拟器通过不能替代实机与现场安全验收。
