## 验证记录

### 个人中心弹窗修订

按用户反馈，显示名称、项目名称及密码均改成操作按钮打开 Dialog；个人中心移除账号与团队表格，只展示当前会话账号的一份摘要，且不再调用 /api/profile 获取团队列表。约定已写入 AGENTS.md。

修订后的 `pnpm typecheck`、`pnpm build:web`、`pnpm build:server`、change 与 account-project-settings 主规范严格校验均通过。`node scripts/test-production-browser.mjs --settings-only` 再次通过，新增验证默认页面没有表格或编辑输入框、弹窗取消、关闭后清空密码、重新打开、名称保存和刷新；既有密码修改、旧会话撤销和重新登录链路继续通过。证据为 `.build/production-browser-04797da2-c793-411e-a29e-fd700f3cd8ee/`。取消后等待关闭动画完成再断言，避免动画期间的误判。

已重启独立本地预览并刷新用户的个人中心，当前展示密码修改弹窗。全量检查的既有报告失败仍按下文保留，未重跑无关服务端测试，未归档。

日期：2026-10-13。本工作区实现并验证，未部署或对生产数据库执行迁移。

- `pnpm typecheck`：通过。
- `pnpm build`：通过；42 个静态页面导出，sqlc v1.31.1 生成一致，Go 嵌入构建成功。补充语音会话复查后再次 `pnpm build:server` 通过。
- 独立临时 PostGIS 数据库的相关 Go API 测试：`TestAccountSettings`、`TestProjectRenameAuthorization`、`TestSettingsNameUnicode`、`TestSettingsCSRF`、既有登录/重启/轮换测试通过。覆盖 Unicode 输入、未知字段拒绝、团队管理角色、跨团队限制、成功审计、错误原密码、72 字节上限、旧密码拒绝和多客户端旧会话失效。
- `TestRealtimeCredentialVersionRevocation`：真实 WebSocket 加本地模拟上游，通过；凭据版本改变后返回 UNAUTHENTICATED 并断开连接。既有 Realtime 测试在同批数据库测试中通过。
- `node scripts/test-production-browser.mjs --settings-only`：通过。真实生产嵌入服务、HTTPS 和临时 PostGIS 环境，验证项目名称保存及侧栏刷新、显示名称保存及侧栏刷新、Enter 提交、刷新持久化、确认密码不一致、错误原密码、改密跳转、两端会话撤销、旧密码失败、新密码登录，以及退出、会话过期与 CSP。
- 浏览器证据：`.build/production-browser-84786bc3-b7cd-4bb3-9118-4c695d198637/` 下的 `account-settings.json`、`result.json`、`profile-settings.png`、`project-settings.png`。截图已目视检查。
- OpenSpec change 严格校验及 16 个主规范严格校验通过。account-project-settings 新主规范与 go-auth-session 增量已同步。

## 全量检查的既有失败

`pnpm check` 前端边界、类型检查、400 项前端单测及历史契约检查通过；Go 全量测试失败于 `TestReportAggregateIncludesAllEvidence`，`report_drafts_test.go:110` 对 task_run 引用版本的期望与现有 `state:3` 不一致。该测试及实现文件与 HEAD 无差异，未在本次设置功能变更中修改。部分需要显式环境变量的旧数据库测试按既有机制跳过；本次相关数据库测试另行真实运行，不以跳过计为通过。

最终全量日志：`.build/account-settings-check-final.log`。保留集成验证任务未勾选和活动 change，待既有全量失败解决后完成归档。

## 验证过程中的修正

- schema 字段顺序调整后重新生成 sqlc，解决一次生成漂移。
- 测试正确处理 204 空响应；账号测试使用 10 秒请求超时以覆盖 bcrypt 操作，产品默认超时未修改。
- 设置专项浏览器模式接受当前 `/projects/:id/` 项目地址；失败登录后重新填写被表单重置的账号，再验证新密码。
