# go-auth-session Specification

## Purpose

定义由统一应用服务管理的账号登录、持久会话和请求权限行为，确保替换旧前端认证运行时后历史账号可继续使用，并维持浏览器写入保护、租户隔离与权限撤销的有效性。

## Requirements

### Requirement: 账号与登录迁移

系统 SHALL 保留现有邮箱或手机号登录及 bcrypt 密码兼容性，提供 csrf、login、logout、session 接口；旧前端登录态 MUST 不被当作新会话。

#### Scenario: 历史账号登录

- **GIVEN** 数据库存在原 bcrypt 账号
- **WHEN** 向 /api/auth/login 提交正确 username/password 与 CSRF Token
- **THEN** 登录成功，无需重置密码，session 接口返回当前用户

#### Scenario: 升级旧会话

- **GIVEN** 浏览器只有旧 Auth.js Cookie
- **WHEN** 请求受保护 API
- **THEN** 返回未认证，用户重新登录后正常使用

#### Scenario: 错误密码

- **GIVEN** 账号存在但提交密码不正确
- **WHEN** 尝试登录
- **THEN** 返回通用认证失败，不暴露账号是否存在

### Requirement: 持久化会话与退出

系统 SHALL 在数据库保存会话，登录时轮换 Token，退出时销毁会话；Cookie 必须 HttpOnly、SameSite=Lax，HTTPS 下必须 Secure，过期会话不得继续授权。

#### Scenario: 重启保留会话

- **GIVEN** 数据库会话仍在有效期内
- **WHEN** 重启应用后再次请求 session
- **THEN** 会话仍有效，用户无需因重启再次登录

#### Scenario: 退出失效

- **GIVEN** 用户已登录
- **WHEN** 成功调用 logout 后重放原 Cookie
- **THEN** 返回未认证；原实时连接在 15 秒复查周期内结束

#### Scenario: 超时会话

- **GIVEN** 会话超过最长 7 天或闲置 24 小时的默认配置
- **WHEN** 请求受保护 API
- **THEN** 返回未认证；配置覆盖值按实际生效期限判断

### Requirement: 浏览器写请求防伪造

系统 SHALL 对所有基于 Cookie 的写请求，包括登录和退出，验证 CSRF Token 与允许来源；独立签名的机器回调 SHALL 使用自身认证，不依赖浏览器会话。

#### Scenario: 缺少 Token

- **GIVEN** 浏览器具有有效登录 Cookie
- **WHEN** 向业务写接口提交不带 CSRF Token 的请求
- **THEN** 返回 403 且没有副作用

#### Scenario: 开发代理保护

- **GIVEN** 浏览器从配置的 Next 开发来源访问
- **WHEN** 经代理携带有效 CSRF Token 写入
- **THEN** 正常处理；其他未允许来源的同类请求被拒绝

#### Scenario: 有效机器回调

- **GIVEN** 算法服务持有有效回调凭据且没有浏览器 Cookie
- **WHEN** 发送回调
- **THEN** 按照回调协议验证处理，不要求浏览器 CSRF

### Requirement: 租户隔离与权限更新

系统 SHALL 保留团队成员、项目权限和领域授权规则，对请求和后台执行重新校验权限；无法访问的资源不得泄露其内容或存在性。

#### Scenario: 跨项目读取

- **GIVEN** 用户属于团队 A 而目标资源属于团队 B
- **WHEN** 通过修改 ID 请求目标资源
- **THEN** 返回既有不可见错误，不返回目标数据

#### Scenario: 权限撤销

- **GIVEN** 用户已登录且某项项目权限刚被撤销
- **WHEN** 再次执行操作或后台开始执行其排队任务
- **THEN** 重新授权并拒绝不再允许的操作

#### Scenario: 成员权限兼容

- **GIVEN** 成员拥有现有显式 event:handle 权限
- **WHEN** 执行原本允许的 issue:handle 操作
- **THEN** 继续按原权限兼容规则授权，不因迁移失去对应权限

