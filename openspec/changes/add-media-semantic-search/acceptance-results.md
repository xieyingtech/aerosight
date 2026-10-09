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

仅支持 JPEG、PNG、MP4，单片最大 512MB、最长一小时。未知或未经验证的拍摄时间不匹配绝对时间筛选；缺少位置事实时不会从画面推断坐标。索引默认关闭，线上尚未启用或部署。临时取样、导入和验收脚本与凭据均在忽略目录。
