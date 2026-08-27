# MVP 基准负载结果

## 验收结论

2026-08-27 在本地隔离的 `postgis/postgis:17-3.5` 数据库上执行 `pnpm benchmark:load`，结果通过：

| 链路 | 样本 | P50 | P95 | 最大值 | 门槛 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 遥测持久化 → 项目快照 → 地图/时间线 | 30 个 tick | 26.10 ms | 31.95 ms | 118.79 ms | ≤ 2,000 ms |
| canonical detection → callback 防重 → 告警可见 | 30 条 | 6.64 ms | 13.88 ms | 15.70 ms | ≤ 5,000 ms |

重复投递未产生重复副作用：360 条唯一遥测各重复投递 1 次后，`device_telemetry`、`observations`、`poses` 均为 360 行；30 条唯一 detection callback 各重复投递 1 次后，callback receipt、detection、detection group、perception event 均为 30 行。

## 固定参数

- 设备数：12
- 遥测 tick：30，每个 tick 每台设备发送 1 条 pose
- 遥测重复投递：每条唯一事件额外 1 次
- canonical detection：30 条
- detection callback 重复投递：每条唯一 callback 额外 1 次
- Node.js：v26.7.0，darwin/arm64，10 个逻辑 CPU
- 数据库镜像：`postgis/postgis:17-3.5`

脚本每次创建全新临时 PostGIS，应用全部迁移后再运行负载；计时范围包含持久化、数据库幂等约束、repeatable-read 项目快照查询，以及地图和时间线投影。外部网络、真实厂商链路和外部算法自身执行时间不在计时范围内。

## 复跑与调参

运行默认验收：

```sh
pnpm benchmark:load
```

可通过以下环境变量调整负载；所有值必须为正整数：

- `BENCHMARK_DEVICES`
- `BENCHMARK_TELEMETRY_TICKS`
- `BENCHMARK_TELEMETRY_DUPLICATES`
- `BENCHMARK_DETECTIONS`
- `BENCHMARK_DETECTION_DUPLICATES`
- `BENCHMARK_POSTGIS_IMAGE`（覆盖数据库镜像）

脚本会输出含环境、参数、阈值、P50/P95/最大值及副作用计数的 JSON；任何 P95 超标、视图不可见或重复副作用都会以非零状态退出。
