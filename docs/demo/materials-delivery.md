# AeroSight 产品介绍版材料

本版聚焦平台概念、业务流程、模块协同与创新设计。报告共 10 页；视频 3 分 39 秒，1920×1080、25 fps，包含 StepFun 配音与中文字幕。

## 文件

目录：`.build/msup-demo/deliverables/`。

- AeroSight项目报告.docx
- AeroSight演示视频1080p.mp4
- AeroSight演示字幕.srt
- AeroSight解说配音.mp3
- AeroSight封面.png
- 制作清单.json

## 章节

| 时间 | 内容 |
|---|---|
| 00:00–00:22 | 平台愿景 |
| 00:22–00:48 | 时空数据 |
| 00:48–01:14 | 任务编排 |
| 01:14–01:39 | 视觉感知 |
| 01:39–02:01 | 智能体研判 |
| 02:01–02:26 | 人机协同 |
| 02:26–02:49 | 工单协作 |
| 02:49–03:15 | 报告汇总 |
| 03:15–03:39 | 平台创新 |

## 制作源文件

报告正文：`project-report-draft.md`。旁白：`narration.json` 与 `narration-draft.md`。封面使用内置 image_gen，提示词见 `cover-prompt.md`。

浏览器操作录制位于 `.build/msup-demo/recording/take3/`，剪辑时间轴位于 `recording/edit-v2/timeline.json`。录制采用 1280×720 的页面布局，以 1.5 倍呈现到 1920×1080 成片；裁掉 CDP 录制帧未使用的画布后合成。产品页面点击、滚动、表单切换和展开来自本次浏览器操作。原案例数据沿用已有运行。

重建脚本位于 `infra/demo/materials/`：synthesize.py 合成旁白，build_video.py 合成视频，build_report.py 导出报告。修改旁白后同步两个源文件，并清理对应 `recording/timed/NN.mp3` 与 `NN.json` 缓存。TTS 读取本地 `.env.tts.local`，使用 Step Plan 端点。

新版文件已覆盖当前交付路径，上一版保留在 `.build/msup-demo/deliverables-v1/`。

## 检查

报告已逐页渲染检查 10 页；视频各章节抽帧检查通过，音视频均为 219.04 秒，完整解码无错误。旁白与字幕已整体换为产品介绍文案。
