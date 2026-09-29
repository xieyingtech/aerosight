package httpapi

import "github.com/gin-gonic/gin"

// This is an API applicability catalog, never an execution grant.
func flightHubDeviceCatalog(device gin.H) gin.H {
	if device["connectorKey"] != "dji.flighthub2" {
		return nil
	}
	role := ""
	switch device["typeKey"] {
	case "dji.dock2":
		role = "机场本体"
	case "dji.matrice3d", "dji.matrice3td":
		role = "配套飞行器"
	case "dji.dock2.camera", "dji.matrice3d.camera", "dji.matrice3td.camera", "dji.matrice3.vision-assist":
		role = "相机负载"
	case "dji.dock2.environment-sensor":
		role = "环境传感器"
	default:
		return gin.H{"family": "其他型号", "role": "待核对", "notice": "当前设备能力目录仅核对 Dock 2 与 Matrice 3D/3TD；其他型号需另行验证。", "groups": []gin.H{}}
	}
	entry := func(label, status, note string, ids ...string) gin.H {
		return gin.H{"label": label, "implementation": status, "note": note, "endpointIds": ids}
	}
	reads := []gin.H{
		entry("设备目录、详情与拓扑", "已接入", "根据项目绑定关系区分机场和飞行器；历史拓扑不用于判断在线状态。", "456680821e0", "456680822e0", "457014318e0", "457048706e0"),
		entry("物模型与 HMS 健康状态", "已接入", "只能展示上游实际返回的状态；空结果不代表设备健康。", "458069501e0", "458069499e0"),
	}
	groups := []gin.H{{"label": "只读查询", "items": reads}}
	if role == "机场本体" || role == "配套飞行器" || role == "相机负载" {
		groups = append(groups, gin.H{"label": "直播与相机", "items": []gin.H{
			entry("开启直播、读取直播状态", "已接入", "观看时启动推流，需账号权限、真实频道和媒体供应商支持。", "456809558e0", "457494963e0"),
			entry("调整画质、云端录制与转发", "已接入适配", "需功能开关与对应能力验证；直播成功不能证明录制或转发可用。", "458069500e0", "458069503e0", "480437503e0"),
		}})
	}
	if role == "机场本体" {
		groups = append(groups, gin.H{"label": "机场控制与飞行", "items": []gin.H{
			entry("切换舱内／舱外相机", "已接入适配", "相机索引和位置以设备目录为准；需能力验证及授权。", "456471285e0"),
			entry("返航、取消返航、暂停／恢复任务", "已接入适配", "四种指令通过机场发送；按设备状态与账号权限操作。", "454273417e0", "454273416e0"),
			entry("飞行下发条件检查、创建航线任务", "已接入适配", "在飞行任务工作台执行；完整平台发起飞行尚待实测，创建成功不代表起飞完成。", "456425797e0", "454273432e0"),
			entry("飞行状态、轨迹与照片／视频", "已接入", "回读任务结果及媒体目录；是否有可读文件取决于真实飞行及上传情况。", "454273438e0", "454273436e0", "457309246e0"),
			entry("自定义网络 RTK 标定、项目绑定更新", "已接入适配", "维护操作需独立授权和现场验证，不能作为默认开关批量执行。", "456680818e0", "456681372e0"),
		}})
	}
	if role == "配套飞行器" {
		groups = append(groups, gin.H{"label": "飞行器负载", "items": []gin.H{
			entry("获取／释放控制权、切换镜头", "已接入适配", "先获取负载控制权；可选镜头以当前相机目录为准，3D 不提供红外镜头。", "458069497e0", "458069498e0", "456470750e0"),
			entry("飞行指令", "通过机场执行", "通用控制接口接收机场 SN，不能把飞行器 SN 当作机场 SN 发送。", "454273417e0"),
		}})
	}
	return gin.H{"family": "Dock 2", "role": role, "notice": "目录表示接口适用范围与代码接入进度，不代表当前账号、固件和设备已通过操作验证。公有云 V2 目录未提供舱盖、上电、充电、调试模式、声光报警或机场重启指令。中继对频不属于本次 Dock 2 能力范围。", "groups": groups}
}
