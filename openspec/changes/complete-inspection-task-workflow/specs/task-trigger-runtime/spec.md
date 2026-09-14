## Purpose

约束首期手动与定时触发的共同输入、身份和幂等行为，并明确跨平台调度的单一所有者，使定时巡检不会因重复扫描、发布新版本或服务恢复而重复发起飞行。

## ADDED Requirements

### Requirement: 手动和定时输入

系统 SHALL 支持手动运行已发布 Task 及带时区的 schedule；按默认值、触发配置和本次输入顺序合并并递归校验，手动试跑不改计划。旧其他触发来源保持兼容，不新增全面重构要求。

#### Scenario: 立即试跑计划

- **GIVEN** 已发布任务包含 cron 和航线 inputs
- **WHEN** 用户立即运行并覆写允许的输入
- **THEN** Run 保存实际输入及固定版本，下次计划不变

#### Scenario: 错误输入

- **GIVEN** 必填参数缺失或类型不符
- **WHEN** 手动或定时触发
- **THEN** 拒绝创建可执行 Run，记录字段错误，不向外部下发

### Requirement: 调度防重与可撤销授权

系统 SHALL 按任务及 UTC occurrence 防重，跨版本限制活动并发为 1；执行时重检启停与委托用户权限。超过 60 秒的错过计划记录 missed 而不补飞，坏计划不能阻塞其他计划。

#### Scenario: 重复扫描与发布竞争

- **GIVEN** 同一 Task 的旧版本仍运行且多个调度实例竞争
- **WHEN** 重复处理 occurrence 或发布新版后触发
- **THEN** 重复 occurrence 返回同一 Run，新 occurrence 按并发规则跳过并留记录

#### Scenario: 停机或撤权

- **GIVEN** 计划在停机期间错过，或用户已撤权/无明确委托者
- **WHEN** 服务恢复或再次调度
- **THEN** 不补飞，不回退项目创建者；分别记录错过或授权错误，其他有效计划仍运行

### Requirement: 唯一调度归属

系统 SHALL 对新飞行模板固定 AeroSight 为调度所有者，每次仅创建一次司空 immediate 飞行。首期既有司空周期计划仅作为已完成架次的手动输入，不另建自动订阅触发器。

#### Scenario: 计划触发一次架次

- **GIVEN** AeroSight 飞行模板启用 schedule
- **WHEN** 一个 occurrence 到达
- **THEN** 一个业务 Run 最多提交一个 immediate 飞行，不创建司空 recurring/timed 计划

#### Scenario: 混用调度模式

- **GIVEN** 新飞行模板要求司空 recurring 或两个 schedulerOwner
- **WHEN** 发布定义
- **THEN** 明确拒绝；选择司空已完成架次只执行分析，不新增飞行
