## Purpose

定义与业务类别无关的算法服务注册、配置快照、运行和结果映射能力，使各种识别、检测、分割、跟踪、OCR 或自定义分析服务可以通过统一目录动态呈现在 UI 中，同时明确模型版本由算法 Provider 管理。

## ADDED Requirements

### Requirement: 通用算法 Provider
系统 SHALL 允许项目管理员配置算法 Provider，包括协议类型、服务地址、认证秘密引用、健康检查、超时、并发和网络策略；Provider MUST 不包含“违建”等固定业务类别语义。Provider 的 API 协议版本（例如 URL 中的 `/v1`）MUST 不得被平台解释为模型版本。

#### Scenario: 注册任意识别服务
- **GIVEN** 管理员具有算法管理权限并提供可支持协议的服务配置
- **WHEN** 健康检查和安全校验通过
- **THEN** 系统保存脱敏 Provider 配置并允许在其下创建算法定义

#### Scenario: 不安全服务地址
- **GIVEN** Provider 地址违反项目出站网络策略或秘密无效
- **WHEN** 管理员测试或启用 Provider
- **THEN** 系统 SHALL 拒绝启用、记录脱敏原因且不得泄露秘密

### Requirement: 算法定义配置与 Provider 模型版本边界
系统 SHALL 允许管理员直接保存任意算法的名称、描述、能力类别、Provider 模型/进程标识、输入模式、参数模式、输出模式、结果映射和展示元数据，保存后 SHALL 立即成为新运行使用的当前配置。模型 revision、digest 或等效模型版本 MUST 由 Provider 管理和报告；AeroSight MUST 不托管模型制品、不生成业务模型版本，也不得要求管理员发布算法版本。系统 SHALL 在每次保存时生成内部不可变 Configuration Snapshot，任务和运行 MUST 引用明确快照以保留历史来源，但该快照序号和发布状态 MUST 不作为管理员操作流程。

#### Scenario: 定义通用目标检测算法
- **GIVEN** Provider 可接收图像并返回目标集合
- **WHEN** 管理员完成输入输出映射测试并保存
- **THEN** 系统立即将该配置用于新运行并在内部保存不可变快照，类别标签来自定义或服务元数据而非平台源码

#### Scenario: 更新生产模型
- **GIVEN** Provider 已将生产模型切换到新的 revision，且历史运行引用旧配置快照
- **WHEN** 管理员按需修改 Provider 模型标识或结果映射并保存
- **THEN** 系统更新当前配置、为后续运行生成新快照，并记录 Provider 返回的 revision/digest；历史运行的算法、参数和结果来源不得被改写

#### Scenario: Provider 未返回模型 revision
- **GIVEN** Provider 仅暴露稳定 API 路径且未返回模型 revision 或 digest
- **WHEN** 算法运行完成
- **THEN** 系统 SHALL 记录所用配置快照并明确标记 Provider 模型 revision 未提供，不得把 `/v1` 等 API 路径伪装成可复现的模型版本

### Requirement: UI 使用动态算法能力目录
算法列表、创建运行和结果展示 UI SHALL 从服务端返回的算法定义、输入/参数模式与展示元数据生成，不得写死违建、人数统计或其他具体识别类型；示例模板 MUST 可删除且不影响平台功能。

#### Scenario: 新算法无需修改 UI 源码
- **GIVEN** 管理员保存一个符合支持契约的 OCR 算法定义
- **WHEN** 用户打开算法目录并选择兼容输入资产
- **THEN** UI 自动展示该算法、参数表单和可运行状态，无需发布新的前端版本

#### Scenario: 管理员直接修改算法配置
- **GIVEN** 管理员需要调整模型标识、输入参数或结果映射
- **WHEN** 管理员保存算法配置
- **THEN** UI SHALL 不要求创建、发布或选择平台算法版本，保存后的配置立即用于新运行

#### Scenario: 删除违建示例
- **GIVEN** 项目删除或禁用疑似违建示例定义
- **WHEN** 用户刷新算法目录
- **THEN** 该项目不再展示此算法，其他 Provider、定义和通用页面保持可用

### Requirement: 通用算法运行契约
每次算法调用 SHALL 创建不可变运行记录，包含项目、定义配置快照、Provider 报告的模型 revision/digest（如有）、输入资源版本、参数、幂等键、状态、尝试、外部任务标识、原始结果引用和标准化结果；运行状态至少覆盖排队、执行、成功、失败、取消和超时。

#### Scenario: 成功执行算法
- **GIVEN** 用户选择兼容输入和当前可用算法定义
- **WHEN** Worker 调用 Provider 并收到有效结果
- **THEN** 系统保存定义配置快照、Provider 模型 revision/digest（如有）、原始结果引用和标准化结果，并向项目发布状态变化

#### Scenario: 重复回调或超时
- **GIVEN** 外部服务重复回调或在期限内未完成
- **WHEN** Worker 处理回调或超时
- **THEN** 系统 SHALL 幂等收敛一次，按策略重试或失败且不得生成重复业务事件

### Requirement: 可扩展标准结果类型
系统 SHALL 支持分类、检测、分割、关键点、跟踪、OCR、标量/表格和自定义资产等标准结果类型，并保留 Provider 原始结果；只有显式配置的结果映射或事件规则才能赋予领域语义。

#### Scenario: 检测结果映射为业务事件
- **GIVEN** 算法返回通用多边形、标签和置信度且项目规则将特定标签映射为事件
- **WHEN** 运行成功并触发规则评估
- **THEN** 系统保存通用检测来源后创建相应业务事件，明确区分模型输出与规则结论

#### Scenario: 未知自定义输出
- **GIVEN** Provider 返回当前无标准映射的扩展字段
- **WHEN** 运行完成
- **THEN** 系统 SHALL 保全受限原始结果并将运行标记为需要映射或部分成功，不得伪装成“无检测结果”

### Requirement: 算法权限与设备控制隔离
算法运行 SHALL 受项目授权和资源兼容性校验；算法结果或 Provider callback MUST 不得直接控制设备，后续设备操作必须通过版本化规则、任务草案、人工确认和统一命令入口。

#### Scenario: 算法建议后续巡查
- **GIVEN** 规则根据算法结果生成后续巡查建议
- **WHEN** 建议满足项目策略
- **THEN** 系统创建可审查任务草案，只有授权用户确认后才进入设备命令流程

#### Scenario: Provider 尝试注入控制指令
- **GIVEN** 算法响应包含未声明的设备控制字段
- **WHEN** 结果映射处理响应
- **THEN** 系统 SHALL 将字段视为非可信扩展或拒绝结果，且不得创建或发布设备命令
