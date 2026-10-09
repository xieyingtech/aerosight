## ADDED Requirements

### Requirement: Compatible compact persistence
系统 SHALL 将八类作业存入一张物理表，保留类型约束、幂等键、项目隔离、原始 UUID 与恢复检查点，并合并已明确的一对一或一对多关系。

#### Scenario: Existing records upgrade
- **WHEN** 旧数据库应用精简迁移
- **THEN** 作业与证据可回查，未启用模块的旧数据归档，迁移失败原子回滚

#### Scenario: Project isolation
- **WHEN** 作业或证据引用其他项目的对象
- **THEN** 数据库约束或项目限定查询拒绝该引用

### Requirement: Historical actors do not lock membership
系统 SHALL 保留历史用户引用，执行操作时仍检查当前权限，历史记录不能阻止成员离队。

#### Scenario: Member leaves
- **WHEN** 操作者已有历史作业或审批并被移出团队
- **THEN** 历史记录保留，新操作不得沿用已失效成员权限

### Requirement: Usage-backed retirement preserves legacy access
系统 SHALL 按生产调用、处理器入口、外键与事务状态消费审计全部表，迁移兼容查询的数据并删除无必要独立关系，保留原始记录归档。

#### Scenario: Retired media and feedback upgrade
- **WHEN** 含上传意向、已发布证据、旧事件反馈及派生来源的数据升级
- **THEN** 原下载文件名、敏感下载限制、已发布素材保护、反馈字段与时间顺序、派生重试及项目约束保持可验证

#### Scenario: State gates remain active
- **WHEN** 表仅通过 UPSERT、唯一键、外键或自动分区参与业务
- **THEN** 审计不能仅因缺少单独 SELECT 或管理写接口将其判为无用表

### Requirement: Active one-to-one extensions share parent storage
系统 SHALL 将每命令最多一条的协议关联、以素材 ID 为主键的访问引用、以观测 ID 为主键的位置记录存于父表，有类型约束、来源身份及历史字段保持可回查。

#### Scenario: Existing extension rows upgrade
- **WHEN** 含完整协议回执、加密访问引用、设备/时间与父观测不同的位置记录升级
- **THEN** 原 ID、时间、状态、凭据密文、位置/姿态/精度全部保留，视图不存独立副本

#### Scenario: Runtime writes and extension deletion
- **WHEN** 位置上报或扩展被删除
- **THEN** 位置与观测一次 INSERT，删除扩展不删除命令/素材/观测父记录，跨项目来源仍拒绝

#### Scenario: Private asset access
- **WHEN** 读取公开素材列表、回放或详情
- **THEN** 响应不包含私有访问密文、摘要或定位信息，远程受控访问使用原资产 ID 解密

### Requirement: Video intermediates live in object storage

系统 SHALL 将普通分析帧、帧诊断和完整时间轴存入对象存储，不为普通帧建立素材或逐次调用数据库记录。

#### Scenario: Video analysis
- **WHEN** 视频按指定帧率分析
- **THEN** 素材目录不随帧数增长，结果播放仍可获得完整动态框选时间轴
