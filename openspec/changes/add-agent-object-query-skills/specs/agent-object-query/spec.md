## Purpose

将巡检人员对影像目标的查询、证据检查和复核经验封装为实际可加载的技能，支持用户用自然语言查询已有检测结果、逐步筛选并查看来源明确的目标框和待核查清单。

## ADDED Requirements

### Requirement: Load trusted inspection skills
系统 SHALL 允许 Agent 加载白名单内置技能，并提供版本、专业操作流程和工具调用证据。

#### Scenario: Skill enters model context
- **GIVEN** 用户提出巡检目标查询
- **WHEN** Agent 加载目标查询技能
- **THEN** 模型收到完整规则，会话显示技能名称和版本。

#### Scenario: Unknown skill denied
- **GIVEN** 模型提供任意文件路径或未知技能名
- **WHEN** 请求加载
- **THEN** 系统拒绝读取，不访问该路径。

### Requirement: Query verified detection candidates
系统 SHALL 仅查询用户有权访问的当前项目检测结果，支持类别、置信度和明确目标 ID 筛选，区分未完成、失败、零匹配和截断。

#### Scenario: Explicit empty selection
- **GIVEN** 成功运行包含检测目标
- **WHEN** selectedDetectionKeys 是空数组
- **THEN** 返回零个目标而非全部目标，并保留原始运行依据。

#### Scenario: Forged or cross-project identity
- **GIVEN** 运行属于其他项目或目标 ID 不存在
- **WHEN** 查询或选择
- **THEN** 拒绝查询，不泄露其他项目数据或创造目标框。

### Requirement: Visual review preserves input identity
系统 MUST 在向模型提供图像前检查资产权限、运行输入版本和 checksum，限制图像大小和候选裁剪数量；图像不可用时 SHALL 明确禁止视觉属性结论。

#### Scenario: Asset changed after detection
- **GIVEN** 识别完成后资产内容或版本变化
- **WHEN** 请求视觉复核
- **THEN** 检测数据仍可查，图像不送入模型且给出明确原因。

### Requirement: Reproducible selected boxes
系统 SHALL 提供能够重开和查看选中框的稳定结果链接，显示筛选理由并允许恢复所有目标、导出待核查清单；结果不认定违法。

#### Scenario: Follow-up selection
- **GIVEN** Agent 返回经验证的选中目标 ID
- **WHEN** 用户打开结果链接
- **THEN** 页面只叠加这些框，显示数量、筛选理由和运行来源。

#### Scenario: Invalid link selection
- **GIVEN** 链接含不存在的目标 ID
- **WHEN** 打开结果页面
- **THEN** 显示失效提示，不默认为全选。
