# media-semantic-search

## Purpose

将项目照片和视频从新增素材、异步解析及自动向量化连接到智能体语义查询和原始证据复核，并保证任务可恢复、索引可重建、素材版本及成员权限始终由业务数据库核验。

## Requirements

### Requirement: Automatic durable indexing
服务 SHALL 自动为可用图片与视频的当前版本建立持久化索引任务，校验原始文件，生成视觉描述和分段向量；服务重启及失败重试不得遗漏素材或并发重复提交。

#### Scenario: New material arrives
- **GIVEN** 当前项目已启用素材语义索引
- **WHEN** 导入或同步产生新的可用图片或视频
- **THEN** 后台自动处理且上传不等待 AI 完成，状态显示排队、处理、完成或失败

#### Scenario: Worker restarts or provider fails
- **GIVEN** 素材索引任务已持久化且尚未完成
- **WHEN** 服务重启或模型服务暂不可用
- **THEN** 任务可恢复并退避重试，失败不可被报告为成功

#### Scenario: Video analysis stops after a completed window
- **GIVEN** 第一个视频窗口的描述及向量已保存到不可变检查点
- **WHEN** 后续窗口分析失败或服务中断后恢复任务
- **THEN** 校验来源后复用首个窗口，只继续尚未完成的窗口

### Requirement: Rebuildable derived projection
系统 SHALL 将素材版本、校验和、模型空间、描述和片段区间存入 PostgreSQL，将向量重建材料存入对象存储；Qdrant SHALL 仅为可重建派生索引。

#### Scenario: Index is lost
- **GIVEN** PostgreSQL 片段事实和对象重建材料仍然可用
- **WHEN** Qdrant collection 被删除并触发重建
- **THEN** 可从既有材料恢复向量而不重新执行视觉分析

#### Scenario: Source becomes stale
- **GIVEN** 素材已存在语义索引
- **WHEN** 素材删除或版本变化
- **THEN** 搜索立即排除旧结果且后台清理其向量

### Requirement: Authorized semantic search
HTTP 搜索与 Agent 搜索 SHALL 固定当前项目，查询后回表验证权限、删除状态、源版本及模型空间。结果 SHALL 包含描述的不确定性、素材引用和视频区间，不能暴露凭据或存储私有地址。

#### Scenario: Another project or obsolete point appears
- **GIVEN** 请求具有当前项目权限且 Qdrant 候选需回表核验
- **WHEN** Qdrant 返回跨项目、已删除或旧版本候选
- **THEN** 不向调用者返回这些候选

#### Scenario: Time filter uses unverified timestamp
- **GIVEN** 素材拍摄时间质量未知或未经验证
- **WHEN** 用户提供绝对拍摄时间范围且素材时间未经验证
- **THEN** 不将上传时间或未验证时间作为拍摄时间匹配

### Requirement: Evidence review workflow
素材库 SHALL 提供语义搜索、索引状态与重试，Agent SHALL 有只读素材搜索工具及稳定证据链接；视频链接 SHALL 定位到匹配片段开始位置。

#### Scenario: Agent returns a video match
- **GIVEN** 智能体已通过只读工具获取授权的素材片段
- **WHEN** 用户打开该证据链接
- **THEN** 打开当前项目素材并定位对应视频区间，媒体访问仍执行权限验证

### Requirement: Local verification and explicit configuration
索引 SHALL 默认关闭；启用需要明确的 Qdrant、embedding 服务及固定模型空间。开发验证 SHALL 使用本地 Docker 与隔离数据库，不对生产执行试验性写入。

#### Scenario: Local E5 mismatches
- **GIVEN** 系统已明确配置固定模型 revision 和向量维度
- **WHEN** embedding 响应维度、revision 或数值异常
- **THEN** 拒绝索引或检索，任务显示失败
