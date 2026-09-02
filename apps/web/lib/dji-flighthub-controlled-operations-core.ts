export type ControlledOperationRisk = "high" | "critical";

export type ControlledOperationDefinition = {
  capabilityCode: string;
  label: string;
  domain: "flight" | "device" | "live" | "geospatial" | "model" | "management";
  risk: ControlledOperationRisk;
  featureFlag: string;
  permission: "mission:operate" | "project:admin" | "organization:manage";
  prerequisites: readonly string[];
  approval: string;
  resultEvidence: string;
  href: string;
};

export const FLIGHTHUB_CONTROLLED_OPERATIONS: readonly ControlledOperationDefinition[] = Object.freeze([
  { capabilityCode: "flight.execute", label: "飞行任务写入", domain: "flight", risk: "critical", featureFlag: "flighthub.actions",
    permission: "mission:operate", prerequisites: ["安全预检", "在线设备", "航线版本"], approval: "任务级审批", resultEvidence: "任务状态回读", href: "flight-operations" },
  { capabilityCode: "device.control", label: "返航、暂停与恢复", domain: "device", risk: "critical", featureFlag: "device.control",
    permission: "mission:operate", prerequisites: ["设备在线", "状态新鲜", "安全策略"], approval: "设备与 action 精确审批", resultEvidence: "指令状态或物模型", href: "devices" },
  { capabilityCode: "device.camera.change", label: "相机模式切换", domain: "device", risk: "high", featureFlag: "flighthub.camera.change",
    permission: "mission:operate", prerequisites: ["支持型号", "设备在线", "状态新鲜"], approval: "设备与 action 精确审批", resultEvidence: "指令状态回读", href: "devices" },
  { capabilityCode: "device.lens.change", label: "镜头切换", domain: "device", risk: "high", featureFlag: "flighthub.lens.change",
    permission: "mission:operate", prerequisites: ["支持型号", "设备在线", "状态新鲜"], approval: "设备与 action 精确审批", resultEvidence: "指令状态回读", href: "devices" },
  { capabilityCode: "device.rtk.calibrate", label: "RTK 标定", domain: "device", risk: "critical", featureFlag: "flighthub.rtk.calibrate",
    permission: "project:admin", prerequisites: ["现场验收", "设备在线", "二次确认"], approval: "设备与 action 精确审批", resultEvidence: "受理后人工/状态对账", href: "devices" },
  { capabilityCode: "device.relay.pair", label: "中继对频", domain: "device", risk: "critical", featureFlag: "flighthub.relay.pair",
    permission: "project:admin", prerequisites: ["现场验收", "设备在线", "二次确认"], approval: "设备与 action 精确审批", resultEvidence: "受理后人工/状态对账", href: "devices" },
  { capabilityCode: "device.active-project.update", label: "设备迁移", domain: "device", risk: "critical", featureFlag: "flighthub.device-migration",
    permission: "project:admin", prerequisites: ["目标项目核验", "现场验收", "二次确认"], approval: "设备与 action 精确审批", resultEvidence: "远端归属回读", href: "devices" },
  { capabilityCode: "security.sn.decrypt", label: "SN 解密", domain: "device", risk: "high", featureFlag: "flighthub.sn-decrypt",
    permission: "project:admin", prerequisites: ["组织范围", "加密输入", "二次确认"], approval: "连接器精确审批", resultEvidence: "加密结果信封", href: "connectors" },
  { capabilityCode: "live.quality.set", label: "直播画质", domain: "live", risk: "high", featureFlag: "flighthub.live.quality",
    permission: "mission:operate", prerequisites: ["设备通道", "直播可用"], approval: "按策略审批", resultEvidence: "Worker 最终状态", href: "realtime" },
  { capabilityCode: "live.recording.control", label: "录制控制", domain: "live", risk: "high", featureFlag: "flighthub.live.recording",
    permission: "mission:operate", prerequisites: ["录制能力", "项目范围"], approval: "按策略审批", resultEvidence: "录制目录回读", href: "realtime" },
  { capabilityCode: "live.share.manage", label: "直播分享", domain: "live", risk: "high", featureFlag: "flighthub.live.share",
    permission: "mission:operate", prerequisites: ["短期凭据", "项目范围"], approval: "按策略审批", resultEvidence: "分享目录回读", href: "realtime" },
  { capabilityCode: "live.converter.create", label: "码流转换器创建", domain: "live", risk: "high", featureFlag: "flighthub.live.converter.create",
    permission: "mission:operate", prerequisites: ["设备通道", "供应商配置"], approval: "按策略审批", resultEvidence: "转换器目录回读", href: "realtime" },
  { capabilityCode: "live.converter.toggle", label: "码流转换器开关", domain: "live", risk: "high", featureFlag: "flighthub.live.converter.toggle",
    permission: "mission:operate", prerequisites: ["目标版本", "项目范围"], approval: "按策略审批", resultEvidence: "转换器目录回读", href: "realtime" },
  { capabilityCode: "live.converter.delete", label: "码流转换器删除", domain: "live", risk: "critical", featureFlag: "flighthub.live.converter.delete",
    permission: "project:admin", prerequisites: ["精确目标", "二次确认"], approval: "目标精确审批", resultEvidence: "资源缺失回读", href: "realtime" },
  { capabilityCode: "geospatial.write", label: "地图标注写入", domain: "geospatial", risk: "high", featureFlag: "flighthub.actions",
    permission: "mission:operate", prerequisites: ["GeoJSON 校验", "远端版本"], approval: "按策略审批", resultEvidence: "标注版本回读", href: "geospatial" },
  { capabilityCode: "geospatial.element.delete", label: "地图标注删除", domain: "geospatial", risk: "critical", featureFlag: "flighthub.geospatial.delete",
    permission: "project:admin", prerequisites: ["精确目标", "远端版本", "二次确认"], approval: "目标精确审批", resultEvidence: "资源缺失回读", href: "geospatial" },
  { capabilityCode: "model.write", label: "模型重建", domain: "model", risk: "high", featureFlag: "flighthub.actions",
    permission: "mission:operate", prerequisites: ["输入资产", "配额", "幂等键"], approval: "按策略审批", resultEvidence: "模型进度与产物", href: "models" },
  { capabilityCode: "model.delete", label: "模型删除", domain: "model", risk: "critical", featureFlag: "flighthub.model.delete",
    permission: "project:admin", prerequisites: ["精确目标", "差异预览", "二次确认"], approval: "预览摘要精确审批", resultEvidence: "资源缺失回读", href: "models" },
  { capabilityCode: "model.resource.delete", label: "模型资源删除", domain: "model", risk: "critical", featureFlag: "flighthub.model-resource.delete",
    permission: "project:admin", prerequisites: ["精确目标", "差异预览", "二次确认"], approval: "预览摘要精确审批", resultEvidence: "资源缺失回读", href: "models" },
  { capabilityCode: "organization.project-member.write", label: "添加或更新项目成员", domain: "management", risk: "high",
    featureFlag: "flighthub.organization.project-member", permission: "organization:manage",
    prerequisites: ["组织成员目标", "精确预览", "二次确认"], approval: "预览摘要精确审批", resultEvidence: "项目用户与成员双回读", href: "connectors" },
]);

