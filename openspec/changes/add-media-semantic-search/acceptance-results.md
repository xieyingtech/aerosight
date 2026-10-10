# 验证记录

日期：2026-10-09。所有素材写入、队列处理、索引删除及重建仅发生在本地隔离数据库 `aerosight_semantic_clean`、本地对象目录和 Docker Qdrant，未对生产业务数据库或 OSS 写入。

## 已验证

- 两段既有视频副本经正式 Go 后台扫描自动分析；E5 使用 `intfloat/multilingual-e5-small`、revision `614241f622f53c4eeff9890bdc4f31cfecc418b3`、384 维归一化向量。
- collection 为 `aerosight_media_ad389c4c110e08e1`。搜索“林间石阶道路”第一候选为航拍片段，搜索“紫色背景黄色热成像区域”第一候选为热成像片段。分数仅代表语义相似度。
- 正式 Go 服务启动后，通过上传 API 新增 18.018 秒视频，后台自动完成两段索引，attempts=1；第二片段为 10000–18018ms，无需调用手工索引命令。
- 删除本地 collection 后，通过管理员重建接口恢复既有材料；自动化测试确认视觉分析调用次数不增加。
- 本地软删除后搜索立即排除旧素材，后台随后删除 Qdrant 点。自动化测试也覆盖版本变化和物理删除 tombstone 清理。
- 真实默认 Provider `step-5-preview` 成功调用 `search_media`，返回片段链接；聊天记录保留 4 项证据引用。发现并修正数字 ID 被聊天保留策略过滤的问题，新增回归断言。
- 浏览器验证素材库搜索、可搜索状态、片段引用和原片预览；实际 video.currentTime=10、duration=18.018，定位正确。验收截图及工具输出存于被忽略的 `.build/`。

## 自动化和构建

- `go test -tags dev ./internal/semantic ./internal/config ./internal/runtime`：通过。包含模型空间、错误向量、时间质量、Provider/Qdrant 故障、材料重建、并发锁及第十次运行中任务恢复。
- 分段检查点测试使用 ffmpeg 生成 11 秒测试视频，模拟第二窗口视觉服务失败；首窗口材料保持有效，重试仅新增一次视觉调用并完成两段索引。相关 semantic 测试再次通过。
- HTTP 相关 `TestMediaSearch*`、`TestChatResponsesToolLoop`：在真实本地 PostgreSQL/PostGIS 上通过。包括 scope 注入、成员撤权、证据持久化、时段链接及只读工具循环。
- `TestSchemaSnapshotMatchesFullMigration`：通过；迁移目录与 schema 快照一致。
- `pnpm typecheck`、`pnpm db:check`、`pnpm build`：通过；后续 Go 修正再次执行相关测试与 `pnpm build:server`，通过。
- `pnpm check`：已完整执行，既有 `TestReportAggregateIncludesAllEvidence` 失败：测试仍期待旧报告资源路径，当前实际为 `/projects/1/tasks/runs/1/`。其余输出未报告失败。本次未修改该既有断言，不把完整检查声称为通过。
- 变更与主规范分别执行 OpenSpec 严格验证，通过。任务全部完成，保留活动 change 及既有完整检查失败记录，未归档。

## 当前边界

仅支持 JPEG、PNG、MP4，单片最大 512MB、最长一小时。未知或未经验证的拍摄时间不匹配绝对时间筛选；缺少位置事实时不会从画面推断坐标。索引默认关闭，2026-10-10 按用户授权在已有线上项目启用。临时取样、导入和验收脚本与凭据均在忽略目录。

## 2026-10-10 线上部署与实测

本节是用户明确授权后的线上 API 验收，区别于上述本地隔离验收；仅通过应用正式接口上传合成视频、创建只读智能体会话及算法运行，没有直接写业务数据库、迁移、删除数据或重建生产 collection。

- 在既有 Zeabur 项目新增 Qdrant v1.19.0 和 E5 CPU 服务；Qdrant 配置 `/qdrant/storage` 持久化卷，两个服务均只提供私网访问并分别配置独立随机密钥。
- Qdrant `/readyz` 200、未鉴权 `/collections` 401、鉴权后 200。E5 `/health` 返回固定模型/revision/384 维；实际带鉴权调用 `/v1/embeddings` 返回 384 维向量。E5 Docker 部署状态 RUNNING 后仍等待模型加载完成，再启用应用配置并重启。
- 正式上传 API 返回 201，合成素材 `online-check-blue-20261010.mp4`（11 秒纯蓝、不含业务或个人内容），assetId=5679。原片签名访问 200、OSS Range 请求 206，MP4 头有效。上传与索引异步。
- 测试素材在历史队列之后于 07:02:05 UTC 自动 indexed，attempts=1、segments=2、errorCode=null。搜索“纯蓝色画面，没有任何物体”返回 200，前两项均为该素材，区间分别为 10000–11000ms、0–10000ms；没有手动执行向量写入或数据库排队操作。YOLO 标注接口返回 200，读取到三帧结果。
- 项目列表、snapshot、素材、设备、算法定义/运行、案件、任务、AI Provider 配置，以及 FlightHub 连接、活动、直播元数据、地理数据和模型列表均返回 200。此项只验证读取链路，不代表真实飞行、直播画面或设备控制完成。
- 索引启用前搜索返回 503、素材状态 disabled；补齐依赖后搜索返回 200，已有图片/视频自动完成索引。智能体项目查询成功执行 query_devices/query_issues/query_tasks；语义检索成功执行 search_media，聊天历史保存素材及片段引用。此前依赖未启用时一次智能体检索返回 AGENT_SESSION_FAILED，启用后重试成功。
- 额外视频 YOLO 检查首次失败：远端 `/infer` 返回 400，允许列表未包含当前对象存储域名。保留原有域名后添加已验证的签名素材域名并重启 YOLO，经正式 retry API 创建新运行 `d6f045e2-4934-4b9d-b095-04c0ec4844f9`，最终 succeeded；11 秒、0.2 FPS、3/3 帧、complete=true。零目标的纯蓝视频只能验证执行与标注链路，不能证明目标识别精度。
- 完整检查仍有上述既有报告引用断言失败，未归档变更。线上浏览器观察超时，线上验收以真实 HTTP/私网调用及持久化状态为依据。

## 同日补充线上浏览器验收

用户要求延长超时并继续检查，采用 120 秒操作超时后成功完成以下实际 UI 操作，替代上述浏览器超时限制：

- 素材库显示 21 个素材，搜索“纯蓝色画面，没有任何物体”后测试视频排在首位，标记为内容匹配，链接包含 10000–11000ms 区间。
- 点击搜索结果打开视频详情，显示“内容索引：可搜索”和“2 个片段”。实际 video.currentTime=10、duration=11、readyState=4，无播放器错误；说明原片已加载并准确定位。
- 智能体页面显示已持久化的项目查询及两次成功语义检索。正文中的地址有时由模型写为代码文本；展开最后一条 search_media 查询完成后，相关依据显示可点击 asset:5679@v1 链接。实际点击第一条依据跳转原片，再次确认 currentTime=10、duration=11、readyState=4。初步认为引用缺失的判断已更正，没有修改渲染代码。
- 算法列表显示合成视频重试运行成功（3 帧），与后台结果一致；保留首次失败记录。浏览器捕获的错误日志为空。
- 截图保存在忽略目录 `.build/online-video-seek.jpg`、`.build/online-agent-evidence-seek.jpg`、`.build/online-algorithm-status.jpg`，未新增测试素材、会话或运行，未执行设备动作。
