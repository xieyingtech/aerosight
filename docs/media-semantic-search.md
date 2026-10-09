# 素材语义索引

导入素材后，Go 后台每五秒补齐图片和 MP4 索引任务。视频按十秒分段，各取三帧；默认 AI Provider 生成中文视觉描述，E5 生成归一化向量。PostgreSQL 保存事实和任务，对象存储保存重建 JSON，Qdrant 保存派生向量。

## 本地启用

运行 `docker compose -f infra/compose.semantic.yaml up -d --build`。已有 6333 服务时复用，避免端口冲突。E5 首次启动下载固定 revision 的模型，CPU 推理。也可安装 `infra/embedding/requirements.txt` 后运行 `python infra/embedding/server.py`。

Go 服务使用被忽略的环境文件配置：

```dotenv
SEMANTIC_ENABLED=true
QDRANT_URL=http://127.0.0.1:6333
EMBEDDING_URL=http://127.0.0.1:6335
EMBEDDING_MODEL=intfloat/multilingual-e5-small
EMBEDDING_REVISION=614241f622f53c4eeff9890bdc4f31cfecc418b3
EMBEDDING_DIMENSION=384
```

需要当前数据库启用默认 AI Provider、有效 APP_SECRET 和对象存储配置，原片 checksum_sha256 须存在。Go 镜像已有 ffmpeg；本地需 ffmpeg/ffprobe 位于 PATH。生产私有网络部署时配置 `QDRANT_API_KEY`、`EMBEDDING_API_KEY`，不要公开无认证端口。

## API 与智能体

- `POST /api/projects/:id/media-search`：`{query,limit?,start?,end?}`，start/end 为 RFC3339 区间，必须同时提供。仅可信拍摄时间参与过滤。
- `GET /api/projects/:id/assets/:assetId/semantic-index`：状态、次数、安全错误码和片段数量。
- `POST /api/projects/:id/assets/:assetId/semantic-index/retry`：项目 owner/admin 重新排队；已有材料时直接重建。运行中的任务不重复启动。
- Agent `search_media`：同样参数，项目由当前会话确定，不接受模型传入 scope ID。

上传不等待模型。故障最多自动尝试十次，退避最长一小时，之后由管理员重试。单片最大 512 MB、最长一小时；超限明确失败，不截断成成功。模型描述需要原片复核，匹配分数不是违规置信度。缺少可信时间时只能按相对视频区间引用，不猜测时间或坐标。

每个视频分段完成后保存不可变检查点；处理中断或失败后复用已完成的分段，继续剩余窗口。检查点只有在素材版本和校验和仍一致时可恢复。

Qdrant 不可用不阻断上传；搜索返回 503。删除或版本变更立即由查询回表过滤，后台最终清理旧向量。collection 丢失后，创建 collection 并逐素材调用重试从对象材料恢复，无需再次视觉分析。切换 E5 配置会创建新空间并重新生成索引。

开发取样、生产只读复制脚本和凭据不属于运行时，放在忽略目录，不纳入提交。
