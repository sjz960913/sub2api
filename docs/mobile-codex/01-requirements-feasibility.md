# 需求整理与可行性

## 1. 产品目标

构建一个以自托管 Sub2API 站点为身份、余额和 AI 网关中心的 Android App，以及一个安装在用户电脑上的 Codex PC 协同工具。用户在 App 中既可以使用自己的 Sub2API API Key 聊天和生成图片，也可以查看本机 Codex 会话、同步消息，并向可写会话提交新的任务。

产品由三部分组成：

- `Sub2API Mobile`：Flutter Android 客户端。
- `Codex PC Companion`：Tauri 2 + Rust + React/TypeScript 桌面工具。
- `Sub2API Collaboration Backend`：集成在现有 Go 后端中的协同、实时中继和计费模块。

## 2. 用户与系统角色

| 角色 | 权限与职责 |
|---|---|
| 普通用户 | 登录站点、管理和选择自己的 Key、聊天、生图、看公告、兑换/充值、绑定自己的 PC、读写自己的 Codex 会话 |
| 管理员 | 拥有普通用户能力；后续进入管理员导航分支；当前仅预留入口和资源 |
| Mobile Client | 发起业务请求、保存本地聊天、订阅设备和 Codex 事件 |
| PC Companion | 登录同一站点、维护设备在线状态、控制本机 app-server、上报会话与事件 |
| Sub2API Backend | 认证、租户隔离、指令中继、计费、审计、公告和 AI 网关 |
| Codex app-server | 本机 Codex thread/turn/item 的权威读写接口 |

## 3. 规范化功能需求

### FR-AUTH 认证与站点

- FR-AUTH-01：用户输入 HTTPS Sub2API 站点地址、邮箱和密码登录。
- FR-AUTH-02：兼容服务端返回的 TOTP 2FA 流程。
- FR-AUTH-03：Access Token 到期后使用 Refresh Token 自动续期，并正确处理 Refresh Token 轮换。
- FR-AUTH-04：移动端和 PC 端各自持有独立登录会话；二者必须解析为同一个 `user_id` 才能协同。
- FR-AUTH-05：用户可登出；登出清理本机密钥、WebSocket 和本地敏感缓存。
- FR-AUTH-06：仅允许 HTTPS 生产站点；开发构建可在显式开关下访问局域网 HTTP。

### FR-KEY API Key 与分组

- FR-KEY-01：底部主导航提供独立“秘钥”页，以卡片列表展示当前用户的 API Key。
- FR-KEY-02：每张卡片展示 Key 名称、脱敏 Key、状态、当前分组、额度/用量和有效期。
- FR-KEY-03：卡片内提供分组下拉栏；只列出当前站点可用且 `platform=openai` 的分组，修改后通过服务端 Key 更新接口持久化绑定关系。
- FR-KEY-04：每张卡片提供“设为当前”操作；全局同一时间只有一个“当前聊天使用”的 Key，普通聊天和 GPT Image 都通过该 Key 调用网关。
- FR-KEY-05：只允许将 `active`、未过期、有可用 OpenAI 分组的 Key 设为当前；当前 Key 失效时清除选择并引导用户重新选择。
- FR-KEY-06：当前 Key 选择保存 `key_id` 与安全存储 alias，不在 Drift、日志、Crash Report、路由参数或普通 SharedPreferences 保存明文 Key。

### FR-CHAT 普通聊天

- FR-CHAT-01：创建本地会话，选择 Key、模型和可选系统提示词。
- FR-CHAT-02：支持流式文本、Markdown、代码块、复制、停止生成、重试和错误重发。
- FR-CHAT-03：模型列表从选中 Key 对应的 `/v1/models` 获取并缓存，不能只写死一个模型。
- FR-CHAT-04：MVP 仅支持 OpenAI 分组；其他分组显示“即将支持”且不可选。
- FR-CHAT-05：普通聊天消息默认只保存在设备本地；不新增服务端聊天历史。
- FR-CHAT-06：切换 Key/模型时创建新会话或明确截断上下文，避免混用不同提供商上下文。

### FR-IMAGE GPT Image

- FR-IMAGE-01：从聊天页进入生图模式，输入 prompt、尺寸、质量、数量和可选参考图。
- FR-IMAGE-02：服务端能力允许时，优先调用 `/v1/images/generations/async` 或 `/edits/async`，轮询 `/v1/images/tasks/{task_id}`。
- FR-IMAGE-03：展示排队、生成、完成、失败、取消/超时状态。
- FR-IMAGE-04：图片保存到 App 私有目录；用户显式操作后再导出到系统相册。
- FR-IMAGE-05：模型和参数从站点能力配置生成；以当前官方模型为默认候选，但不硬编码为永久唯一值。
- FR-IMAGE-06：分组必须启用图片生成权限，否则在提交前给出明确提示。

### FR-ANN 公告

