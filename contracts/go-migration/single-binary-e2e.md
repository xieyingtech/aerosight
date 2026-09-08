# 单服务端到端验收

对应任务 8.2，2026-09-08 执行 `pnpm test:release-image` 成功，测试资源清理完成。正式镜像 `sha256:7d1cc9e8ef0c68edc3e27ee2c5eeac93ce1311e876a2367a5c74a04fb6c4062a`；同一次执行的证据目录为 `.build/container-lifecycle-68eaa516-9640-4dcd-abfb-2bcc437f5d05/`。

| 要求 | 实际执行与证据 |
| --- | --- |
| 单二进制生产包 | 镜像自己的 Go 二进制作为 PID 1，以默认非 root 用户运行；无源码/二进制 bind mount，无 Node/pnpm，只有一个 8080 TCP listener，直接返回嵌入页面。`result.json` 的 mode 为 release-image。 |
| 登录、新项目 | 真实 CSRF/login API 登录初始化账号，通过 API 创建团队和项目，后续全部业务使用同一项目。重启保留会话与项目。 |
| 设备模拟接入 | 真实 dji-setup 与连接检查启用适配器；认证 Mosquitto 接收现有 Go Dock 2 模拟器拓扑与 OSD，自动投影六个设备并进入项目 snapshot。`device-flow.json` 记录设备 ID、epoch 1→2 和重启后的新遥测。 |
| 任务执行 | 迁移清单没有创建任务 API，预置暂停任务定义/版本/步骤；真实 resume API 产生审计/outbox，后台发出 dock.debug.control/debug.open 服务命令，模拟器通过 MQTT 返回 ACK，任务成功。`mission-flow.json` 记录一条命令、一次 attempt、一次 resume 审计；broker.log 只有一次向模拟器投递 services，重启不重发。 |
| 算法回调 | 预置 provider/输入图像，API 创建算法定义与运行；outbox 向受信任 HTTPS 假上游实际请求，Go 签发回调凭据。重启后完成回调及重放，结果和回执不重复，上游只执行一次。见 `result.json.callbackRecovery`。 |
| SSE | 24 条连接配合 80 次快照读取；超过 30 秒后全部收到新插入事件。SIGTERM 全部 EOF、数据库连接归零；本轮退出 592 ms。见 `result.json.load`。 |
| 资产 | Go 生成输入资产签名 URL，实际 HTTP 下载并逐字节核对 PNG，使用应用真实本地对象存储。见 callbackRecovery.signedAssetBytesVerified。 |
| AI 假上游 | API 创建 provider/session/message，官方 SDK 通过真实 HTTPS 请求 Responses helper；六个只读工具实际查库，证据留存，上游错误脱敏，活动请求在 SIGTERM 取消且连接关闭。`ai-flow.json` 记录重启后历史不变、不重发、不产生虚假 assistant。 |

PostGIS、MQTT、HTTPS helper 和协议模拟器是隔离测试依赖，不是应用生产进程。媒体/NTP 地址仅为配置连通 fixture，真实播放由独立媒体验收覆盖。本文不把暂停任务 fixture 描述成任务创建 API，也不证明相机拍照、飞行等未列出的设备动作。任务 8.1 的全入口兼容、8.3 的完整退出/恢复门槛和 8.5 的规格同步仍需独立收尾；尤其结果文件跨重启持久化及预算耗尽恢复不能从本次正常停止推导。

后续 `.build/container-lifecycle-31a30e35-38e0-40a9-a3f8-8334ff987511/object-persistence.json` 已单独补齐文件持久化：实际 named volume、完成结果后的再次进程重启、结果 JSON/SHA-256、原签名输入下载与回调重放全部通过。预算耗尽恢复仍需单独核对。
