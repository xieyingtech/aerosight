# 实时作业工作台本地验收

## 启动

按照 `infra/demo/README.md` 配置本地演示环境后，在仓库根目录运行：

```sh
./infra/demo/run-local.sh
```

打开脚本输出的 `/projects/{projectId}/realtime` 地址，使用本地演示账号登录。

## 验收步骤

1. 打开项目总览，确认页面只有摘要和只读态势地图，不出现设备控制、直播播放器或完整时间线。
2. 打开实时作业，确认设备搜索中同时包含 Dock 2、Dock 3、Matrice 3TD、Matrice 4TD、相机和传感器。
3. 选择无坐标设备，确认 URL 带有 `deviceId`，页面提示暂无位置，但仍显示驱动能力和实时面板。
4. 选择相机的视频频道并启动直播，确认 URL 原地增加 `streamId`，频道依次显示 `requested`/`starting`/`live`，媒体就绪前不挂载播放器。
5. 直播进入 `live` 后，确认活跃直播切换器显示设备与 `streamKey`，播放器使用签名 WebRTC 或 HLS 地址。
6. 停止直播，确认状态先进入 `stopping`，服务端确认后频道从活跃列表移除且 URL 保留设备选择。
7. 打开设备管理，确认只展示资产、DeviceType、Driver、能力、通道和诊断信息，并通过“进入实时作业”深链接操作设备。

## 本次实测基线

- Dock 2 + Matrice 3TD 模拟拓扑
- Dock 3 + Matrice 4TD 模拟拓扑
- DJI MQTT 命令与状态回执
- MediaMTX RTMP 推流和签名 WebRTC 播放
- 视频频道启动、活跃频道切换、停止与状态收敛
- 无坐标相机和传感器的搜索选择
