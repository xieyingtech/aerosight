## Purpose

定义 AeroSight 通过 DJI 司空 2 公有云 OpenAPI 完成组织 Token 项目发现、项目选择、加密连接和只读设备目录同步的行为，并确保该模式复用统一 Connector/Device 平台且不冒充 OAuth 或直连控制能力。

## ADDED Requirements

### Requirement: 连接器管理页面采用列表与类型选择流程

系统 SHALL 默认以常规列表展示项目已有的连接器实例，并为项目 owner/admin 提供“新建连接器”入口；用户点击入口后 SHALL 先选择当前可用的连接器类型，再进入对应配置流程。系统 MUST NOT 把尚未完成交付或依赖当前不可用现场设备的接入表单常驻在列表页面。

#### Scenario: 查看已有连接器

- **GIVEN** 项目中存在已启用、失败或已断开的连接器实例
- **WHEN** 用户打开项目连接器页面
- **THEN** 系统以列表展示每个实例的类型、外部项目、状态和最近同步信息
- **AND** 已断开的实例及其历史仍然可见

#### Scenario: 新建司空连接器

- **GIVEN** 项目 owner/admin 位于连接器列表页面
- **WHEN** 用户点击“新建连接器”并选择当前可用的“DJI 司空 2”类型
- **THEN** 系统按需打开司空两阶段 Token 向导
- **AND** 在用户选择类型前不展示 Token 或项目配置表单

#### Scenario: 其他接入方式尚不可用

- **GIVEN** DJI Cloud API 直连或其他接入方式依赖当前不具备的现场设备验收
- **WHEN** 用户查看新建连接器类型
- **THEN** 系统不把这些类型展示为可创建选项
- **AND** 系统不在连接器列表页渲染其内联配置表单

#### Scenario: 只读成员不能进入连接器管理页

- **GIVEN** 当前用户不是项目 owner/admin
- **WHEN** 用户尝试打开连接器管理页面
- **THEN** 系统在渲染列表或新建入口前拒绝访问
- **AND** 不向用户展示实例信息或任何秘密字段

### Requirement: 用户只输入一次 Token 即可选择司空项目

系统 SHALL 为项目 owner/admin 提供两阶段连接向导：先使用用户输入的组织 Token 获取可访问司空项目，再让用户从返回结果中选择一个项目完成连接；系统 MUST NOT 要求用户手工输入项目 UUID。

#### Scenario: Token 可访问多个项目

- **GIVEN** 项目 owner/admin 输入的 Token 可访问多个司空项目
- **WHEN** 用户执行验证并获取项目
- **THEN** 系统展示所有可访问项目的名称和必要标识供用户选择
- **AND** 项目发现请求不持久化 Token

#### Scenario: Token 只可访问一个项目

- **GIVEN** Token 仅可访问一个司空项目
- **WHEN** 项目发现成功
- **THEN** 系统自动选中该项目但仍要求用户确认连接
- **AND** 用户不需要复制项目 UUID

#### Scenario: Token 下没有可访问项目

- **GIVEN** Token 有效但没有可访问的司空项目
- **WHEN** 用户执行项目发现
- **THEN** 系统展示明确空状态且不创建 Connector Instance
- **AND** Token 不被持久化

### Requirement: 项目发现遵循司空项目 API 的最小请求作用域

系统 SHALL 由服务端调用目标区域官方司空项目列表接口，并在项目发现阶段只发送 `X-User-Token` 及协议要求的非秘密请求头；系统 MUST NOT 发送伪造的项目作用域头、浏览器 Cookie 或 AeroSight 会话信息到司空。

#### Scenario: 服务端获取项目列表

- **GIVEN** owner/admin 提交非空 Token
- **WHEN** 服务端请求司空项目列表
- **THEN** 请求发送到部署允许的官方 HTTPS 主机
- **AND** 请求包含 `X-User-Token`、关联 ID和协议要求的分页参数
- **AND** 请求不包含 `X-Project-Uuid`

#### Scenario: 租户尝试指定上游地址

- **GIVEN** 客户端请求包含自定义 base URL、代理或 API path
- **WHEN** 服务端处理项目发现或连接创建
- **THEN** 系统忽略或拒绝这些字段
- **AND** Token 不会发送到部署允许列表之外的主机

### Requirement: Token 不得进入浏览器持久化或可观察输出

系统 SHALL 仅在当前连接表单的内存状态和必要的服务端请求生命周期中处理明文 Token；系统 MUST NOT 将 Token 写入 URL、Cookie、localStorage、sessionStorage、服务端渲染 props、日志、trace、metrics label、审计输入或 API 响应。

#### Scenario: 用户完成连接

- **GIVEN** 用户已发现项目并选择目标项目
- **WHEN** 连接创建成功
- **THEN** 客户端立即清空 Token 和临时项目列表
- **AND** 数据库只保存使用现有 credential envelope 加密的 Token

