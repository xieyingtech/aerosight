# DJI 统一设备平台交付验证（2026-08-27）

## 自动化验证

- `pnpm check`：通过；TypeScript typecheck 通过，Web 单元测试 209/209 通过，Worker Go 测试通过。
- `pnpm build`：通过；Next.js 生产构建与 Worker 二进制构建成功。
- `go test ./...`：所有 Worker 包通过，无失败或跳过的业务测试。
- `openspec validate integrate-dji-cloud-device-platform --strict`：change 有效。
- `pnpm test:security` 与真实 MQTT Broker 定向验收：结果见 [安全验收记录](./dji-security-acceptance-2026-08-27.md)。

## 本地协议级验收

- Dock 2 + Matrice 3TD 与 Dock 3 + Matrice 4TD 经真实 MQTT 5 Broker 自动认领为统一 Device 拓扑；页面显示 DeviceType、`dji.cloud` Driver、状态新鲜度与有效 capability。
- 飞行器返航命令走 capability RBAC、命令账本、DJI `services`/`services_reply`，匹配 reply `result=0` 后才进入 acknowledged。
- 摄像头直播走 DJI `live_start_push`、模拟 H.264 RTMP、MediaMTX 实际媒体健康、短期 WebRTC 播放授权；停止后 stream 进入 `stopped` 且媒体摄取清理。
- 通用文档 OCR 走动态 Algorithm Definition、HTTP-JSON Provider、Worker outbox、canonical `ocr` mapping 和原始结果对象存储，运行状态 `succeeded`。
- DJI Adapter 向导支持 LAN/Public，连接自检结果区分服务端验证和设备侧 pending；秘密只显示 `[SECRET_REF]`。

## 未完成边界

OpenSpec 10.2–10.4 仍未完成：当前没有用户提供的 Dock 2/3 实机、Cloud API 凭据、现场安全授权、公网域名/证书与验收网络。模拟器结果不能替代实机证据，后续必须按 [实机接入与现场验收清单](./dji-field-acceptance.md) 执行。
