# 司空公有云首次飞行测试

## 当前门槛

现场验证记录（`field-write`）只作为诊断证据保存，不再决定写操作是否开放。
接口和后台执行器仍检查账号权限、项目及设备范围、连接状态、项目功能开关。
AeroSight 的航线任务执行流程要求本次执行授权，并在下发前调用司空的条件检查；不再要求平台安全策略或策略版本。

## 公有云 OpenAPI 流程

使用司空公有云 HTTPS API，不使用设备直连的 MQTT Cloud API。

1. 查询航线目录及详情，得到“测试”航线的 `wayline_uuid`，确认机场 SN、机型、航点和返航参数。
2. `GET /openapi/v2.0/workspaces/{workspace_id}/flight-tasks/dispatch-checks?sn={dock_sn}&wayline_uuid={wayline_uuid}`：读取下发条件告警。该接口不执行飞行，告警列表也不是所有现场条件的保证。
3. 在本次飞行获用户明确确认之后，仅发送一次 `POST /openapi/v2.0/flight-task`。
4. 保存返回的 `data.task_uuid`，查询任务状态、实时遥测和任务日志，记录起飞、返航及完成结果。创建成功仅代表司空接收了请求。
5. 如果下发超时或响应丢失，先按任务名称、机场 SN、航线 UUID 查询是否已经创建；不得盲目重发立即任务。

创建接口请求头：`X-User-Token`、`X-Project-Uuid`，可附 `X-Request-Id`。认证值由现有连接器在服务端解密，不能写入预览文件或浏览器。

请求结构：

```json
{
  "name": "测试-AeroSight-首次验证",
  "sn": "<当前机场 SN>",
  "wayline_uuid": "<测试航线 UUID>",
  "time_zone": "Asia/Shanghai",
  "task_type": "immediate",
  "rth_altitude": 100,
  "rth_mode": "preset",
  "out_of_control_action_in_flight": "return_home",
  "resumable_status": "manual"
}
```

`immediate` 会触发执行，不是草稿。立即任务不填 `begin_at`。SN 填机场的 SN，不是飞行器 SN。

## 本机准备结果（2026-09-28）

- 项目 1、设备 2：DJI Dock 2；配套 Matrice 3TD。
- “测试”航线：1 个航点，路径距离 0，结束动作 `goHome`。
- 起飞安全高度 20 米；返航高度 100 米；航点高度使用 WGS84 绝对高度，不能当作离地高度。
- 本次读取机场在线、飞行器关机，司空下发条件查询没有返回告警。查询结果需在实际下发前重新读取。
- 含真实机场和航线标识的请求预览保存在本机 `.build/test-flight-request.preview.json`，不包含 API Token。
- AeroSight 安全策略的强制要求已删除。准备请求不等于已经执行飞行。

## 官方接口文档

