# 参赛材料交付记录

2026-09-14，高校组。定位：完整平台框架，以历史航拍巡检闭环为例。未对外提交。

## 交付文件

产物目录：`.build/msup-demo/deliverables/`。

- `AeroSight项目报告.docx`：9 页，平台架构、模块说明、真实案例截图、验收边界与后续路线；团队、学校和成员待填。
- `AeroSight演示视频1080p.mp4`：1920×1080，16:9，25 fps，H.264/AAC，232.6 秒，中文字幕已烧录。
- `AeroSight解说配音.mp3`：StepFun 配音，带章节停顿。
- `AeroSight演示字幕.srt`：根据 TTS 返回的逐词时间戳编排。
- `制作清单.json`：章节、时长、模型和音色记录。

## 内容与录制口径

主线为浏览器自动化点击录制的成功 Run 3 回放；片头片尾使用说明卡。视频持续标示“已完成运行回放”。操作包括任务 YAML/表单切换、观察与检测查看、研判与修订展开、工单证据回溯、报告及运行结果查看。剪辑调整页面停留时长以配合解说，不代表系统实时处理耗时。

案例为 1 张历史图片、8 个车辆预测、真实模型研判、演示人工复核、1 个待核实工单和报告草稿。未发起真实飞行，不宣称自动取回司空媒体、实机闭环、违法判定或识别准确率。

本次新运行结果如实保留：Run 4 模型超时；Run 5 证据引用校验失败；Run 6 模型超时。三个运行检测均完成。成片使用此前成功 Run 3，未把新失败记录拼接为成功运行。现场实时重跑仍需解决模型超时与引用稳定性。

## 视频章节

| 时间 | 内容 |
|---|---|
| 00:00–00:24 | 平台框架 |
| 00:24–00:46 | 数据与观察范围 |
| 00:46–01:13 | Task 编排 |
| 01:13–01:40 | 真实视觉检测 |
| 01:40–02:08 | 智能体研判 |
| 02:08–02:36 | 人工复核 |
| 02:36–03:00 | 工单与证据回溯 |
| 03:00–03:26 | 报告与运行结果 |
| 03:26–03:53 | 平台扩展与后续工作 |

## 修改与重新导出

正文：`docs/demo/project-report-draft.md`；旁白：`docs/demo/narration.json` 与 `narration-draft.md`。修改旁白后需同步两个源文件，并删除对应 `recording/timed/NN.mp3` 和 `NN.json` 缓存再合成。

在仓库根目录运行：

```sh
.build/msup-demo/yolo-venv/bin/python infra/demo/materials/synthesize.py
.build/msup-demo/yolo-venv/bin/python infra/demo/materials/build_video.py
/Users/hrwen/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3 infra/demo/materials/build_report.py
```

视频重建依赖本机保留的 `.build/msup-demo/recording/take2` 原始浏览器帧及时间索引；报告依赖 `screenshots1080` 截图。不要清理这些目录。TTS 密钥只从忽略的 `.env.tts.local` 读取，端点为 `https://api.stepfun.com/step_plan/v1/audio/speech`，模型 `stepaudio-2.5-tts`，标准音色 `cixingnansheng`。

## 验证

DOCX 已渲染并逐页检查 9 页，中文字体、图注与分页正常。视频已检查各章节抽帧，确认 1920×1080、25 fps、音视频均为 232.6 秒；ffmpeg 完整解码无错误。字幕使用服务返回的时间戳。未完成逐句人工试听。

提交前补齐团队信息，人工观看成片并核对赛事最新附件格式与截止日期。视频不足 5 分钟；这不代表已确认初赛视频时限。
