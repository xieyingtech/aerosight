# 连接器开发契约

连接器负责把外部平台或设备网络映射到 AeroSight 的统一设备事实，不拥有另一套设备、命令或权限模型。新增连接器必须遵守以下边界。

## 注册与可用性

- 通过版本化 `connector_definitions` 声明 key、版本、配置/凭据 schema、发现模式、协议、兼容 Driver 和租约参数。
- 只有完成目标环境验收的类型才可以进入“新建连接器”目录。实现存在、fixture 通过或历史实例存在，都不等于允许新建。
- 暂不可用的类型保留定义、迁移兼容和历史读取，但不渲染秘密/端点配置表单。
- 项目页面采用列表优先；新建按钮先选择类型，再挂载该类型的临时配置组件。关闭、成功或失败后必须清空内存凭据。

## 安全与租户边界

- 所有读写先验证项目权限，连接器配置仅限 owner/admin；普通成员只能读取获准的设备事实。
- 凭据只进入加密 envelope 或外部 secret reference，不进入 URL、普通 JSON、日志、审计输入、事件 payload 和浏览器回显。
- Worker 的租约、游标、同步运行和 outbox 事件都带项目与连接器实例范围；重试必须幂等。
- 外部错误归一化为稳定安全码，不回传上游响应体、请求头或凭据片段。

## 发现与纳管

- 外部对象先写 `device_external_identities`，状态只能在 `discovered`、`managed`、`conflicted`、`ignored`、`missing` 中转换。
- `automatic` 仅在唯一精确 DeviceType 匹配、无重复身份、无跨来源冲突时创建设备；`review` 和 `observe-only` 永不自动创建。
- 已忽略对象再次出现仍保持忽略；完整快照缺失只改变来源状态，不删除 Device 或历史。
- DeviceType/Driver 是能力事实来源。连接器上报的任意能力字符串不能绕过 catalog 和 manifest 直接成为可执行能力。

## 路由与控制

- 每个纳管来源写 `device_connector_bindings`，明确 `direct`、`gateway` 或 `inherited`，并维护 priority 与 active/standby/disabled/conflicted 状态。
- 下行命令只能选择唯一的最高优先级活动绑定；无路由或双主同优先级必须在发布前失败关闭。
- 连接器迁移复用原 Device ID，把旧绑定切为 standby/disabled，再激活新绑定；不得通过删除重建设备迁移。
- 所有命令继续使用 `device_commands`、`command_attempts` 和协议关联表；连接器发送成功不能直接写业务成功。

## 最低验证集

- manifest/schema、项目隔离、凭据脱敏、租约并发、游标重放和分页上限。
- 唯一/未知/多候选/冲突/忽略后重现的纳管策略。
- 多连接器、父子拓扑、主备切换、双主冲突和下行唯一路由。
- 连接器关闭、凭据轮换、立即扫描去重、来源缺失和历史保留。
- UI 的 owner/admin/member 边界，以及设备页不承载连接器凭据配置。
