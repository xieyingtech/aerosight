# sqlc-data-access Specification

## Purpose

定义数据访问的类型契约、可重现生成和数据库行为一致性要求，使新增与迁入查询能够在交付前发现结构不匹配，同时保持租户过滤、事务原子性和时空数据表达的既有语义。

## Requirements

### Requirement: 查询类型与生成一致性

项目 SHALL 为迁入与新增业务查询提供从 SQL 生成的类型化参数与结果；生成产物必须可重现，检查阶段 MUST 发现 SQL、schema 与已提交产物的漂移。

#### Scenario: 生成可重现

- **GIVEN** 工具版本和 SQL/schema 输入固定
- **WHEN** 重复生成并比较结果
- **THEN** 输出一致，不需要人工修改生成文件

#### Scenario: 漂移检测

- **GIVEN** 开发者修改 SQL 但未更新生成产物
- **WHEN** 执行数据层检查
- **THEN** 检查失败并指出漂移，不静默使用旧类型，也不覆盖开发者文件

### Requirement: 数据库契约与 HTTP 解耦

系统 SHALL 保持列的 null、JSON、日期及标识符语义，数据库类型改变不得隐式改变既有 HTTP JSON 契约。

#### Scenario: 空值与 ID 兼容

- **GIVEN** 旧接口有 nullable 字段、空集合和约定的 ID 形状
- **WHEN** 通过新数据层获取并序列化
- **THEN** 字段名、null、空数组、数字/字符串 ID 与旧契约一致

#### Scenario: 项目过滤

- **GIVEN** 查询接收资源 ID 与项目作用域
- **WHEN** 目标 ID 属于其他项目
- **THEN** 返回不可用结果，不省略项目/团队过滤

### Requirement: 共享事务

系统 SHALL 允许现有后台处理与迁入业务在同一个数据库事务内执行查询，任何原子操作不得把一部分写入放到事务之外。

#### Scenario: 混合查询回滚

- **GIVEN** 一个操作包含既有后台 SQL 与生成查询
- **WHEN** 最后一步失败并回滚
- **THEN** 所有写入均不可见，审计/outbox 不残留

#### Scenario: 提交一致性

- **GIVEN** 业务、幂等、审计与事件写入均成功
- **WHEN** 提交事务
- **THEN** 相关记录同时可见，后台可消费对应事件

### Requirement: 时空数据与 schema 一致性

系统 SHALL 保持现有 PostGIS 坐标、空间运算和 JSON 数据语义，查询生成与真实迁移后数据库结构必须一致；数据库迁移仍是独立可审计的操作。

#### Scenario: 时空往返

- **GIVEN** 存在带 SRID 和坐标的空间记录
- **WHEN** 写入后查询 GeoJSON 或坐标投影
- **THEN** 空间参考、位置与现有精度要求一致，不退化为空或无类型数据

#### Scenario: 真实 schema 验证

- **GIVEN** 从空库执行完整迁移
- **WHEN** 执行生成查询和空间测试
- **THEN** 查询可执行，schema 快照与迁移后的结构一致

#### Scenario: 生成不迁移

- **GIVEN** 数据库结构尚未升级
- **WHEN** 运行查询代码生成
- **THEN** 不改变数据库结构；应用仍须通过迁移流程升级

