# 本地 YOLO 算法 API

独立的演示服务：FastAPI + Ultralytics YOLO11n COCO，默认 Apple MPS，否则 CPU。仅监听 `127.0.0.1:8091`，不启动 AeroSight 主服务、调度器或飞行控制。算法执行真实推理，平台接入流程尚需另行验收。

## 启动与测试

在项目根目录执行：

```sh
uv venv --python 3.12 .build/msup-demo/yolo-venv
uv pip install --python .build/msup-demo/yolo-venv/bin/python -r infra/demo/yolo/requirements.lock.txt
infra/demo/yolo/start.sh
```

首次启动自动下载官方 `yolo11n.pt` 到 `.build/msup-demo/models/` 并预热。用 `YOLO_DEVICE=cpu infra/demo/yolo/start.sh` 显式切换 CPU。端口可通过 `YOLO_PORT` 修改。锁文件记录当前 macOS 环境版本。

- `GET /healthz`：模型、设备与权重 SHA256。
- `/docs`：Swagger，可展开 `POST /predict` 上传真实图片。
- `POST /predict`：multipart `file`，快速测试。
- `POST /infer`：接收 AeroSight 风格的 JSON 输入，返回 `result.kind=detection` 和 `result.detections`。

```sh
.build/msup-demo/yolo-venv/bin/python infra/demo/yolo/smoke.py
```

测试会使用已下载的六张 RGB 图片，结果及标框缩略图写入 `.build/msup-demo/yolo-results/`。检测类别为 person、bicycle、car、motorcycle、bus、truck；默认 confidence=0.25、imgsz=1280。检测框为原始图片像素坐标，未进行地理坐标投影。DJI JPEG 内含 MPO 多图结构，读取默认主图；不进行 EXIF 旋转以保持原始像素坐标。

## JSON 接入

```json
{
  "schemaVersion": "aerosight.algorithm.input/v1",
  "runId": "local-demo-001",
  "inputAsset": {"checksumSha256": "填入原图真实SHA256"},
  "parameters": {
    "localAsset": "DJI_20260818151715_0012_V.jpeg",
    "confidence": 0.25,
    "imgsz": 1280
  }
}
```

`localAsset` 是本地演示专用扩展，只允许读取 `.build/msup-demo/source-media/rgb/images/` 内的文件。正式路径应省略此字段，并传入 `inputAsset.accessUrl`（HTTPS 签名地址）和真实 SHA256。启动时用 `YOLO_ASSET_HOSTS` 配置可信资产域名，多个域名用逗号分隔；默认禁止远程拉图、禁止重定向。图片限 40 MiB，JPEG/MPO/PNG。

AeroSight 的 http-json 映射：`kind=detection, resultPath=result.detections`。旧字段映射可使用 `detectionsPath=result.detections`、`keyPath=detectionKey`、`labelPath=label`、`confidencePath=confidence`、`geometryPath=pixelGeometry`。输出还包含 `modelRevision`、`modelDigest` 和推理耗时。

平台当前要求算法 Provider 使用 HTTPS 并限制私网地址；这个本地 HTTP 地址不能直接在正式 Provider 表单注册。下一步需要可信 HTTPS 部署或限定的开发接入方案；本次未放宽全局安全策略、未注册数据库 Provider、未验证平台 Task Run。服务无认证，仅用于本机测试，不应直接暴露公网。

当前后台进程 PID 记录在 `.build/msup-demo/yolo-server.pid`，日志在 `.build/msup-demo/yolo-server.log`。重启前确认该 PID 仍对应本服务，再停止它并运行启动命令。
