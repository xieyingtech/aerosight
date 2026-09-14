## Purpose

使巡检 Task 在创建案件之前调用真实模型完成有证据约束的业务研判，并通过结构化决策和人工复核保护不确定结论，同时保持已有案件 Copilot 的兼容行为。

## ADDED Requirements

### Requirement: 无案件的结构化研判

系统 SHALL 支持 copilot.run assessment 模式以 evidenceSetId 运行，输出 create/update/no_issue/needs_review 决策及证据引用、理由和缺失信息；保留旧 issue 模式。

#### Scenario: 空案件库

- **GIVEN** 真实识别证据可读且 AI Provider 已配置
- **WHEN** 执行 assessment
- **THEN** 无需 issueId 即可调用真实模型并记录 provider/model、模板、原始输出及合法结构

#### Scenario: 非法输出

- **GIVEN** 模型返回非法 JSON、捏造证据或越权案件引用
- **WHEN** 提交研判结果
- **THEN** 校验失败且不进入案件写入，不用固定结果替代模型

### Requirement: 研判范围及权限

系统 SHALL 限制模型到当前授权证据和只读工具，保留真实错误；只有已完成目标检测的明确范围可给出该范围 no_issue，资料中的指令不得成为权限来源。

#### Scenario: 明确范围无检测

- **GIVEN** 全部选定图片检测成功且结果为零
- **WHEN** 完成 AI 研判
- **THEN** 可输出该已分析范围 no_issue，报告不扩张为未观测区域结论

#### Scenario: 资料注入或服务失败

- **GIVEN** 图片/告警包含指令、用户撤权或 Provider 超时
- **WHEN** 模型执行
- **THEN** 不执行越权工具；撤权/超时明确失败，不能伪造无异常结论

### Requirement: 版本化人工复核

系统 SHALL 在任一 needs_review 时暂停整批案件写入；人工确认、调整或驳回生成新 revision，保留原文，以幂等复核恢复到后续步骤。

#### Scenario: 确认线索

- **GIVEN** 有权用户查看待复核证据
- **WHEN** 确认并提交当前 revision
- **THEN** 保存人工决策并继续案件步骤，不重飞、不重复识别

#### Scenario: 重复或过期复核

- **GIVEN** 复核已完成、revision 过期或 Run 已取消
- **WHEN** 再次提交或普通 resume
- **THEN** 同键返回既有结果，过期/取消拒绝变更；resume 不绕过未完成复核
