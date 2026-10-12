## Context

ChatMarkdown 对所有链接使用 a 元素。AgentWorkspace 已在共享布局保存会话，客户端导航时会将其转为悬浮面板。Next.js 16.1.0 文档确认 Link 导航复用共享布局状态。

## Goals / Non-Goals

**Goals:** 在既有聊天内容渲染中接通会话保持导航。

**Non-Goals:** 不修改 AgentWorkspace 生命周期，不增加飞行操作，不补造设备数据，不调整概览飞行统计口径。

## Decisions

生产核验发现 `/projects/1/devices/index.txt` 及 `__next.*.txt` 未映射，导致客户端导航失败。沿用已校验的 legacyPageURL，仅将其对应页面目录下的导出导航文本映射到构建产物。拒绝点段，未知页面及不存在的文件仍返回 404。增加 Go HTTP 边界测试和使用真实导出文件的本地浏览器测试。

根相对 `/projects/` 链接使用 Next.js Link；prefetch=false，避免多个查询结果提前触发资源加载。其余链接继续使用 a。相比为每个链接自定义 click/router.push，Link 保持键盘和修饰键等常规行为。绝对 URL 仍按现有外链方式处理。

## Risks / Trade-offs

- 链接指向未授权项目 → 页面及 API 继续校验当前成员权限。
- 模型给出错误资源链接 → 本改动不重写其语义，录制时逐项核验真实结果。
- 共用 Markdown 渲染影响其他页面 → 验证外链属性及 API 链接不被拦截。

## Migration Plan

无迁移；本地录制同一会话跳转并返回，构建通过后部署。回滚单个渲染组件即可。