#### Scenario: 用户取消或离开向导

- **GIVEN** 表单内存中存在 Token
- **WHEN** 用户取消、导航离开或组件卸载
- **THEN** 客户端丢弃 Token
- **AND** 后续页面源代码和浏览器持久化存储中不存在该 Token

#### Scenario: 上游返回包含敏感详情的错误

- **GIVEN** 司空错误响应包含 Token 片段、组织信息或完整请求内容
- **WHEN** 系统映射该错误
- **THEN** 用户、日志、审计和指标只得到归一化错误码及安全摘要
- **AND** 原始错误正文不被持久化

### Requirement: 最终创建前必须重新验证所选项目

系统 SHALL 在加密保存 Token 前重新查询其可访问项目并确认所选项目 UUID 仍属于该结果；客户端项目列表不得作为授权依据。

#### Scenario: 所选项目仍可访问

- **GIVEN** 用户选择的项目仍存在于 Token 可访问列表
- **WHEN** 用户确认连接
- **THEN** 系统在项目级事务中创建司空 Connector Instance、加密凭据和首次同步 outbox
- **AND** 响应不回显 Token 或凭据 envelope

#### Scenario: 客户端伪造项目 UUID

- **GIVEN** 提交的项目 UUID 不在 Token 可访问列表
- **WHEN** 服务端执行最终验证
- **THEN** 系统拒绝创建连接
- **AND** 不持久化 Token、项目 UUID 或部分 Connector Instance

#### Scenario: 发现后权限被撤销

- **GIVEN** 项目发现成功后 Token 的项目权限被撤销
- **WHEN** 用户确认连接
- **THEN** 系统以权限已变化的安全错误拒绝创建
- **AND** 用户可以重新输入 Token 再次发现

### Requirement: 连接器必须复用统一 Connector 和设备身份模型

系统 SHALL 将司空接入注册为 `dji.flighthub2` Connector Definition，并使用现有 Connector Instance、同步运行、外部身份、设备绑定、DeviceType 和 Driver 模型；系统 MUST NOT 创建平行的司空连接、设备或事件账本。

#### Scenario: 创建司空连接器实例

- **GIVEN** 最终项目验证成功
- **WHEN** 系统持久化连接
- **THEN** 实例引用 `dji.flighthub2` Connector Definition
- **AND** 司空项目 UUID 保存为受约束的发现作用域
- **AND** Token 保存于现有加密凭据 envelope

#### Scenario: 重复连接同一司空项目

- **GIVEN** 当前 AeroSight 项目已连接某司空项目
- **WHEN** 用户再次尝试连接相同司空项目
- **THEN** 系统拒绝重复实例或引导更新现有连接
- **AND** 不创建重复外部身份或同步调度

#### Scenario: 不同租户连接相同外部项目标识

- **GIVEN** 两个不同 AeroSight 项目分别提交相同外部项目 UUID
- **WHEN** 系统处理各自连接
- **THEN** 每次读写仍以各自 `project_id` 和团队权限隔离
- **AND** 任一租户不能读取另一租户的 Token、同步运行或设备身份

### Requirement: 持久连接后的设备同步由 Go Worker 执行

系统 SHALL 通过 outbox 和 Connector 租约调度首次、手动及周期同步，并由 Go Worker 使用已保存凭据获取所选司空项目的设备目录；Web 请求 MUST NOT 等待完整设备目录同步。

#### Scenario: 连接创建成功触发首次同步

- **GIVEN** Connector Instance 已在事务中创建
- **WHEN** 事务提交
- **THEN** outbox 中存在一次幂等的首次同步请求
- **AND** 获得该实例租约的单个 Worker 执行同步

#### Scenario: 用户重复点击立即同步

- **GIVEN** 同一 Connector 已存在排队或执行中的同步
- **WHEN** owner/admin 重复请求立即同步
- **THEN** 系统合并或幂等处理重复请求
- **AND** 不并发执行两个相同实例的完整扫描

#### Scenario: Worker 重启

- **GIVEN** 同步期间 Worker 终止且租约最终过期
- **WHEN** 新 Worker 启动并取得租约
- **THEN** 系统从持久游标和同步运行状态安全恢复
- **AND** 不因重放批次重复创建设备身份

### Requirement: 只有完整且可信的受限设备目录才能提交完整快照

系统 SHALL 按中国大陆公有云契约单次获取最多 1000 条设备拓扑，并仅在响应未超限、项目归属一致且通过 schema 校验后提交 `CompleteSnapshot=true`；请求失败、达到无法证明完整性的上限或响应不兼容时 MUST NOT 推进游标或把未返回设备标记为 missing。

#### Scenario: 完整设备目录成功