export type ControlledOperationJob = { id: string; action: string; status: string; lastErrorCode: string | null;
  completedAt: string | Date | null; updatedAt: string | Date };

export function buildFlightHubControlledOperations(input: { projectId: number; connectorStatus: string; role: string;
  permissions: ReadonlySet<string>; managementGranted: boolean; manifestCapabilities: ReadonlySet<string>;
  featureFlags: Readonly<Record<string, boolean>>; fieldWriteCapabilities: ReadonlySet<string>; jobs: ControlledOperationJob[] }) {
  const connectorReady = input.connectorStatus === "connected";
  return {
    actions: FLIGHTHUB_CONTROLLED_OPERATIONS.filter((definition) => input.manifestCapabilities.has(definition.capabilityCode)).map((definition) => {
      const permissionReady = definition.permission === "organization:manage" ? input.managementGranted
        : definition.permission === "project:admin" ? new Set(["owner", "admin"]).has(input.role)
        : input.permissions.has("mission:operate");
      const featureEnabled = input.featureFlags[definition.featureFlag] === true;
      const capabilityVerified = input.fieldWriteCapabilities.has(definition.capabilityCode);
      const missing = [!connectorReady && "连接器未连接", !permissionReady && `缺少 ${definition.permission}`,
        !featureEnabled && `功能开关 ${definition.featureFlag} 未开启`, !capabilityVerified && "缺少 field-write 现场验收"].filter(Boolean) as string[];
      return { ...definition, href: `/projects/${input.projectId}/${definition.href}`, connectorReady, permissionReady,
        featureEnabled, capabilityVerified, available: missing.length === 0, missing };
    }),
    jobs: input.jobs.slice(0, 50),
  };
}
