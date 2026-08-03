# Sub2API Mobile 与 Codex PC 协同方案

> 文档状态：架构设计基线（2026-07-31）
> 目标读者：产品负责人、后端/Flutter/Tauri 开发者、AI Coding Agent、测试与运维人员

## 结论

该产品可以实现，并且 Sub2API 已经覆盖了大部分移动端基础能力：邮箱密码登录、JWT + Refresh Token、用户 API Key 与分组、OpenAI 兼容网关、图片生成、公告和余额。需要新增的核心只有“设备协同域”：PC 设备在线状态、Codex 会话同步、指令中继、事件同步、幂等计费和审计。

推荐的实现不是模拟键盘向任意终端注入文字，而是由 PC 工具管理一个 `codex app-server` 子进程，通过其 JSON-RPC 协议调用 `thread/list`、`thread/read`、`thread/resume` 和 `turn/start`。这样可以得到结构化会话、流式事件和错误状态，也能跟随本机 Codex 版本生成匹配的协议 Schema。

一个重要边界是：如果同一 Codex thread 已被另一个 Codex/app-server 进程占用，PC 工具只能读取，不能安全地接管写入。产品应提示用户关闭原终端后重试，或显式创建 fork；不得直接修改 rollout JSONL，也不得把文本写进未知终端的 stdin。

## 文档导航

1. [需求整理与可行性](01-requirements-feasibility.md)：范围、角色、功能、验收标准和风险边界。
2. [系统与软件架构](02-system-architecture.md)：总体架构、Flutter/PC/后端分层、数据模型、状态机和目录结构。
3. [API 与实时协议](03-api-realtime-protocol.md)：复用接口、新增 REST/WS 契约、Codex 适配协议和错误码。
4. [AI Coding 开发指南](04-ai-coding-guide.md)：里程碑、任务拆分、编码约束、测试、CI/CD 与交付定义。
5. [UI 生成提示词](05-ui-generation-prompts.md)：移动端与 PC 端可直接复制到 UI 生成工具的提示词。
6. [测试、安全与发布清单](06-test-security-release.md)：端到端场景、安全基线、性能指标和灰度发布。
7. [开发进度](07-development-progress.md)：已落盘实现、验证证据、环境限制和下一步。

## 已确认可复用的现有能力

| 能力 | 现有实现 | 本项目处理方式 |
|---|---|---|
| 邮箱密码登录 | `POST /api/v1/auth/login` | 直接复用，兼容 TOTP 二次验证 |
| Token 自动续期 | `POST /api/v1/auth/refresh`，服务端会轮换 Refresh Token | 移动端与 PC 端分别安全保存并串行刷新 |
| 当前用户与角色 | `GET /api/v1/auth/me` | 路由守卫与管理员入口预留 |
| API Key | `GET /api/v1/keys` 返回 key、group、状态、额度 | 直接复用；敏感值仅进安全存储 |
| 可用分组 | `GET /api/v1/groups/available` | 只展示 `platform=openai` 的可用分组 |
| 文本聊天 | `/v1/responses` 与 `/v1/chat/completions` | MVP 使用 Responses 流式接口，保留 Chat Completions 适配器 |
| GPT Image | `/v1/images/generations`、`/edits` 及异步任务接口 | 移动端优先走异步提交 + 轮询，减少 CDN 长连接风险 |
| 公告 | 用户列表、已读接口、`silent/popup` 模式、定向规则 | 直接复用；启动/回前台刷新并弹窗 |
| 余额 | 用户余额和现有计费基础设施 | 新增协同指令原子直接扣费账本，不复用允许透支的普通扣款函数 |
| 会话浏览参考 | cc-switch 扫描 Codex JSONL、state DB、会话标题 | 仅作为降级读取参考；主路径使用 app-server |
| Codex 控制 | Codex app-server JSON-RPC | PC 工具的正式控制通道 |

## 推荐产品范围

### MVP

- Android App；iOS 保持代码兼容但不交付。
- 单个 Sub2API 站点登录，可登出并切换站点；不同时登录多个站点。
- OpenAI 分组的文本聊天与 GPT Image 生图。
- 底部主导航固定为“聊天、协同、秘钥、我的”；公告列表从“我的”页打开。
- “秘钥”页以卡片展示名称、分组下拉、用量和当前聊天 Key 选择。
- “我的”页提供兑换、充值、公告、设置和关于；充值使用外部浏览器打开 `https://pay.ldxp.cn/shop/codecodeai`。
- 多 PC 设备列表、在线状态、会话查询、会话消息同步、发送新任务。
- PC 工具支持 Windows、macOS、Linux；若人力受限，首发 Windows + Linux。
- 每条被后端成功接收的协同任务在后台收取一次 `0.10 USD`；App 不展示费用、账务状态或二次确认，幂等重试不得重复扣费。
- 不提供 PC 审批中心或电脑确认流程。Companion 使用本机预先配置的非交互安全策略；意外 approval request 直接结束任务，不能自动放宽权限。

### 后续版本

- iOS。
- Anthropic/Gemini/Grok 等其他分组聊天。
- 移动端打断 turn、steer、fork、归档。
- FCM/APNs 后台公告与任务完成推送。
- 管理员专属监控、设备管理、费率配置和审计界面。
- 可选端到端加密会话快照、多站点同时在线。

## 已确认的计费参数与产品策略

协同任务币种和单价已经确认；其余策略按以下基线实现：

1. **协同任务计价单位与单价**：原需求中的“0.1 元”明确指 **0.10 USD**，不是 CNY。MVP 后台配置 `collaboration.task_fee.amount = "0.100000"`、`currency = "USD"`；该金额仅用于后台扣费与审计，App 不展示费用。
2. **扣费时点**：后端完成设备在线、thread 可写、余额和幂等预检后，在成功接收 command 的事务中直接扣费；产品没有冻结、结算和退款流程。
3. **外部占用会话策略**：默认只读并提示关闭原 Codex；可选提供“创建 fork 后继续”，但必须由用户显式确认。

## AI Coding 使用顺序

每次只让 Coding Agent 完成一个里程碑，并要求先引用本目录对应章节：

1. 先完成后端数据库与领域状态机，不接 UI。
2. 完成 WebSocket 中继与伪 PC 客户端测试。
3. 完成 PC 工具的 app-server 适配器与本地集成测试。
4. 完成 Flutter 登录、Key/分组、公告和普通聊天。
5. 最后接入协同 UI、计费和端到端测试。

不要让 Agent 一次生成三个客户端和后端全量代码；协议、幂等、计费和会话锁需要分阶段验收。

## 参考依据

- Sub2API 路由与 DTO：`backend/internal/server/routes/auth.go`、`user.go`、`gateway.go`，`backend/internal/handler/dto/types.go`。
- Sub2API 公告：`backend/internal/handler/announcement_handler.go`、`backend/internal/domain/announcement.go`。
- cc-switch 会话读取：`../cc-switch/src-tauri/src/session_manager/providers/codex.rs`（相对于当前仓库的同级项目）。
- Codex app-server：`../codex/codex-rs/app-server/README.md`（相对于当前仓库的同级项目）。
- OpenAI 当前图片模型和端点能力：[GPT Image 2 模型页](https://developers.openai.com/api/docs/models/gpt-image-2)。模型名必须服务端下发，不应永久硬编码在 App 中。
