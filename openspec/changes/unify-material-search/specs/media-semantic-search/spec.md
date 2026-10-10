## MODIFIED Requirements

### Requirement: Evidence review workflow
素材页 SHALL 使用顶部全宽的单一搜索框同时查询名称、来源与内容，表格列表每素材一行，文字匹配优先于纯内容匹配，其中等值匹配优先于包含匹配。列表 SHALL 不内嵌素材预览，点击素材进入独立详情；详情 SHALL 提供预览、下载、来源、索引状态和重试。Agent SHALL 有只读素材搜索工具及稳定证据链接；视频链接 SHALL 定位到匹配片段开始位置。导航 SHALL 使用“素材”和“算法”，素材列表标题 SHALL 为“素材”。

#### Scenario: Unified results
- **GIVEN** 当前项目有名称/来源匹配素材及语义命中片段
- **WHEN** 用户输入一个关键词
- **THEN** 先展示等值与包含文字匹配，再按语义顺序补充其他素材，同素材多个区间集中在一行，类型过滤同时作用两类结果

#### Scenario: Search unavailable or outdated
- **GIVEN** 文字匹配已展示且内容请求仍在执行
- **WHEN** 内容搜索失败或用户改变或清空关键词
- **THEN** 失败保留文字结果并提示，旧请求不得覆盖新搜索，清空恢复所有素材

#### Scenario: Open material detail
- **GIVEN** 用户浏览表格列表或匹配片段
- **WHEN** 点击素材或片段并随后返回列表
- **THEN** 独立详情展示授权素材与原始信息，片段定位保留，返回列表保留搜索与分类

#### Scenario: Agent returns a video match
- **GIVEN** 智能体已通过只读工具获取授权的素材片段
- **WHEN** 用户打开新链接或既有素材列表证据链接
- **THEN** 打开当前项目素材详情并定位对应视频区间，媒体访问仍执行权限验证

#### Scenario: Detail source unavailable
- **GIVEN** 详情所指素材已删除或不属于授权项目
- **WHEN** 打开详情
- **THEN** 显示素材不可用，不回退展示其他素材
