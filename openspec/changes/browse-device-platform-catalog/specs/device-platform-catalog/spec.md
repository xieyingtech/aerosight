## Purpose

为平台管理员提供设备类型、驱动、能力、数据通道和操作键的统一查询入口，展示真实定义及关联关系，区分目录中的声明与具体设备的当前运行状态，减少通过图标配置猜测平台能力的混淆。

## ADDED Requirements

### Requirement: Read-only catalog tables

设备类型管理页 SHALL 使用表格并提供设备类型、驱动、能力、数据通道和操作 tab；支持按键、名称和关联驱动搜索。目录页 SHALL 不提供图标编辑，现有地图图标 SHALL 保留。

#### Scenario: Admin searches a key
- **WHEN** 管理员选择目录 tab 并输入关键字
- **THEN** 表格仅显示匹配记录，能够查看该记录的定义详情

### Requirement: Authoritative definitions and relationships

目录 SHALL 来源于持久化设备类型、驱动 manifest 和平台操作目录；展示版本、状态、能力与通道关联及可用 schema。相同能力键在不同驱动下 SHALL 分别展示，不覆盖差异；目录声明 SHALL 不代表具体设备当前可用、在线或授权执行。

#### Scenario: Driver declares capability
- **WHEN** 驱动存在某能力或通道定义
- **THEN** 对应 tab 展示真实键与来源驱动，详情可查关联 schema，不宣称具体设备已验收

### Requirement: Platform administrator boundary

目录 API SHALL 仅允许平台管理员读取，不返回项目设备、连接配置或凭据；浏览定义 SHALL 不触发设备命令。

#### Scenario: Non-admin reads catalog
- **WHEN** 普通登录用户请求目录
- **THEN** 返回拒绝访问且不泄露目录内容
