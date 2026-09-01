# DJI 司空 2 连接器真实验收记录

本记录用于 OpenSpec `add-dji-flighthub-connector` 的 6.2–6.3。连接生命周期与实际可访问产品必须使用具备 OpenAPI 权限的司空测试组织完成真实请求；团队当前不具备的产品不得冒充实机验收，只能以版本化 fixture 验证映射与失败关闭边界，并明确保留为非现场验收。

## 1. 验收基线

- AeroSight 构建/提交：`d064286`
- 验收日期与时区：`2026-09-01 Asia/Shanghai`
- AeroSight 项目编号：`1（隔离本地项目）`
- 司空区域主机：`https://es-flight-api-cn.djigate.com`
- 司空项目 UUID 脱敏指纹：`md5:199ad63944c3`
- Token 指纹或轮换编号（不得记录 Token）：`轮换 #1；不记录 Token 内容或可逆指纹`
- Worker 实例名：`aerosight-worker`
- 验收人员：`用户 + Codex（自动化证据采集）`

验收材料不得包含 Token、credential envelope、完整组织名称、完整设备 SN、Cookie、请求 Authorization 信息或上游原始错误正文。设备标识只保留稳定脱敏指纹或末四位；截图需先检查浏览器地址栏、开发者工具和通知内容。

## 2. 连接生命周期

| 步骤 | 预期结果 | 脱敏证据 | 结果 |
| --- | --- | --- | --- |
| 项目发现 | 仅发送 `X-User-Token`，返回测试 Token 可访问项目；无项目、单项目和多项目交互正确 | 审计 `#3`；真实 Token 返回 1 个项目并自动选中；无项目/多项目由契约与组件测试覆盖 | ☑ |
| 选择并连接 | 服务端重新验证所选项目，创建一个通用 Connector Instance | connector `#1`；审计 `#4`；envelope 仅有算法、密文、nonce、认证标签、key 指纹与版本字段 | ☑ |
| 首次同步 | Web 立即返回，Worker 从 outbox 领取并完成目录同步 | outbox `#1`、sync run `#1`；发现 2、纳管 0、缺失 0；身份为 Dock 2 + M3TD | ☑ |
| 重复同步 | 重放同一请求不重复创建设备身份或同步副作用 | sync run `#2/#3` 均成功且新增计数为 0；前后外部身份总数/唯一数均为 2 | ☑ |
| 手动同步 | “立即同步”进入队列；重复点击合并为现有请求 | outbox `#2/#3` 各执行一次；请求期间按钮禁用，未形成并发重复执行 | ☑ |
| Token 撤销 | 真实 401/403 使连接器降级/失败，停止快速重试且保留上次快照 | 轮换旧 Token 后 sync run `#6/#7` 返回 `CREDENTIAL_INVALID`；连接器为 failed，快照游标不变、身份仍为 2；修复终态 outbox 完成语义后 event `#6` completed，等待后 run 数不变且无 pending/processing | ☑ |
| Token 更新 | 无效新 Token 保留旧 envelope；有效新 Token 原子替换并触发同步 | 无效 Token 返回 `credential_invalid`，envelope hash 前后相同，失败审计 `#8` 不含 Token；有效 Token 审计 `#11` 显示项目重新验证，envelope 已变化，outbox `#7` 一次完成，sync run `#8` 成功并恢复 connected | ☑ |
| Worker 重启 | 运行中停止 Worker，租约过期后新 Worker 恢复且不重复身份 | Worker 停止后 outbox `#4` 保持 pending/attempts=0；新 Worker 启动后一次完成为 sync run `#4`；身份仍为 2 | ☑ |
| 断开 | 禁止新同步、释放租约，设备/身份/审计保留并按新鲜度降级 | 审计 `#12`；connector disabled、租约与到期时间清空、envelope 保留；身份 2、sync run 8、活动 outbox 0；UI 禁用同步/断开并显示 `CONNECTOR_DISCONNECTED` | ☑ |

## 3. Dock 2/3 产品与拓扑

分别记录真实目录返回并由 AeroSight 映射后的结果。测试组织缺少 Dock 3/M4D 系列，因此该产品只记录版本化 fixture 的类型与拓扑回归结果，不声明真实设备、在线状态或控制能力已验收。

| 产品 | 脱敏外部标识 | DeviceType / Driver | 父子关系 | 状态能力 | 控制能力为空 | 结果 |
| --- | --- | --- | --- | --- | --- | --- |
| Dock 2 | `md5:4f1d6018cbe3` | `dji.dock2@1 / dji.cloud@1.0.0` | gateway | 目录/状态 | ☑ | ☑ |
| Matrice 3D 或 3TD | `md5:ae28dbf42835` | `dji.matrice3td@1 / dji.cloud@1.0.0` | parent=`md5:4f1d6018cbe3` | 目录/状态 | ☑ | ☑ |
| Dock 3（fixture） | `DOCK-003（测试标识）` | `dji.dock3@1 / dji.cloud@1.0.0` | gateway | 仅契约映射 | ☑ | ☑（非实机） |
| Matrice 4TD（fixture） | `AIRCRAFT-004（测试标识）` | `dji.matrice4td@1 / dji.cloud@1.0.0` | parent=Dock 3 | 仅契约映射 | ☑ | ☑（非实机） |

额外负向验证：

- [x] 未知型号由 `TestMapDirectoryFailsClosedForDuplicateAndUnknownProducts` 保持 `dji.unknown`、`knownProduct=false` 和 review reason，不推断控制能力。
- [x] 同一 SN 多来源由 `TestSQLSyncStoreMarksCrossSourceSerialConflictWithoutChangingBindings` 标记 `conflicted`，不自动创建 binding 或改变下行路由。
- [x] 司空 runtime fixture 对所有已知产品只投影 `state.read` 且 `readOnly=true`；UI 边界测试确认不渲染任务下发、返航、机场调试、直播控制。
- [x] 当前页面不开放 DJI Cloud API 或其他现场设备接入类型；相关实机控制不属于本只读连接器交付范围，不能因保留后端代码而声明已验收。

## 4. 完成结论

- OpenSpec 6.2：☑ 通过 / ☐ 未通过
- OpenSpec 6.3：☑ 通过（真实 Dock 2/M3TD + Dock 3/M4TD fixture 边界；不含 Dock 3/M4TD 实机能力声明） / ☐ 未通过
- 回归检查：`pnpm check`、`pnpm build` 与 `openspec validate add-dji-flighthub-connector --strict` 均通过；连接器列表与新建类型选择另经本地浏览器验收
- 未通过项与后续动作：`无当前交付阻塞。获得 Dock 3/M4D/4TD 或其他现场设备后，应另行发起接入/控制变更并补做真实硬件验收。`
- 脱敏证据存放位置：`本记录；数据库记录仅保留 connector #1、outbox #1–#7、sync run #1–#8 与 audit #3–#12。`
- 验收人与复核人签字：`________________________________________________`

当前仅开放司空 2 只读目录连接器。其他接入方式保持不可创建；获得对应设备并完成独立验收前，不得把 fixture 结果解释为现场控制能力。