- FR-ANN-01：登录后和 App 回到前台时拉取用户可见公告。
- FR-ANN-02：支持全部/未读列表与标记已读。
- FR-ANN-03：`notify_mode=popup` 的未读公告在前台弹窗；同一公告同一设备会话只弹一次，用户确认后调用已读接口。
- FR-ANN-04：“公告”入口位于“我的”页，点击后弹出公告列表；`silent` 公告只显示在该列表和未读角标中。
- FR-ANN-05：MVP 不承诺 App 被系统杀死后的后台推送；FCM/APNs 单独立项。

### FR-NAV 主导航与账户操作

- FR-NAV-01：底部主导航固定为“聊天、协同、秘钥、我的”，顺序不可因用户角色改变。
- FR-NAV-02：“兑换”位于“我的”页，点击后弹出兑换码输入 Dialog，提交既有兑换接口并刷新余额。
- FR-NAV-03：“充值”位于“我的”页，点击后使用系统外部浏览器打开 `https://pay.ldxp.cn/shop/codecodeai`；不得在内嵌 WebView 中携带 JWT、Refresh Token 或 API Key。
- FR-NAV-04：“我的”页同时提供公告、设置、关于与退出登录；管理员入口仍作为额外菜单项，不新增底栏项目。

### FR-DEVICE PC 设备

- FR-DEVICE-01：PC 端使用同一 Sub2API 站点邮箱密码登录，并自动续期。
- FR-DEVICE-02：PC 注册稳定的设备 ID，展示设备名、平台、Companion 版本、Codex 版本和最后在线时间。
- FR-DEVICE-03：PC 只建立到 Sub2API 的出站 TLS WebSocket，不监听公网端口。
- FR-DEVICE-04：移动端可查看、重命名、撤销自己的设备；不得访问其他用户设备。
- FR-DEVICE-05：多个 PC 在线时，用户必须先选择设备再查询会话。

### FR-SESSION 会话发现与同步

- FR-SESSION-01：用户点击“查询”后，后端向目标在线 PC 发出会话同步请求。
- FR-SESSION-02：PC 通过 app-server `thread/list` 获取会话；结果包含 thread ID、标题、工作目录、更新时间、状态、是否归档和可写性。
- FR-SESSION-03：列表支持搜索、工作目录筛选、状态筛选、分页和手动刷新。
- FR-SESSION-04：用户进入会话后，PC 通过 `thread/read(includeTurns=true)` 或分页历史 API 返回可展示消息。
- FR-SESSION-05：点击“同步”只拉取最新增量；客户端以稳定 item ID 去重。
- FR-SESSION-06：工具调用和命令输出应显示为可折叠事件卡，不把全部原始日志塞进聊天气泡。
- FR-SESSION-07：后端默认不长期持久化完整 Codex 内容；移动端持久化用户已经同步到本机的内容。

### FR-COMMAND 协同任务

- FR-COMMAND-01：用户输入文本并发送，客户端生成 UUID `idempotency_key`。
- FR-COMMAND-02：后端验证移动端用户、PC 用户、设备所有者和 thread 所有者均为同一 `user_id`。
- FR-COMMAND-03：后端先完成 PC 在线、thread 可写、余额和幂等预检；接收任务时由后台原子扣取一次固定任务费并中继任务。
- FR-COMMAND-04：PC 对未加载 thread 先执行 `thread/resume`，成功后调用 `turn/start`。
- FR-COMMAND-05：MVP 不设计冻结、结算或退款流程；任务一旦被后端成功接收即完成扣费，后续执行失败不退款。
- FR-COMMAND-06：相同用户和相同 `idempotency_key` 永远只创建一条任务、最多扣一次费用。
- FR-COMMAND-07：实时状态只包括 queued、delivered、started、completed、failed；App 不显示任何账务状态。
- FR-COMMAND-08：产品不提供电脑审批流程。PC Companion 使用本机明确配置的非交互安全策略；若 app-server 仍返回 approval request，立即以 `approval_required` 结束任务，不弹出 PC 确认界面，也不自动扩大权限。
- FR-COMMAND-09：同一 thread 同时只允许一个普通 turn；正在执行时，新消息默认排队，不自动调用 `turn/steer`。

### FR-BILL 计费

- FR-BILL-00：每次被后端成功接收的移动端协同任务费率固定为 `0.10 USD`；原需求中的“0.1 元”指 USD，不是 CNY。该费用是后台行为，App 不展示金额、确认条或账务状态。
- FR-BILL-01：费率由后端设置，不信任客户端传入金额。
- FR-BILL-02：金额使用数据库现有余额精度策略；禁止 float 在协议层反复换算，内部建议用十进制定点/最小单位。
- FR-BILL-03：余额不足时不创建可执行任务，返回明确错误和当前可用余额。
- FR-BILL-04：扣费、command 创建与幂等键写入在同一数据库事务内完成；不创建 held/refunded 状态。
- FR-BILL-05：协同收费记录仅供后端审计和管理员后续能力使用，移动端 MVP 不提供收费记录界面。
- FR-BILL-06：后端成功接收任务后即扣费；PC 中继或 Codex 后续执行失败不触发退款。

### FR-ADMIN 管理员预留

