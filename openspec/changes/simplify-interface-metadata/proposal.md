## Why

中点串联状态、数量、来源、提示和调试字段使界面层次不清。用户已授权按盘点建议整理，保留 Enter 快捷键提示。

## What Changes

- 数量使用括号，状态使用独立标签，元信息分列或换行，提示使用完整句子。
- 核对后端命名用途，整理安全的显示名称；保留参与外部对账的持久化名称与历史契约。
- 同步当前测试文案，保留“Enter 发送 · Shift + Enter 换行”。

## Capabilities

### New Capabilities
- `interface-metadata-presentation`: 非版本元信息与提示展示。

### Modified Capabilities
- `version-label-presentation`: 非版本信息按已授权的元信息规范整理，不套用 @v。

## Impact

前端元信息、提示、状态与诊断布局，服务端纯展示名称和当前文案测试。无数据迁移、权限变化或外部服务新依赖；已有版本格式保持。
