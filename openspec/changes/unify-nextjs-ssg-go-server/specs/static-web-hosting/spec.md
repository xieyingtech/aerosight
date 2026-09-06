## Purpose

定义不依赖前端服务端运行时的静态页面交付能力，覆盖构建时的数据隔离、上线后新增资源的访问、旧链接兼容以及开发和生产两种同源访问方式，保证部署简化后页面功能与安全边界可验证。

## ADDED Requirements

### Requirement: 静态构建与运行独立

系统 SHALL 在不连接业务数据库、不提供服务端密钥的条件下生成完整静态前端，运行时由应用服务提供页面与资源，静态产物 MUST 不包含用户或租户私有数据。

#### Scenario: 离线业务依赖构建

- **GIVEN** 构建环境无业务数据库连接和认证密钥
- **WHEN** 执行生产前端构建
- **THEN** 构建成功，产物不包含数据库地址、凭据或租户记录

#### Scenario: 无前端运行时访问

- **GIVEN** 部署环境没有 Node.js 且应用服务已启动
- **WHEN** 浏览器访问登录、项目列表和页面静态资源
- **THEN** 页面壳和资源正常加载，业务数据通过应用 API 获取

### Requirement: 运行时资源路由与旧链接

系统 SHALL 使用固定页面路径与业务 ID 查询参数访问运行时资源，新增资源不得要求重新构建；已知旧 ID 页面 GET/HEAD SHALL 重定向到等价页面，保留筛选上下文。

#### Scenario: 构建后创建资源

- **GIVEN** 当前静态包发布后新建了项目与任务运行
- **WHEN** 访问 /projects/tasks/runs/detail/?projectId=34&runId=56
- **THEN** 页面按参数加载新资源，前端不需要重新构建

#### Scenario: 旧链接访问

- **GIVEN** 用户持有 /projects/34/devices?deviceId=7
- **WHEN** 访问旧链接
- **THEN** 返回 307 到 /projects/devices/?projectId=34&deviceId=7，并保留设备上下文

#### Scenario: 冲突参数

- **GIVEN** 旧路径指定项目 34 而查询参数包含 projectId=99
- **WHEN** 访问该旧链接
- **THEN** 规范化目标使用路径中的项目 34，不访问项目 99

#### Scenario: 无效资源

- **GIVEN** 查询参数缺失或资源不存在或无权访问
- **WHEN** 打开资源页面
- **THEN** 展示缺参或不可用状态，不加载其他项目的默认资源

### Requirement: 资源缓存与路径隔离

系统 SHALL 对哈希资源使用长期不可变缓存，对 HTML 重新验证，对私有 API 禁止缓存；未知 API 与页面 MUST 返回正确类型的 404，静态服务不得暴露任意文件或目录。

#### Scenario: 缓存更新

- **GIVEN** 浏览器缓存了旧 HTML 与哈希资源
- **WHEN** 发布新版本后刷新页面
- **THEN** HTML 重新验证并引用新资源，旧哈希 URL 不被替换成不同内容

#### Scenario: 未知路径与文件穿越

- **GIVEN** 请求目标为未知 API、缺失 chunk 或目录穿越路径
- **WHEN** 访问这些路径
- **THEN** 分别得到 API 404 或资源 404/拒绝，不返回首页或本地私有文件

### Requirement: 开发同源代理

开发环境 SHALL 由前端开发服务器向 Go 代理浏览器 API 与签名资产路径，保留 Cookie、CSRF、SSE 和 Range；生产导出 MUST 不依赖开发代理配置。

#### Scenario: 开发登录

- **GIVEN** 浏览器访问 Next dev，Go 位于另一内部端口
- **WHEN** 通过相对 /api 地址登录并获取 session
- **THEN** 代理正确传递 Cookie 与 Set-Cookie，后续请求登录有效，Go 不反向代理前端

#### Scenario: 开发实时连接

- **GIVEN** 用户已通过开发入口登录
- **WHEN** 订阅 SSE 并随后关闭页面
- **THEN** 事件及时到达，连接取消传递到 Go，资源被释放

#### Scenario: 生产构建隔离

- **GIVEN** 开发配置包含 API 目标地址
- **WHEN** 生成并运行生产发布包
- **THEN** 导出不包含 rewrites 依赖或内部目标地址，API 由 Go 直接提供

