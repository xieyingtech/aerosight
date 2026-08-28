# 设备网络 Profile 部署指南

实机接入前还必须完成 [DJI Dock 2/3 实机接入与现场验收清单](./dji-field-acceptance.md)。

设备网络 Profile 是项目 Adapter 引用的显式连接配置。它分别描述机场或协议模拟器可达的 MQTT、API、WebSocket、媒体摄取地址，以及用户浏览器可达的媒体播放地址。不得从浏览器当前 Host 自动推导这些地址。

## 配置文件与校验

- 局域网模板：[lan.example.json](../../infra/demo/profiles/lan.example.json)
- 公网模板：[public.example.json](../../infra/demo/profiles/public.example.json)

先复制模板并替换地址与 `secretRef`。`secretRef` 只保存秘密管理系统中的引用；用户名、密码、token、流 key 不得写入 URL、JSON 或数据库普通字段。配置策略校验是独立入口，不会修改 `package.json`：

```sh
node infra/demo/validate-network-profiles.mjs path/to/profile.json
```

仓库模板可以在无环境变量的干净 shell 中校验：

```sh
env -i PATH="$PATH" node infra/demo/validate-network-profiles.mjs
env -i PATH="$PATH" docker compose --env-file infra/demo/.env.example -f infra/demo/compose.yaml config -q
```

此命令只验证 profile 的地址与安全策略。保存并绑定 profile 后，管理员还必须调用 Adapter 的连接自检；自检显示 `serverVerification=verified` 只表示 AeroSight 服务端已验证 DNS、TCP/TLS/HTTP，机场侧 MQTT 登录、回调和 RTMP 握手成功前，`deviceVerification` 保持 `pending`。

## 局域网部署

所有地址必须填写机场所在网段能够路由到的主机 IP 或内部 DNS 名称。允许 RFC1918 地址，拒绝 `localhost`、`127.0.0.0/8`、`::1`、链路本地和未指定地址。单机演示默认端口如下：

| 用途 | 示例 | 默认端口 | 防火墙方向 |
| --- | --- | --- | --- |
| MQTT 5 | `mqtt://192.168.20.10:1883` | TCP 1883 | 机场/模拟器 → Broker |
| AeroSight API | `http://192.168.20.10:3000` | TCP 3000 | Pilot/设备 → Web |
| WebSocket | `ws://192.168.20.10:3000/device-events` | TCP 3000 | Pilot/设备 → Web |
| RTMP 摄取 | `rtmp://192.168.20.10:1935` | TCP 1935 | 机场 → MediaMTX |
| HLS 播放 | `http://192.168.20.10:8888` | TCP 8888 | 浏览器 → MediaMTX |
| WebRTC | 播放服务协商地址 | TCP 8889、UDP 8189 | 浏览器 ↔ MediaMTX |

只向受信设备网段开放 MQTT/RTMP，只向运维网开放 MediaMTX API 9997。`MEDIA_WEBRTC_ADDITIONAL_HOSTS` 必须填写浏览器实际访问的 LAN 地址，不能保留 `127.0.0.1`。多网卡环境要分别从机场网段和用户网段验证路由，摄取与播放地址可以不同。

局域网也必须关闭 MQTT 匿名访问并使用每个 Adapter 独立凭据。启动本地依赖并验证真实认证消息与媒体链路：

```sh
cd infra/demo
cp .env.example .env
# 将所有 replace-with-* 值替换为随机密码，并设置 LAN 地址。
docker compose up -d mqtt media
./verify.sh
```

## 公网部署

公网 Profile 的 MQTT、API、WebSocket、RTMP 摄取和播放入口分别必须使用 `mqtts://`、`https://`、`wss://`、`rtmps://` 与 `https://`。建议只开放 TCP 443，并通过支持对应协议的负载均衡器或反向代理分流；若 MQTT/RTMPS 使用独立端口，常用端口为 8883 与 1936。MediaMTX 控制 API 不得暴露公网。

为 `mqtt`、`api`、`ingest`、`media` 域名配置受信 CA 签发的证书，证书 SAN 必须覆盖实际域名，并包含完整中间证书链。设备和服务端时钟必须同步，部署前检查证书有效期。不要使用自签名证书、开发证书、IP 与证书域名不一致的入口，或者由反向代理降级到匿名上游。

公网防火墙至少执行以下限制：

- MQTT/RTMPS 入口启用速率限制和连接数限制；能取得稳定出口网段时增加来源 allowlist。
- API/WebSocket 只暴露需要的 callback 与协商路径，管理后台和 MediaMTX API 走独立内网。
- HLS/WebRTC 播放需要短期授权；禁止公开目录枚举和永久播放 URL。
- 证书续期后重新执行 Adapter 自检；单个连接器凭据通过空白敏感表单更新，平台 `AUTH_SECRET` 按统一原子轮换手册执行。

## MQTT Topic ACL

`infra/demo/mosquitto.acl.template` 只用于单项目演示。生产中必须按项目、Adapter 和网关序列号隔离 Topic。例如为网关 `GW001` 签发的设备身份只允许在 `dji/<project-key>/GW001/#` 范围发布/订阅，Worker 身份只获得该项目 Adapter 已认领网关的范围；禁止 `#`、`dji/#` 或跨项目通配授权。Broker 必须启用认证，`mqttAnonymous` 必须为 `false`。

ACL 变更后分别用设备身份和 Worker 身份验证允许 Topic，再验证相邻项目 Topic 被拒绝。日志只记录凭据标识和 Topic 范围，不记录密码或完整 token。

## 媒体权限

发布、播放和管理使用三种独立身份：连接器中的发布凭据只能向随机、短期摄取 path 推流；播放者使用 AeroSight 签发的短时令牌；`MEDIA_ADMIN_USER`/`MEDIA_ADMIN_PASSWORD` 只访问内网控制 API。公网部署必须在播放入口校验短期凭据，并在直播停止或租约过期时撤销 path。

验收时依次确认：错误发布密码失败、错误播放密码失败、发布者不能读取、播放者不能发布、随机 path 以外的推流失败、停止后 HLS/WebRTC 不再可读。任何一项未通过，Adapter 和直播能力都不得标记为可用。
