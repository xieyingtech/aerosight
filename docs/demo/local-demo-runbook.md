# 本地完整演示：已完成的历史图片链

2026-09-14 实测：独立数据库 `aerosight_msup_demo`，项目 #1，Task #3，Run #3。Task 编号以 `.build/msup-demo/demo-state.json` 为准；运行编号和任务编号是不同实体。

## 演示入口

- 平台：http://127.0.0.1:8092
- 成功运行：http://127.0.0.1:8092/projects/tasks/runs/detail/?projectId=1&runId=3
- 报告：http://127.0.0.1:8092/projects/reports/detail/?projectId=1&reportId=11cc1bf8-0c64-4280-b423-9e4421b3be33
- 工单：http://127.0.0.1:8092/projects/issues/detail/?projectId=1&issueId=1
- YOLO 上传测试：http://127.0.0.1:8091/docs

当前浏览器已登录独立本地演示库的默认管理员。演示服务仅监听本机；数据库没有司空连接器或飞行设备。生产库未启动后台执行器，本次无飞行、返航或设备控制动作。

## 已验证的链

1. 经 `POST /api/projects/:id/assets/import` 导入用户手动下载的 RGB 原图 `DJI_20260818151715_0012_V.jpeg`，8064×6048，资产 #1。服务器完整解码，记录 SHA256、来源和拍摄时间，不生成虚构 GPS。
2. 正式 API 创建、发布、启用 YAML Task；手动触发后由标准 Go 后台执行器执行五步，无 SQL 强制成功。
3. `inspection.observe` 冻结一张历史图片的范围，`inspection.detect` 调用本地 YOLO11n。YOLO 从平台 HTTPS 签名网关读取真实原图、核对哈希，返回 8 个 car 预测框。
4. `copilot.run` 调用已配置的真实 `step-3.7-flash`，基于结构化检测证据研判，进入 needs_review。该调用不是视觉大模型直接看图；图像感知由 YOLO 完成。
5. 在网页复核表单模拟演示操作：确认一条待现场核实线索、驳回其余七条，保存 revision 2。此操作由助手代操作，仅证明人工复核入口可用，不冒充现场人员审核。
6. 继续执行 `issue.create-or-update`，生成工单 #1「巡检待核实线索」；报告步骤完成，生成报告草稿。五步全部 succeeded。

模型结论与复核理由均保留；工单不是违法停车认定。模型原文把空 `allowedIssueIds` 误解成不能新建案，这是本次真实输出的一项局限；平台仍通过明确候选与人工复核建立线索。未验证准确率、米级定位、实机飞行或无人值守闭环。

报告当前为自动生成的 draft，不是已对外发布的比赛报告。范围 complete 只表示选定的一张图片处理完成，不代表完整架次或公园全覆盖。汇总证据的模型版本字段仍为 unknown；检测页已从各图片结果显示实际模型版本，可从关联算法运行查看权重摘要。报告的汇总版本展示仍可继续完善。

## 录屏顺序

Task YAML / 参数表单 → 运行五步状态 → 观察原图 → 算法运行与检测框 → 模型原始研判 / 复核修订 → 工单证据链 → 报告。

成功运行可随时回放。重新演示时从 Task 页面点击“手动运行”；真实模型输出可能变化，不保证每次同文。相同来源有防重逻辑，重跑可能关联已有工单，不能宣称每次都新建一条。此次未单独完成重放矩阵。

截图位于 `.build/msup-demo/screenshots/`，结果快照位于 `.build/msup-demo/run-detail.json`。Run #1 保留 TLS 信任失败，Run #2 保留输出映射配置错误，Run #3 为成功链。旧调试 Task 已停用。

## 本地服务与重启

- 平台进程：`.build/msup-demo/platform.pid`；日志：`platform.log`。
- YOLO 进程：`.build/msup-demo/yolo-server.pid`；日志：`yolo-server.log`。
- TLS 网关：`.build/msup-demo/tls-proxy.pid`；日志：`tls-proxy.log`。
- 演示密钥、环境、对象存储、证书及数据库备份不提交 Git。

在项目根目录运行；先核实端口占用与 PID，避免重复启动：

```sh
# YOLO 需要访问本地签名资产网关并信任演示证书
YOLO_ASSET_ORIGINS=https://127.0.0.1:8444 \
SSL_CERT_FILE="$PWD/.build/msup-demo/tls/cert.pem" \
infra/demo/yolo/start.sh

# 单独终端启动 TLS 桥，只转发 /infer 和 /algorithm-assets/
.build/msup-demo/yolo-venv/bin/python infra/demo/yolo/tls_proxy.py

# 单独终端启动平台（后台运行；依赖已有 .env.local 的数据库连接）
node infra/demo/msup/start-platform.mjs

# 创建缺失的演示配置；已有 demo-state.json 则复用
.build/msup-demo/yolo-venv/bin/python infra/demo/msup/setup.py
# 首次创建一次 Run；已有 Run 时只读取其状态
.build/msup-demo/yolo-venv/bin/python infra/demo/msup/run.py
```

证书当前有效期 30 天，过期需重新生成并重启两个客户端和网关。平台通过 `ALGORITHM_CA_FILE` 显式加载部署 CA，不关闭 TLS 校验。`ALGORITHM_DEVELOPMENT_ENDPOINT` 只在 development 模式允许完全匹配的 `https://127.0.0.1:端口/路径`，不改变 AI 或其他出站策略。生产环境拒绝该开发配置。

新增图片导入接口限项目 owner/admin、最大 40 MiB、JPEG/PNG；可携带 `capturedAt`（RFC3339）和 `sourceDescription`。新导入保留独立原始字节，不自动推断飞行或目标坐标。当前为 API 入口，资产页上传表单仍可后续补齐。

## 检查状态

`pnpm check`、`pnpm build` 已通过。真实临时数据库测试覆盖超过原 2 MiB 限制的图片导入、哈希一致性、不创建 Run、非法图片不入库、成员与跨项目拒绝；开发地址测试覆盖生产拒绝、地址/端口/路径精确匹配及通用出站策略不放宽。部署 CA 测试覆盖真实 TLS 握手与无效证书文件拒绝。

OpenSpec 仍为 16/32：本次提供真实单链证据，未冒充 4.4/7.3 等任务要求的完整异常、重启、取消、跨项目、效果评测矩阵全部通过。
