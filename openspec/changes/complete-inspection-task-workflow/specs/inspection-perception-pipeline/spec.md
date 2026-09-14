## Purpose

将司空原生告警与现有外部识别服务统一为可追溯的巡检证据，记录分析范围、数据缺失和坐标来源，使后续研判能够区分真实零检测、未执行检测和局部观察。

## ADDED Requirements

### Requirement: 不可变观察输入

系统 SHALL 为 existing-flight/assets/flighthub-flight 建立观察清单，保留不可变资产版本及来源；资产引用必须授权，不改写原 task_run_id。

#### Scenario: 重复分析既有照片

- **GIVEN** 当前项目资产可读
- **WHEN** 多个 Run 分别选择同一照片
- **THEN** 每个 Run 有独立观察绑定，原资产归属与内容版本不变

#### Scenario: 归属错误

- **GIVEN** 收到另一 flight 的媒体或跨项目 assetId
- **WHEN** 绑定观察输入
- **THEN** 拒绝误绑，不能按当前活动步骤猜测归属

### Requirement: 显式识别来源和完整性

系统 SHALL 支持 flighthub-ai 与 external 两种显式来源，记录实际算法/模型及范围；外部识别复用既有服务并汇总全部选定输入。检测失败不能当零检测，来源不可静默切换。

#### Scenario: 真实外部检测

- **GIVEN** 已配置适用服务且输入清单封闭
- **WHEN** 对选定真实图片执行识别
- **THEN** 记录每张实际调用及版本，全部成功才标范围完成，合法零结果保留

#### Scenario: 原生告警不可证明无异常

- **GIVEN** 查询得到零告警但无法确认目标算法启用，或部分输入失败
- **WHEN** 生成证据集
- **THEN** 标 unavailable/partial 并要求复核或失败，不输出完整无异常结论

### Requirement: 坐标与业务适用性

系统 SHALL 标记 target/capture/unknown 的位置来源与质量，未知模型版本及范围必须明示；不得将照片 GPS、告警 target_value 或人车船检测解释为精确目标位置、通用置信度或违建证明。

#### Scenario: 只有拍摄位置

- **GIVEN** 告警提供机载/照片位置
- **WHEN** 地图和研判展示证据
- **THEN** 展示拍摄或未知位置标签，不自动跨图合并地物

#### Scenario: 业务依据不足

- **GIVEN** 只有单期建筑影像或不适用的目标检测
- **WHEN** 研判新增或违法属性
- **THEN** 保留证据限制，给出疑似线索或待复核，不认定已证实新增违建