- FR-ADMIN-01：登录结果角色为 admin 时，在导航和路由表中注册 Admin 入口。
- FR-ADMIN-02：MVP 入口显示开发中，占位页面使用正式 Design Token、图标和本地化 key。
- FR-ADMIN-03：预留功能：在线设备、协同任务、收费账本、费率开关、公告管理和服务健康。
- FR-ADMIN-04：普通用户无法通过深链进入管理员路由。

## 4. 非功能需求

### 安全

- 所有生产通信使用 TLS；JWT 不放 URL query。
- Refresh Token、Sub2API API Key、设备凭证分别存入 Android Keystore 和桌面 OS Keyring。
- 任何日志中禁止出现密码、JWT、Refresh Token、API Key、完整 Codex prompt 和文件内容。
- 所有协同资源查询必须同时带 `user_id` 和 `device_id` 条件，禁止只按 UUID 查询。
- PC 不接受后端下发的 shell 字符串；后端只能下发文本任务和协议动作，实际执行由 app-server 权限系统决定。
- 会话 ID 视为不透明字符串，移动端不得提交本机路径。

### 性能与可靠性

- 登录与普通列表接口 P95 < 500 ms（不含公网延迟）。
- 在线 PC 的会话查询 P95 < 3 s；超时 10 s。
- 任务从 App 提交到 PC 收到 P95 < 1 s。
- WebSocket 断线使用指数退避 + 抖动，最大 30 s；恢复后重新订阅状态。
- Redis 短时故障不允许造成重复扣费；计费以 PostgreSQL 为准。
- 单用户/单设备/单 thread 都有明确的速率与并发限制。

### 可维护性

- Flutter：Feature-First + Clean Architecture、Riverpod、Dio、GoRouter。
- PC：Tauri Commands → Services → Adapters/Repositories；React UI 不直接调用进程或文件系统。
- Go：handler → service/domain → repository，沿用现有依赖注入和响应封装。
- 协议必须版本化，事件字段只增不删；未知事件由客户端安全忽略。

## 5. 可行性矩阵

| 需求 | 可行性 | 说明 |
|---|---|---|
| 登录与自动续期 | 已具备 | 现有登录会返回 token pair；还需移动/桌面安全存储 |
| 获取 Key 与分组 | 已具备 | Key DTO 当前包含完整 key；必须加强客户端脱敏和日志策略 |
| OpenAI 聊天 | 已具备 | 网关支持 Responses 与 Chat Completions |
| GPT Image | 已具备 | 同步和异步图片路由均已存在 |
| 公告与 popup | 已具备 | 服务端已有 `notify_mode=popup` 和用户已读状态 |
| Codex 会话列表 | 可实现 | app-server 主路径；cc-switch JSONL 扫描作为兼容参考 |
| Codex 消息读取 | 可实现 | `thread/read`/分页接口；旧 Codex 可降级只读 JSONL |
| 向会话发消息 | 有条件可实现 | PC 工具必须拥有 thread 写锁；外部活跃会话不能透明注入 |
| 实时同步 | 可实现 | app-server item/turn 通知 → PC → Sub2API WS → App |
| 每条任务扣费 | 可实现 | 需要新增 command + charge 原子直接扣费事务和幂等键 |
| 同用户链路 | 可实现 | 所有资源由 JWT 的 `user_id` 绑定；设备注册归属用户 |
| 管理员功能预留 | 可实现 | 先做路由、资源、权限和 Design Token 占位 |

## 6. 不能错误承诺的边界

1. **不能保证控制任意正在运行的 Codex TUI。** app-server 对活跃 thread 有所有权约束；另一个进程占用时应只读、重试或 fork。
2. **cc-switch 不是写入方案。** 它当前扫描 JSONL、读取 state DB 标题并执行 `codex resume <id>`，没有结构化远程发送能力。
3. **不能把 Key 与分组当作两个独立路由参数。** Sub2API 的网关分组绑定在 Key 上；App 的“分组选择”应筛选可用 Key。
4. **不能使用 CNY 或进行币种换算。** 协同任务计价明确为 `0.10 USD`；后台配置、账本和对账始终使用 USD，App 不展示该金额。
5. **不能依赖 WebSocket 恰好送达来计费。** 计费必须基于 PostgreSQL 状态机和幂等键。
6. **不能默认在服务端永久保存源码和会话全文。** MVP 只短暂中继，持久审计保存 hash、长度、状态和时间。

## 7. MVP 验收标准

- 新安装 Android App 能配置站点、登录、自动刷新 Token、登出。
- 能列出 OpenAI Key/分组并完成一次流式文字聊天。
- 能提交一次异步图片生成，看到进度并保存结果。
- popup 公告只弹一次，标记已读后不再弹。
- PC 登录相同账号后，App 在 5 秒内显示在线设备。
- 点击查询能看到 app-server 返回的普通 CLI thread，搜索和选择有效。
- 进入 thread 后能读取用户/助手消息，点击同步只新增未见 item。
- 对可写 thread 发送任务，App 能看到 turn 流式输出和完成状态，余额只扣一次。
- PC 离线、余额不足、外部占用、重复点击和服务重启场景均不会永久错误扣款。
- 用户 A 无法查询、订阅或提交到用户 B 的设备/thread。
- 所有自动化测试日志均不出现完整 Key、JWT 或 prompt。
