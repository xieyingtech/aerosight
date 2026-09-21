# 项目智能体实时语音

新建对话且输入框为空时，发送按钮显示声波图标。点击后授权音频输入，即可与配置的实时模型持续语音交流；输入文字则切回原有文字发送。通话期间使用同一组聊天消息、Markdown 和工具证据组件，支持说话打断和挂断后继续打字。

## 配置与部署

- 在管理后台 → AI Provider 中，选择实时协议 `StepFun Realtime`，填写实时模型 ID（如 `stepaudio-2.5-realtime`），保存并启用为默认 Provider。模型 ID 来自数据库配置，没有代码兜底值。新配置默认关闭实时语音。
- 文字与语音模型分别配置，复用该 Provider 的基础地址与加密 API Key。基础地址支持 StepFun 官方地址或兼容该协议的 HTTPS 网关，仍执行服务端出站地址校验。协议目前支持 StepFun Realtime；不将其当作通用 OpenAI Realtime 协议。
- 配置修改对新接通的会话生效；已接通的会话保持连接时配置。管理页“测试 API 连接”仅检查模型列表端点，实时模型可用性在接通时验证。
- 浏览器需要 HTTPS 或 localhost，以及音频输入权限。采集音频转换为单声道 24 kHz PCM16；浏览器与后端使用同源 WebSocket，后端连接 StepFun 的 `/realtime?model=<配置的实时模型 ID>`。
- 反向代理需转发 `/api/projects/:id/agent-sessions/:sessionId/realtime` 的 WebSocket Upgrade，并允许长连接。Origin 必须匹配 `PUBLIC_ORIGIN`。
- API Key 不下发到浏览器。客户端只能上传音频、挂断或报告播放截断位置；工具配置、执行和项目范围由服务器控制。

## 会话与权限

沿用 `agent:use` 权限及用户自己的开放会话，每个用户最多一通通话，单通最多 15 分钟。每次工具执行和消息保存重新检查权限，空闲连接也定期检查。语音模型通过 `query_project(resource)` 选择查询类别，后端映射到文字聊天的六项只读查询，气泡显示实际执行的工具名；模型不能指定项目或用户范围，每轮最多八次调用。

用户转写、助手转写和最小工具证据存入原有 `agent_messages`，不存原始音频、推理内容或原始工具参数。先分配消息顺序，再更新转写，避免异步 ASR 导致用户问题排在回答后面。挂断、页面离开、连接失败都会释放音频设备与连接；浏览器权限等待可以取消。

协议参考：[StepFun Realtime 开发指南](https://platform.stepfun.com/docs/zh/guide/realtime)、[StepAudio 2.5 Realtime](https://platform.stepfun.com/docs/zh/guides/models/stepaudio-2.5-realtime)。StepFun 的工具定义使用嵌套 `function` 对象，不能直接使用 OpenAI Realtime 的扁平定义。