- **GIVEN** 所选司空项目返回少于官方上限的有效设备目录
- **WHEN** Worker 获取并验证完整响应
- **THEN** 系统以稳定外部身份 upsert 机场和飞行器
- **AND** 仅在事务提交后推进同步游标并处理 missing 身份

#### Scenario: 设备目录限流或失败

- **GIVEN** 设备目录请求返回 429、5xx 或网络错误
- **WHEN** 有限重试仍未恢复
- **THEN** 本次同步运行标记失败并进入有界退避
- **AND** 上次成功快照、游标和已有设备状态保持不变

#### Scenario: 设备目录达到官方返回上限

- **GIVEN** 设备目录返回 1000 条拓扑且接口没有分页或总数证明
- **WHEN** Worker 无法证明响应包含项目全部设备
- **THEN** 本次同步按不完整快照失败关闭并产生脱敏诊断
- **AND** 不推进游标、不处理 missing 身份且不部分提交设备

#### Scenario: 返回跨项目或缺失身份的设备

- **GIVEN** 上游响应包含项目标识不一致、缺少 SN 或重复冲突身份
- **WHEN** Worker 校验响应
- **THEN** 异常对象进入隔离诊断或使批次失败
- **AND** 不覆盖其他项目或其他 Connector 的设备身份

### Requirement: 司空来源默认只读且不得继承直连控制能力

系统 SHALL 将司空 Connector runtime 的可用能力与现有 DJI DeviceType/Driver 能力求交集；在没有已实现并验收的司空下行接口时，系统 MUST NOT 向司空来源设备暴露任务、返航、机场调试、直播或其他控制 action。

#### Scenario: 同步已知 Dock 型号

- **GIVEN** 司空返回可唯一匹配的 Dock 2/3 或配套飞行器型号
- **WHEN** 系统生成设备候选或纳管设备
- **THEN** 系统复用相应 DeviceType/Driver 身份和只读状态能力
- **AND** 不展示直连 MQTT 才支持的控制或直播动作

#### Scenario: 同步未知型号

- **GIVEN** 司空返回未知产品枚举或不兼容固件
- **WHEN** 系统处理该外部身份
- **THEN** 身份保持待确认、冲突或只读不可用状态
- **AND** 不推断或开放任何控制能力

#### Scenario: 同一 SN 已由直连 Adapter 管理

- **GIVEN** 司空同步发现的 SN 已绑定到 `dji` 直连来源
- **WHEN** 系统执行身份匹配
- **THEN** 系统标记多来源冲突并要求管理员处理
- **AND** 不自动合并设备或改变既有下行主路由

### Requirement: 连接生命周期必须安全且可诊断

系统 SHALL 展示连接健康、最近验证、最近同步、同步计数和脱敏错误，并允许 owner/admin 更新 Token、立即同步和断开；member 不得管理连接或查看秘密信息。

#### Scenario: Token 被撤销

- **GIVEN** 已保存 Token 在司空侧被撤销
- **WHEN** Worker 或连接测试收到 401/403
- **THEN** Connector 进入 degraded 或 failed 并停止无意义的快速重试
- **AND** UI 提示管理员更新 Token但不披露上游原始响应

#### Scenario: 更新 Token 成功

- **GIVEN** owner/admin 为现有连接输入新 Token
- **WHEN** 新 Token 仍可访问当前司空项目
- **THEN** 系统原子替换加密 envelope 并请求新一轮同步
- **AND** 旧 Token 不再可由运行时读取

#### Scenario: 更新 Token 验证失败

- **GIVEN** 新 Token 无法访问当前司空项目
- **WHEN** 用户提交更新
- **THEN** 系统拒绝替换凭据并保留旧 envelope
- **AND** 审计只记录失败结果和安全错误码

#### Scenario: 用户断开连接

- **GIVEN** owner/admin 确认断开司空连接
- **WHEN** 系统禁用 Connector Instance
- **THEN** Worker 停止新同步并释放租约
- **AND** 已纳管设备、资产和审计历史保留并按数据新鲜度规则收敛为不可用

### Requirement: 快捷入口必须准确表达授权能力

系统 SHALL 提供打开官方司空 2 页面和获取组织 Token 的操作指引，同时明确当前流程是 Token 接入而不是 OAuth；系统 MUST NOT 声称 AeroSight 已获得 DJI 账号级授权中继能力。

#### Scenario: 用户打开司空快捷入口

- **GIVEN** 用户尚未取得组织 Token
- **WHEN** 用户点击“前往司空 2 获取 Token”
- **THEN** 系统在新窗口打开部署允许的官方司空入口
- **AND** 链接不包含 AeroSight 会话、项目 UUID 或 Token

#### Scenario: 用户查看连接方式说明

- **GIVEN** 用户打开司空连接向导
- **WHEN** 系统展示凭据说明
- **THEN** 页面明确说明 Token 来源、最小权限和轮换责任
- **AND** 页面明确说明当前不是 DJI OAuth 或账号密码登录
