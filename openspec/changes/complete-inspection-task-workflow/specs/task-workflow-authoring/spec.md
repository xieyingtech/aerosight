## Purpose

提供轻量且可演进的巡检 Task 作者契约，使用户能够从空项目创建、校验并发布同一份 YAML 或表单定义，同时保留历史 JSON 工作流的执行语义与审计记录。

## ADDED Requirements

### Requirement: 任务创建与版本契约

系统 SHALL 原子幂等创建 disabled Task 和草稿，支持 aerosight/v2 YAML/JSON 规范化与发布冻结，保留历史 v1 和运行快照；资源及步骤引用必须经过服务端权限和 schema 校验。

#### Scenario: 创建并发布

- **GIVEN** 用户具有项目权限且资源有效
- **WHEN** 从模板创建并发布任务
- **THEN** 生成不可变规范化版本；同幂等键重试不创建第二条 Task，旧版本仍可读取

#### Scenario: 拒绝无效作者输入

- **GIVEN** 源文档含重复键、YAML 别名、自定义标签、超限内容、未知版本、非法 uses 或跨项目资源
- **WHEN** 校验或发布
- **THEN** 给出字段错误并拒绝发布，不产生部分任务或外部动作

### Requirement: 基础表单与 YAML 共用草稿

系统 SHALL 提供模板资源、触发器和研判参数的基础可视化表单，与 YAML 使用同一草稿；表单不能表达的合法内容必须保留，不能静默丢弃。本次不要求完整步骤设计器。

#### Scenario: 双向修改

- **GIVEN** 当前模板定义有效
- **WHEN** 表单修改时间后切换 YAML 并修改模型参数再切回
- **THEN** 两处显示同一执行语义，保存重开后配置一致

#### Scenario: 无法解析或表达的内容

- **GIVEN** YAML 暂时不合法或包含表单不支持的合法自定义步骤
- **WHEN** 切换模式或保存
- **THEN** 保留原文本，非法内容禁止发布；合法扩展不被表单覆盖并提示使用 YAML 编辑

### Requirement: 草稿冲突与启停

系统 SHALL 使用草稿 revision 检查并发修改，启用绑定明确委托者，停用阻止新 Run 而不隐式取消已有 Run。

#### Scenario: 停用活动任务

- **GIVEN** 任务有活动 Run
- **WHEN** 有权用户停用任务
- **THEN** 新的触发被拒绝或跳过，现有 Run 保持原状态

#### Scenario: 过期修订

- **GIVEN** 另一用户已经保存新 revision
- **WHEN** 提交旧 revision 或无权限启用
- **THEN** 拒绝更新并保留当前草稿，不覆盖授权用户
