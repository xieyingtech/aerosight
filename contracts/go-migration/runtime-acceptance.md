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

任务 7.3 的启动、必要故障、关闭和租约恢复契约已验证。任务 8.2 的完整业务端到端、8.3 的同时负载/代理对照，以及 7.5 的最终镜像仍是独立未完成门槛；本文不宣称外部设备命令具有无条件 exactly-once，也不以数据库事务幂等推导外部副作用永不重复。
