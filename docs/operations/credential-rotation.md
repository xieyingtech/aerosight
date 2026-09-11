# `APP_SECRET` 与数据库凭据轮换

`APP_SECRET` 用于短时令牌签名和外部 Provider 凭据加密。计划更换时不能直接替换部署变量，否则既有连接器、算法 Provider 和 AI Provider 密文将无法解密。

仅将变量名从 `AUTH_SECRET` 改为 `APP_SECRET` 时，保持原值，不运行重加密。`CSRF_AUTH_KEY` 同样仅改名为 `CSRF_SECRET` 并保留原值；它独立用于 CSRF Cookie 签名。部署新版应用时同步切换变量名称。

## 计划内轮换

1. 进入维护窗口，停止 Web/Worker 的写流量和外部调用；保留数据库备份以及当前、新 `APP_SECRET`。
2. 让命令环境中的 `DATABASE_URL` 和 `APP_SECRET` 指向目标数据库及当前旧值。
3. 先校验全部 envelope：`go -C apps/server run ./cmd/rotate-credentials --dry-run`。
4. 交互运行 `go -C apps/server run ./cmd/rotate-credentials`。命令在终端无回显读取并二次确认新值。
5. 自动化环境通过标准输入传入新值：`printf '%s\n' "$NEW_APP_SECRET" | go -C apps/server run ./cmd/rotate-credentials --new-secret-stdin`。不要把密钥作为命令行参数。
6. 命令取得 advisory lock，在单个 serializable 事务中锁定、解密、重新加密并校验全部凭据；任意损坏、AAD 不匹配、并发变化或验证失败都会回滚。
7. 成功后原子更新 Web/Worker 的部署 `APP_SECRET`，重启二者，验证登录、DJI MQTT/直播、算法调用和默认 AI Provider。
8. 验证完成前保留旧值；确认新版本稳定后再退出维护窗口并销毁旧值。

成功输出只包含每类资源数量、新 key fingerprint 和后续指引。输出不得包含旧值、新值或业务凭据。

## 失败与恢复

- dry-run 或正式轮换失败时不要更新部署变量；修复损坏 envelope/AAD 或并发操作后继续使用旧值重试。
- 正式命令在提交前失败不会产生新旧密钥混用，可继续用旧值启动服务。
- 如果命令已成功但应用切换失败，优先修复部署并使用新值；不要再用旧值启动会读取外部凭据的进程。
- 如果旧 `APP_SECRET` 已遗失，旧密文不可恢复，也无法执行轮换，只能在管理界面逐项重新填写连接器、算法和 AI Provider 凭据。