- [检查飞行任务下发条件](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/456425797e0.md)
- [创建飞行任务](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/454273432e0.md)
- [查询飞行任务](https://s.apifox.cn/4de4a239-c2cc-4572-9b65-90738289f37a/454273439e0.md)

现有客户端对应 `CheckFlightTaskDispatch`、`CreateFlightTask` 和 `ListFlightTasks`；创建请求禁用自动重试，后台执行器负责丢失响应的对账。

## 首次下发与官方 Demo 对照（2026-09-28）

用户授权后发送了一次立即任务，返回 `upstream_error`，没有任务编号；随后只读查询未找到同名任务，机场待机、飞行器关机。收据在 `.build/test-flight-dispatch.receipt.json`，请求编号为 `fc8075a4-e841-4805-94b5-501ac41bc117`。该次原始响应的业务码已被旧客户端丢弃，无法据此确定具体拒绝原因，不应以参数猜测代替诊断。

已对照 [DJI 官方 OpenAPI V2 Demo](https://github.com/dji-sdk/FlightHub-2-OpenAPI-V2-Demo)：

- `Shared/request.ts` 的 Token、项目、请求编号和语言请求头与本客户端一致。Demo 保留响应业务码，本客户端现已补上 `APIError.BusinessCode`、`RequestID` 及结构化拒绝日志，包含 HTTP 状态；日志不包含 Token、原始消息或签名链接。
- 仓库是 TypeScript/Vue 教学示例，不是 Go SDK。公有云示例只有设备列表。
- 飞行计划库属于 `Privatization/FlightTaskLibrary`，创建路径为 `/openapi/v2.0/task/api/v1/workspaces/{projId}/flight-tasks`，使用数字枚举。本项目使用公有云 `/openapi/v2.0/flight-task` 和字符串枚举，不能直接照搬私有化参数。
- 本次对照未重复下发飞行命令。下一次下发前需先对账；若失败，使用新日志里的业务码和请求编号定位。

## GNSS 与双直播准备

第二次下发保存在 `.build/test-flight-dispatch-retry-2.receipt.json`：HTTP 200、业务码 219019（航线合法性检查未通过），没有任务编号。用户随后在司空增加第二个航点并手动执行成功。

当前读取“测试”航线为 2 个航点、约 16.67 米，机场待机；下发条件没有告警。新的本机请求预览设为 `task_type=immediate`、`wayline_precision_type=gps`（GNSS）、`rth_altitude=50`、`rth_mode=preset`、`repeat_type=nonrepeating`，名称为“测试-AeroSight-GNSS-50m”。这里只执行 `prepare-gnss-flight.go --prepare`，尚未下发。

任务页面的手动飞行运行会监测司空受理结果，收到远端任务编号后进入 `/projects/{projectId}/realtime/devices/{deviceId}/?autoLive=1`。实时作业根据设备关联展示机场与飞行器的独立直播，并尝试分别启动一个可用视频通道；飞行器离线时等待上线，不盲目重发直播请求，失败可手动重试。普通历史运行页面不会自动跳转。

发现并修复了先导入飞行器、后导入机场时缺少关联的导入顺序问题；本机已有两台设备的关联根据司空身份记录补齐。双播放器使用两个独立 RTC 引擎，只销毁各自的观看实例。代码与受理/关联测试已验证，仍需设备同时在线时验证两路真实视频画面。

## 真实飞行与平台入口（2026-09-29）

GNSS、预设返航高度 50 米的立即任务已下发成功。第一次任务 `0164376d-c72f-4578-8752-20206ef3d7d2` 有一条飞行记录、48 个航迹点、97 秒时长，高度变化约 50.2 米；任务和飞行器身份均匹配。第二次任务 `831a8c1e-f834-4fa6-a47c-d8e5def59700` 下发后，用户现场确认飞行器真实起飞。两次均使用独立持久化回执，只发送一次创建请求。

平台实时作业的设备操作现提供“执行航线”弹窗：选择同步的司空航线、填写任务名称、选择 GNSS/RTK 与返航高度，点击“起飞”创建立即任务。默认 GNSS、50 米。选中飞行器时通过项目内设备关系解析所属机场；不硬编码设备或航线。

`GET/POST /api/projects/{projectId}/devices/{deviceId}/flight-launch` 使用当前登录用户的 RBAC 权限，事务中创建任务运行、操作授权记录、加密的连接器任务与 outbox 事件。人工点击不要求第二位审批人，智能体授权流程保持独立。重复的请求键复用同一任务，参数变化拒绝复用。后台执行前查询真实设备目录、航线型号与航点数，并检查司空下发条件；未知响应只对账，不盲目再次创建飞行。

页面收到司空受理回执后进入实时作业并尝试启动关联设备的两个视频通道。设备状态同步按遥测轮询频率重新查询真实在线目录，避免飞行器上电后仍等待慢速库存刷新；任务回执按已知远端任务编号读取，不依赖任务列表是否完整。起飞入口、权限/范围隔离、参数、重复下发和上线刷新通过自动化测试；本次实现期间未另行发送真实起飞命令，平台按钮与双视频仍待用户点击实测。
