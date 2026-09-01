## 1. 官方契约与实现基线

- [x] 1.1 针对试点司空区域确认官方 OpenAPI 基地址、组织 Token 获取位置、`GET /openapi/v2.0/project` 请求头/分页规则和设备目录接口，并保存不含真实 Token、组织名或完整 SN 的契约说明
- [x] 1.2 采集项目列表、空列表、设备目录、1000 条上限、401、403、404、429、5xx 和畸形响应的脱敏 fixture，建立 TypeScript/Go 共用的字段与错误码契约测试
- [x] 1.3 盘点现有 Connector Definition、Adapter、同步运行、凭据 envelope、outbox 和 DeviceType/Driver 复用点，以测试确认不新建司空专用连接、设备或事件账本

## 2. 数据库与 Connector Definition

- [x] 2.1 新增迁移注册 `dji.flighthub2@1.0.0`，声明 HTTPS、poll、凭据 schema、发现 scope 和兼容的只读 DJI Driver，并验证旧 `dji` Adapter 不受影响
- [x] 2.2 为通用 Connector Instance 增加 `external_scope_key` 或等效唯一约束，验证同项目不能重复连接同一司空项目、不同 AeroSight 项目仍保持隔离
- [x] 2.3 扩展迁移测试和完整 schema snapshot，覆盖升级、回滚、凭据 envelope 约束及 Connector Definition 外键

## 3. Web 临时项目发现与连接 API

- [x] 3.1 实现部署级司空配置校验和 TypeScript 项目客户端，固定官方 HTTPS 主机、超时、请求 ID、分页、有限重试和脱敏错误映射
- [x] 3.2 新增项目级项目发现 Route Handler，只允许 owner/admin，调用时只发送 `X-User-Token`，不持久化 Token，并用测试验证 member、跨项目、速率限制和日志/审计脱敏
- [x] 3.3 新增连接创建 Route Handler，重新获取项目列表并复核所选 UUID，在同一事务中创建 Connector Instance、AES-GCM 加密 Token、写审计摘要和首次同步 outbox
- [x] 3.4 实现连接状态、更新 Token、立即同步和断开 API；更新失败保留旧凭据，立即同步幂等合并，断开不删除设备或审计历史
- [x] 3.5 添加 API 安全测试，覆盖伪造项目 UUID、发现结果过期、重复创建、SSRF 输入、Token 出现在 URL/响应/日志/审计以及并发更新

## 4. Go Worker 司空 Runtime

- [x] 4.1 实现 Go FlightHub HTTP 客户端，使用版本化契约 fixture 覆盖项目作用域请求、设备目录、1000 条完整性上限、超时、401/403、429 `Retry-After`、5xx、响应大小上限和 schema 漂移
- [x] 4.2 注册 `dji.flighthub2` Connector runtime 和凭据解析器，复用 Connector registry、租约和 scope filter，并验证同一实例只有一个 Worker 执行同步
- [x] 4.3 将通用 Connector 调度器接入 Worker main，消费首次/手动同步 outbox并按抖动周期运行 poll；用重启、租约失效和并发请求测试验证最终恢复
- [x] 4.4 将完整且未触及上限的设备目录映射为 `ExternalDevice`，使用项目 UUID + SN 稳定标识并表达 Dock/飞行器父子关系；请求失败、达到 1000 条上限或 schema 不兼容时不得推进游标、部分提交或标记 missing
- [x] 4.5 将已知 Dock 2/3 和配套飞行器映射到现有 DeviceType/Driver，只投影司空 runtime 已实现的只读能力；未知型号、重复 SN 和跨来源身份进入待确认/冲突
- [x] 4.6 实现健康状态、指数退避、撤权降级和脱敏指标，覆盖同步成功/失败/耗时、429、凭据失效、积压和 schema 不兼容告警

## 5. 连接器 UI

- [x] 5.1 在项目连接器页增加“DJI 司空 2”入口，与现有“DJI Cloud API 直连”并列展示并解释只读公有云同步与直连控制的能力差异
- [x] 5.2 实现两阶段向导：Token 只保存在 client state，验证后自动显示项目下拉框，用户选择项目后完成连接；成功、失败、取消和卸载时清空 Token
- [x] 5.3 增加打开官方司空页面的快捷链接和获取 Token 指引，明确不是 OAuth，且不在链接、URL 参数、浏览器存储或页面源码中携带 Token
- [x] 5.4 展示已选项目、连接健康、最近验证/同步、同步数量、脱敏错误以及更新 Token、立即同步、断开操作；member 不得看到管理页面或秘密字段
- [x] 5.5 扩展设备候选/连接器同步日志界面，展示 discovered、managed、conflicted、missing 状态；同 SN 多来源必须要求人工处理且不得自动改变下行路由
- [ ] 5.6 添加组件与浏览器测试，覆盖无项目、单项目、多项目、Token 失效、项目被撤权、重复点击、离开页面清理和只读角色

## 6. 文档、真实验收与交付

- [x] 6.1 更新环境变量和运维文档，说明区域 API 主机、出口 HTTPS、代理、同步周期、Token 最小权限/轮换、功能开关及司空与直连 Cloud API 的选择方式
- [ ] 6.2 在司空测试组织完成项目发现、选择连接、首次同步、重复同步、手动同步、Token 撤销/更新、Worker 重启和断开验收，并保存脱敏证据
- [ ] 6.3 验证 Dock 2/3 与配套飞行器的类型、拓扑和只读能力；确认司空来源不会显示任务、返航、机场调试或直播操作
- [x] 6.4 运行迁移测试、`pnpm check`、`pnpm build`、Go 全量测试、安全验收和 OpenSpec strict validation，修复失败并记录最终结果
- [x] 6.5 按数据库、Web 握手、Worker runtime、UI 和文档边界创建 Conventional Commits，确认最终工作区只保留用户原有 `docs/` 修改
