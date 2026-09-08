# 统一生命周期验收

对应任务 7.3。核对日期：2026-09-08。入口为 `apps/server/cmd/aerosight/main.go`，后台组装与监督为 `apps/server/internal/runtime/runtime.go`。以下分别说明组件契约和整进程证据，不将组件 fixture 当作全部业务同时运行的负载演练。

| 要求 | 实现与通过证据 |
| --- | --- |
| 统一启动、取消与关闭 | `serve` 初始化迁移、两类数据库池、后台运行时和 HTTP，HTTP BaseContext 使用根 context；退出撤销 ready、取消根 context、关闭会话清理、执行 HTTP Shutdown，并等待 HTTP/后台返回后关闭连接池。回滚演练实际发送 SIGTERM 后退出 0，SSE EOF，数据库客户端归零。 |
| 必要组件失败撤销 readiness | `RunWithFailure` 在等待同伴排空前报告错误或意外正常返回。真实 PostGIS 中数据库 Ping 正常、outbox 表缺失，实际消费者领取失败；`TestComponentFailureRevokesHTTPReadinessWhileDraining` 验证 ready 从 200 变 503、health 仍 200，并协调取消。 |
| MQTT 关闭后才释放租约 | 管理器取消所有会话，等待 watcher 和 MQTT Done，再在限时 context 中释放租约；关闭失败保留租约用于到期恢复。`TestManagerShutdownWaitsForMQTTBeforeReleasingLease`、`TestLostLeaseRemainsTrackedUntilSessionCloses` 及超时用例通过。另在真实 Mosquitto 运行 `TestMQTTManagerShutdownAndRestart`，验证连接、发布、关闭、释放和新管理器接管；其中租约 repository 为 fixture。 |
| outbox 未完成工作可恢复 | 处理事务持有事件行锁；取消不提前 Complete。真实数据库的 `TestPostgresRestartRecoversWithoutDuplicateTransactionEffects`、`TestPostgresActiveTransactionCannotBeReclaimed`、`TestPostgresExhaustedLeaseResolvesCommittedAndUncommittedWork` 通过，覆盖旧持有者拒绝、提交后未确认的恢复、活动事务不被重领、耗尽租约收敛及事务副作用不重复。 |
| 调度停止领取并等待续租结束 | 取消后批次不继续执行，不写虚假成功；真实数据库 `TestPostgresSchedulerCancellationAndRestartRecovery` 验证租约保留、到期后更高 epoch 接管、旧持有者写入/释放被隔离、新结果和游标成功持久化。调度取消与阻塞续租单元测试通过。 |
| 会话清理属于应用生命周期 | 禁用 postgresstore 自带定时器，应用拥有清理 context 和 Done。`TestSessionCleanupPostgresExpiryAndLockedShutdown` 在真实数据库验证过期清理及被表锁阻塞的 SQL 可取消；并发 Close、截止时间与持久会话回归通过。 |
| 正常停止及应用重启 | `test-container-lifecycle` 在非 root、无 Node/pnpm 的容器里直接运行生产二进制，登录、创建资源、打开 SSE，真实 SIGTERM 后退出 0、连接归零；启动同一容器后会话和项目保留，再次停止成功。该测试使用缓存运行 fixture，不冒充最终发布镜像。 |

## Evidence

- `.build/full-go-postgis.jsonl`：本次全 Go 真实 PostGIS 回归，以上具名数据库及生命周期测试均为 pass，已逐项核对事件。
- `.build/mqtt-lifecycle-16b80e9a-5d6f-4820-889c-6fde0567fffd/`：真实认证 Mosquitto 与管理器退出/接管。
- `.build/container-lifecycle-ef99f91b-6bad-43ad-adaa-f12986475143/result.json`：两次停止和中间重启，首轮停止 479 ms。
- `.build/upgrade-rollback-7e1a2d8e4c301333/result.json`：生产 Go 服务 SIGTERM 497 ms、连接归零后回切旧应用。

任务 7.3 的启动、必要故障、关闭和租约恢复契约已验证。后续正式镜像验收已完成任务 7.5，证据见 implementation-evidence.md。任务 8.2 的完整业务端到端和 8.3 的全部活动任务恢复仍是独立未完成门槛；本文不宣称外部设备命令具有无条件 exactly-once，也不以数据库事务幂等推导外部副作用永不重复。

## 正式镜像负载与回调重启补验

`pnpm test:release-image` 使用最终镜像内部二进制，无宿主源码或二进制挂载，保留镜像默认用户和入口。24 条 SSE 在 8 并发、共 80 次快照读取期间保持连接；超过 30 秒后全部收到新插入事件，数据库连接采样未超过默认合计预算 30。SIGTERM 后全部流结束、数据库连接归零，重启后会话和项目保留。

`.build/container-lifecycle-55780be7-884a-4de8-b8e0-4b1bae4671e6/result.json` 同时记录真实签名算法回调：预置进行中 run，在停止前处理 processing；重启后等待状态与回执保留，旧回执重放去重，completed 成功且重复提交不改变结果元数据和回执数量。此测试覆盖接收端恢复，不包含上游执行发起；对象存储使用测试 tmpfs，未将元数据去重作为结果文件跨后续重启持久化的证明。

后续 `.build/container-lifecycle-498a6675-9338-4395-ab6a-182e0df60df8/result.json` 将 run 改为真实 API 发起：API 创建定义/运行，outbox 发出 HTTPS 请求，Go 签发的资产 URL 能返回一致 PNG 字节，实际签发的 callback token 在重启前后有效。重启及完成重放后上游请求和成功 attempt 均只有一次，回执为两条。provider 和输入资产仍由 fixture 准备，运行服务未使用注入 handler 或替换 transport。结果文件跨后续重启的限制保持不变。

`.build/container-lifecycle-d0d7205e-b759-4a0c-9af0-1b9f5f073ba8/ai-flow.json` 补充活动 AI 请求：确认真实 HTTPS 上游尚未响应且连接开放后发送 SIGTERM，在 592 ms 整进程停止期间返回单一 REQUEST_CANCELLED、关闭上游连接；客户端期限 60 秒，排除客户端超时造成的假阳性。重启后该会话只有已提交的用户消息，没有虚假回复或自动重发。此过程同时验证 SCS 保存取消时不会向业务错误响应追加第二个 JSON。

`.build/container-lifecycle-31a30e35-38e0-40a9-a3f8-8334ff987511/object-persistence.json` 使用独占持久卷保存真实输入与算法结果，在完成回调之后再次停止/启动进程，结果文件 JSON 和 SHA-256 与数据库记录一致，原签名 URL 返回一致输入字节，重放不改完成状态或重发上游。由此补齐前述 tmpfs 证据的对象文件持久化限制。
