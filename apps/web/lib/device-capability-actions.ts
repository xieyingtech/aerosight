export type DeviceActionField = {
  key: string;
  label: string;
  type: "text" | "number";
  required: boolean;
  unit?: string;
};

export type DeviceCapabilityAction = {
  capabilityCode: string;
  key: string;
  label: string;
  kind: "command" | "live" | "workflow";
  risk: "low" | "medium" | "high" | "critical";
  fixedParameters: Record<string, unknown>;
  fields: DeviceActionField[];
};

const catalog: Record<string, Omit<DeviceCapabilityAction, "capabilityCode" | "risk">[]> = {
  "mission.execute": [
    { key: "mission.create", label: "创建航线任务", kind: "workflow", fixedParameters: {}, fields: [] }
  ],
  "flight.return_home": [
    { key: "return_home", label: "返航", kind: "command", fixedParameters: {}, fields: [] }
  ],
  "dock.debug.control": [
    { key: "cover.open", label: "打开舱盖", kind: "command", fixedParameters: {}, fields: [] },
    { key: "cover.close", label: "关闭舱盖", kind: "command", fixedParameters: {}, fields: [] },
    { key: "aircraft.power_on", label: "飞行器上电", kind: "command", fixedParameters: {}, fields: [] },
    { key: "aircraft.power_off", label: "飞行器下电", kind: "command", fixedParameters: {}, fields: [] },
    { key: "charge.start", label: "开始充电", kind: "command", fixedParameters: {}, fields: [] },
    { key: "charge.stop", label: "停止充电", kind: "command", fixedParameters: {}, fields: [] },
    { key: "debug.open", label: "开启调试模式", kind: "command", fixedParameters: {}, fields: [] },
    { key: "debug.close", label: "关闭调试模式", kind: "command", fixedParameters: {}, fields: [] },
    { key: "alarm.enable", label: "开启声光报警", kind: "command", fixedParameters: { action: 1 }, fields: [] },
    { key: "alarm.disable", label: "关闭声光报警", kind: "command", fixedParameters: { action: 0 }, fields: [] },
    { key: "reboot", label: "重启机场", kind: "command", fixedParameters: {}, fields: [] }
  ],
  "stream.video.control": [
    { key: "start", label: "启动直播", kind: "live", fixedParameters: {}, fields: [] }
  ],
  "sensor.configure": [
    { key: "configure", label: "配置传感器", kind: "command", fixedParameters: {}, fields: [
      { key: "sample_interval_seconds", label: "采样间隔", type: "number", required: true, unit: "s" },
      { key: "report_threshold", label: "上报阈值", type: "number", required: false }
    ] }
  ]
};

export function actionsForCapability(capabilityCode: string, risk: DeviceCapabilityAction["risk"]) {
  return (catalog[capabilityCode] ?? []).map((action) => ({ ...action, capabilityCode, risk }));
}
