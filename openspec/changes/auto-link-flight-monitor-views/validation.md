# 验证记录

## 2026-10-12 本地验证

- 线上只读发现目录确认设备 1 的 parentExternalId 精确对应设备 2；快照 deviceRelations 却为空。没有按名称或坐标猜测配对。
- 修复前工作台未调用已有 relatedLiveDevices，且 autoStart 仅匹配初始机场，造成只显示机场直播。
- `node --test apps/web/lib/monitor-layout.test.ts apps/web/lib/realtime-workbench-core.test.ts apps/web/lib/device-video-channel.test.ts`：14 项通过。
- `pnpm typecheck`、`pnpm build:web`：通过。
- `go test -tags dev ./internal/connector -count=1`：通过，设置了本机 127.0.0.1:55439 隔离 PostGIS 的 AEROSIGHT_TEST_DATABASE_URL，包含真实 SQL 集成测试，不使用生产数据库测试。
- 新 SQL 测试覆盖 review 已接入身份的关联补齐、游标重放去重、ignored 身份不参与、旧父关系重放不覆盖、跨连接器父身份不参与。
- Playwright 使用真实快照结构构造本地延迟上线场景，拦截全部写请求。离线飞机不发起请求，上线后机场和主相机各请求一次；三视角分别为飞机主相机、机场、地图。手动交换及刷新保持布局，飞行下发请求数为 0。
- `pnpm exec openspec validate auto-link-flight-monitor-views --strict` 与主规范严格验证均通过。
- 未归档：尚未运行仓库完整 check/build，不能以本次针对性验证代替归档验收。

## 实飞素材

用户确认后仅下发一次测试航线，GNSS，返航高度 50 米。运行 5 返回 success。完整机场视角录屏 297.68 秒已保存在 26dgx/demo/edit/continuous-20261012/flight-takeoff/capture/raw/flight-complete.webm，另有完整 MP4。该次未录到飞行中的机载视角，不能声称是双图传录屏。本次代码验证没有再次起飞。

## 线上验证

- 修复提交 5d9b10ab3e2b5306309b0eeb1137407b7345b941 已推送 main；Zeabur 部署 6acc759eda7cf920c263ad84 于 2026-10-12 05:56:25 UTC 返回 RUNNING。
- 部署后常规同步自动恢复 contains：机场 2 → 飞行器 1。核验脚本读取到关系已存在，未触发额外扫描或数据库写入。
- 2026-10-12 05:58:51 UTC 线上 Playwright 在实际 150% 缩放下确认三个视角：M3TD（云台主相机）为主视角、协盈智慧巡查（Dock 2 Camera）和项目地图为分视角。浏览器拦截全部非 GET API，实际没有发生写请求。
- 验收时飞机已离线，页面按真实状态显示等待设备连接；双图传的空中联动尚未再次实飞录制。本轮没有再次起飞。
- 本地证据：.build/flight-monitor/production.json 与 production.png。
