# 系统与软件架构

## 1. 架构决策摘要

| 决策 | 选择 | 原因 |
|---|---|---|
| 移动端 | Flutter + Riverpod + Dio + GoRouter | 符合既定技术栈，便于后续 iOS |
| 移动端组织 | Feature-First + Clean Architecture | 功能边界清晰，适合 AI 分里程碑生成 |
| PC 工具 | Tauri 2 + Rust + React/TypeScript | 与 cc-switch 类似，跨平台且能安全管理本地进程/Keyring |
| Codex 接入 | `codex app-server --stdio` JSON-RPC | 结构化、可流式、跟随本地 Codex 协议，不解析 TUI ANSI |
| 互联网链路 | App 和 PC 都主动连接 Sub2API | 不开放 PC 入站端口，适应 NAT/防火墙 |
| 实时传输 | HTTPS REST 做命令；WSS 做事件/中继 | REST 易做幂等和计费，WS 适合在线状态和流式事件 |
| 权威存储 | PostgreSQL 保存设备/任务/账本；Redis 保存在线路由和短暂 payload | 计费可靠，源码/会话内容最小化持久化 |
| 普通聊天历史 | 默认只保存在移动设备 | 不扩大 Sub2API 数据责任边界 |
| 协同计费 | 预检成功后在 command 创建事务中直接扣费 | 后台固定收费、幂等且不向 App 暴露账务流程 |
| 活跃会话冲突 | 只读、重试或显式 fork | 尊重 app-server thread 所有权，不破坏 rollout |

## 2. 总体架构

```mermaid
flowchart LR
    A[Flutter Android App] -- HTTPS / WSS --> B[Sub2API Go Backend]
    P[Tauri PC Companion] -- HTTPS / WSS 出站 --> B
    B --> PG[(PostgreSQL)]
    B --> R[(Redis)]
    B -- OpenAI-compatible gateway --> U[OpenAI upstream accounts]
    P -- JSON-RPC over stdio --> C[codex app-server]
    C --> F[(Codex thread store / rollouts)]
    C --> W[Local workspace and tools]
```

信任边界：

- App 只信任用户配置的 Sub2API HTTPS Origin。
- PC 只接收后端定义的协同协议，不接收任意 shell 命令。
- Sub2API 以 JWT 的 `user_id` 为唯一租户边界。
- app-server 仍然是本机文件、命令和 Codex thread 的权威；PC 工具不能绕过其沙箱与权限模型，也不提供审批界面。

## 3. 核心业务流

### 3.1 登录与续期

```mermaid
sequenceDiagram
    participant M as Mobile/PC
    participant S as Sub2API
    M->>S: POST /api/v1/auth/login
    alt 用户启用 TOTP
        S-->>M: requires_2fa + temp_token
        M->>S: POST /api/v1/auth/login/2fa
    end
    S-->>M: access_token + refresh_token + expires_in + user
    M->>M: 安全存储 token pair
    M->>S: Authorization: Bearer access_token
    S-->>M: 401 token expired
    M->>S: POST /api/v1/auth/refresh (只允许一个并发刷新)
    S-->>M: 新 access_token + 新 refresh_token
    M->>M: 原子替换 token pair 并重放等待请求
```

移动端与 PC 必须使用独立 token pair。不要把移动端 Refresh Token复制给 PC，也不要用 API Key 代替面板 JWT 登录。

### 3.2 Key/分组与普通聊天

1. App 使用 JWT 拉取 `/keys` 和 `/groups/available`，在底部“秘钥”页渲染卡片列表。
2. 卡片内的分组下拉栏通过服务端更新接口修改 Key 的绑定分组；完成后刷新 Key 与能力状态。
3. 用户选择唯一“当前聊天使用”的 Key；App 使用其安全存储凭证调用 `/v1/models` 和网关接口。
4. 新建本地 conversation，保存 `site_id/key_id/group_id/model` 快照；当前 Key 选择独立保存为用户偏好。
5. 文本请求调用 `/v1/responses` 并消费 SSE；实现层保留 Chat Completions adapter。
6. 消息和生成状态写入本地数据库；Key 只引用安全存储中的别名。

不要在同一 conversation 中静默改变 Key、group 或 provider。用户切换时默认创建新 conversation。

### 3.3 公告

1. App 冷启动登录完成、回前台、用户从“我的 → 公告”打开列表或下拉刷新时调用 `/announcements`。
2. Store 先合并本地已展示集合，再选择最早的未读 `popup` 公告。
3. 用户点击“我知道了”后调用 `POST /announcements/{id}/read`。
4. 网络失败时本地记录 dismissed，后台重试 mark-read；下一次进程启动仍以服务端 read state 为准。

### 3.4 会话查询

```mermaid
sequenceDiagram
    participant A as App
    participant S as Sub2API
    participant P as PC Companion
    participant C as codex app-server
    A->>S: POST device/{id}/session-syncs
    S->>S: 验证 user_id、设备在线，创建 sync_id
    S-->>A: 202 sync_id
    S-->>P: WS session.list.request
    P->>C: thread/list (分页)
    C-->>P: threads
    P->>S: WS session.list.snapshot
    S->>S: Redis 缓存短期快照
    S-->>A: WS session.list.updated(sync_id)
    A->>S: GET session-syncs/{sync_id}
    S-->>A: 规范化会话列表
```

PC 返回的是规范化 DTO，不返回本机 `source_path`、绝对 rollout 路径或环境变量。

### 3.5 会话消息同步

1. App 创建 thread sync request，携带 `after_item_id` 或本地 cursor。
2. PC 对未加载 thread 调用只读 `thread/read`；大历史优先分页 `thread/turns/list`/`thread/items/list`。
3. PC 将 app-server item 映射为移动 DTO：message、reasoning summary、command、file change、tool、error。
4. 后端仅用 Redis 短暂中继/缓存，PostgreSQL 不保存正文。
5. App 以 `(device_id, thread_id, item_id)` 去重并写入本地数据库。

### 3.6 发送任务与计费

```mermaid
stateDiagram-v2
    [*] --> accepted: 预检并原子扣费
    accepted --> dispatched: 推送到 PC
    dispatched --> started: thread/resume + turn/start 成功
    started --> completed: turn/completed
    started --> failed: turn/completed failed/interrupted
    accepted --> failed: 中继失败/超时
    dispatched --> failed: PC 拒绝/外部占用/启动失败
    completed --> [*]
    failed --> [*]
```

实现注意：扣费在 `accepted` 创建事务内一次完成，不存在 held/settled/refunded 状态。App 仅消费业务状态，账务明细只用于后台审计。

## 4. Flutter App 架构

### 4.1 分层规则

每个 feature 使用四层：

- `presentation`：页面、Widget、Riverpod controller/state、GoRouter route。
- `domain`：Entity、Value Object、Repository 接口、Use Case；不依赖 Dio/Flutter UI。
- `data`：DTO、Mapper、Remote/Local DataSource、Repository 实现。
- `application` 可选：跨多个 Use Case 的协调器。小 feature 可并入 presentation controller。

共享核心：

- `core/network`：两个 Dio client。Panel client 使用 JWT；Gateway client 使用 API Key。禁止在一个 interceptor 中混合两种凭证。
- `core/auth`：单飞 refresh、token rotation、站点隔离、登出广播。
- `core/realtime`：WSS 生命周期、heartbeat、重连、事件分发。
- `core/storage`：Secure Storage 保存 secret；Drift/SQLite 保存非 secret 业务数据。
- `core/router`：auth/user/admin 分支和深链守卫。
- `core/logging`：字段级 redact；release 默认不记 request/response body。

### 4.2 Riverpod 状态边界

- `activeSiteProvider`：当前站点 Origin 和站点 ID。
- `authSessionProvider`：未认证、登录中、待 2FA、已认证、刷新中、失效。
- `apiKeyCatalogProvider`：秘钥卡片列表、分组下拉更新和用量；State 中只保留 Key ID/脱敏值。
- `selectedChatProfileProvider`：全局当前聊天 Key 的 `keyId + groupId + model`；聊天和生图统一读取。
- `conversationControllerProvider.family(conversationId)`：消息、流式 buffer、错误、停止句柄。
- `announcementControllerProvider`：公告、未读和 popup 队列。
- `deviceListProvider`：设备在线状态。
- `codexSessionListProvider.family(deviceId)`：同步任务、查询、分页。
- `codexThreadControllerProvider.family(ThreadRef)`：消息 cursor、实时事件、发送状态。

禁止在 Widget 中直接调用 Dio、Secure Storage 或 WebSocket。

### 4.3 GoRouter 路由

```text
/bootstrap
/site/setup
/auth/login
/auth/2fa
/app
  /chat
  /chat/:conversationId
  /images/new
  /collab
  /collab/devices/:deviceId/sessions
  /collab/devices/:deviceId/threads/:threadId
  /keys
  /profile
    /announcements
    /settings
/admin                 # role=admin 才可进入
  /coming-soon         # MVP 占位
```

路由守卫顺序：站点是否有效 → token 是否可恢复 → 用户角色 → feature flag。禁止只在 UI 隐藏管理员入口而不做路由守卫。

### 4.4 本地数据

建议表：

- `sites`：origin、display_name、last_used_at；不存 token。
- `chat_conversations`：配置快照、标题、时间。
- `chat_messages`：role、content、status、remote_item_id、created_at。
- `generated_images`：task_id、local_path、prompt、model、status。
- `codex_thread_cache`：device/thread 元数据，不含本机路径。
- `codex_items`：已同步的规范化 item。
- `announcement_seen`：用于抑制同进程重复 popup，不替代服务端 read state。

Secure Storage key 必须包含站点 hash 和账号 user ID，防止站点切换串 token。

## 5. PC Companion 架构

### 5.1 进程结构

```mermaid
flowchart TB
    UI[React UI] --> CMD[Tauri Commands]
    CMD --> AUTH[Auth Service]
    CMD --> DEV[Device Service]
    CMD --> SES[Session Service]
    DEV --> WS[Relay WebSocket Client]
    SES --> ADP[Codex AppServer Adapter]
    ADP --> PROC[Managed codex app-server process]
    AUTH --> KR[OS Keyring]
    SES --> DB[(Local SQLite metadata/spool)]
```

### 5.2 Codex adapter 职责

- 查找 `codex` binary，运行 `codex --version`。
- 可选运行 `codex app-server generate-json-schema` 到版本缓存目录，生成当前安装版本的 Schema。
- 启动 `codex app-server --stdio`，分离 stdout(JSONL) 与 stderr(log)。
- 完成 `initialize` request + `initialized` notification。
- 维护 JSON-RPC request ID → oneshot response map。
- 调用 thread/list/read/resume、turn/start/interrupt；消费 item/turn 和 server request。
- 不提供 Approval Service 或审批 UI。启动时使用用户在本机预先配置的非交互安全策略；遇到 approval/request_user_input 时返回拒绝/unsupported，并把任务标记为 `approval_required` 失败，不自动扩大权限。
- 子进程退出时标记设备 degraded，指数退避重启；正在执行的任务标记失败并保留审计状态。

### 5.3 会话可写性

会话列表需要额外的 `write_state`：

- `writable_loaded`：由当前 Companion 的 app-server 加载且可接受 direct input。
- `writable_resumable`：未加载，但可尝试 resume。
- `busy_external`：另一个 app-server/Codex 进程拥有写锁。
- `read_only`：归档、子代理限制或协议不支持。
- `unknown`：只读 list 无法确定，发送前再次探测。

如果 `thread/read.canAcceptDirectInput=false`，不显示可写。若该字段为 null，仅允许在发送时尝试 resume，不提前承诺。

### 5.4 cc-switch 可复用思想

可以借鉴：

- 会话标题取显式 state DB title，退化到第一条用户消息和目录名。
- 过滤 subagent、环境注入和 AGENTS.md 注入，避免把它们作为标题。
- 只读扫描 sessions + archived_sessions 的兼容策略。
- 消息中把 function call/output 变成可折叠工具项。

不要直接复制：

- `source_path` 不能发给手机。
- `codex resume <id>` 只用于打开终端，不是结构化协同写入。
- 直接读取 JSONL 只作为 app-server 不可用时的只读降级，不能写回 JSONL。

## 6. Go 后端架构

### 6.1 模块边界

新增 `collaboration` 领域，遵循现有层次：

- Handler：REST 参数、JWT subject、响应 envelope、WS 协议边界。
- Service/Domain：设备归属、sync、command 状态机、计费策略、事件授权。
- Repository：PostgreSQL 事务、CAS、分页；不得把业务状态机散落在 Handler。
- Realtime Hub：进程内连接表 + Redis pub/sub/stream，支持多实例。
- Sweepers：过期设备状态、过期 command 状态和短暂 payload 清理；不执行退款。

建议依赖方向：

```text
handler/collaboration
  -> service/collaboration ports + domain
      -> repository implementations
      -> realtime hub interface
      -> billing cache invalidator interface
```

### 6.2 PostgreSQL 数据模型

#### collaboration_devices

| 字段 | 类型 | 说明 |
|---|---|---|
| id | UUID | 设备 ID |
| user_id | BIGINT FK | 租户边界 |
| name | VARCHAR | 用户可改名称 |
| platform | VARCHAR | windows/macos/linux |
| companion_version | VARCHAR | 客户端版本 |
| codex_version | VARCHAR | 本机 Codex 版本 |
| status | VARCHAR | offline/online/degraded/revoked |
| capabilities | JSONB | 协议能力，不含 secret |
| last_seen_at | TIMESTAMPTZ | 心跳时间 |
| revoked_at | TIMESTAMPTZ nullable | 撤销时间 |
| created_at/updated_at | TIMESTAMPTZ | 审计 |

唯一约束由安装 ID 决定，但安装 ID 应先 hash，再与 user 绑定；重新登录其他账号不得继承原设备授权。

#### collaboration_sync_requests

| 字段 | 类型 | 说明 |
|---|---|---|
| id | UUID | sync_id |
| user_id/device_id | FK | 租户和目标设备 |
| kind | VARCHAR | session_list/thread_snapshot |
| thread_id | VARCHAR nullable | thread sync 时填写 |
| cursor | VARCHAR nullable | 增量游标 |
| status | VARCHAR | pending/running/completed/failed/expired |
| error_code | VARCHAR nullable | 规范化错误 |
| expires_at | TIMESTAMPTZ | 短期任务 |
| timestamps | TIMESTAMPTZ | 创建/完成 |

正文快照放 Redis，表中只保存状态和摘要元数据。

#### collaboration_commands

| 字段 | 类型 | 说明 |
|---|---|---|
| id | UUID | command_id |
| user_id/device_id | FK | 所属用户和 PC |
| thread_id | VARCHAR | 不透明 thread ID |
| idempotency_key | UUID | 客户端重试键 |
| prompt_sha256 | CHAR(64) | 审计去重，不存明文 |
| prompt_bytes | INT | 大小审计 |
| status | VARCHAR | accepted/dispatched/started/completed/failed/expired |
| turn_id | VARCHAR nullable | app-server turn ID |
| error_code | VARCHAR nullable | 规范化错误 |
| created_at/started_at/completed_at | TIMESTAMPTZ | 生命周期 |

唯一约束：`UNIQUE(user_id, idempotency_key)`。

#### collaboration_charges

| 字段 | 类型 | 说明 |
|---|---|---|
| id | UUID | charge_id |
| command_id | UUID UNIQUE | 一任务一账单 |
| user_id | BIGINT | 所属用户 |
| amount | NUMERIC | MVP 固定 `0.100000` |
| currency | VARCHAR | MVP 固定 `USD`，不做 CNY 换算 |
| status | VARCHAR | charged |
| balance_before/after | NUMERIC | 对账快照 |
| charged_at | TIMESTAMPTZ | 扣费时间 |
| reason | VARCHAR nullable | 审计原因 |

不得使用 `float64 ==` 做账务幂等判断；迁移中明确 NUMERIC 精度，并在 Go 领域使用 decimal 或最小单位。

### 6.3 Redis Key 设计

```text
collab:presence:user:{userId}:device:{deviceId}       TTL 45s
collab:conn:device:{deviceId}                         实例路由
collab:session-snapshot:{syncId}                      TTL 5m
collab:thread-snapshot:{syncId}                       TTL 10m
collab:command-payload:{commandId}                    TTL 2m-5m
collab:events:user:{userId}                           Redis Stream（短保留）
collab:rate:{userId}:{action}                         限流
```

Redis 丢失时允许在线状态短暂失真，但不能重复扣费；command 与 charge 的唯一约束仍以 PostgreSQL 为准。

## 7. 计费事务设计

MVP 协同任务费率为每个被后端成功接收的任务 `0.10 USD`。服务端配置和账本使用 `amount = "0.100000"`、`currency = "USD"`；客户端不得传入、展示或换算该金额。普通聊天和 GPT Image 继续使用 Sub2API 现有渠道费率。

### 7.1 预检与直接扣费

一个事务内：

1. 按 `(user_id, idempotency_key)` 查询；存在则返回原 command。
2. 验证设备未撤销，并以 Redis presence 判断在线（在线只作为前置优化）。
3. 验证最近一次同步快照中的 thread 可写且无外部占用；失败时不创建任务、不扣费。
4. 读取服务端费率并锁定用户余额行；`balance >= fee` 才执行 `balance -= fee`。
5. 在同一事务插入 command(accepted) 与 charge(charged)，二者都受幂等键/唯一约束保护。
6. 提交后将 prompt payload 写 Redis 并派发；后续中继或 Codex 执行失败只更新 command 状态，不退款。

### 7.2 后续状态

PC 调用 `turn/start` 获得 turn ID 后，仅更新业务状态：

- `command accepted/dispatched -> started`
- `command started -> completed/failed`
- `charge` 始终保持 `charged`

每个 UPDATE 都带旧状态条件并检查 affected rows。产品不提供冻结、结算或退款状态机。

## 8. 安全架构

- 移动端 API Key 仅用于网关域名；Dio redirect 必须关闭或在 redirect 时剥离 Authorization。
- Site Origin 规范化为 scheme + host + optional port，拒绝路径、userinfo、fragment。
- PC WebSocket 使用 Access JWT 的 Authorization header；Refresh Token 永不走 WS。
- 每条 WS event 由服务端基于连接 subject 重写 user/device 上下文，忽略客户端声明的 user_id。
- 设备撤销立即关闭连接并拒绝新任务；已经成功接收并扣费的任务不退款。
- PC 收到 task 后再次校验 command_id、thread_id 来自已同步列表，prompt 字节上限建议 32 KiB。
- 移动端上传参考图设置像素/文件大小上限；服务端继续使用现有 body limit 和内容审核。
- 协同审计保存 hash、长度、状态、错误，不保存源码、命令输出和完整 prompt。
- 管理员操作另加 step-up/TOTP 与审计，不使用普通 user endpoint。

## 9. 推荐项目目录

若保持单仓库，建议在现有根目录新增 `clients/`，而不把移动端塞进 Vue `frontend/`：

```text
sub2api/
├── backend/
│   ├── ent/schema/
│   │   ├── collaboration_device.go
│   │   ├── collaboration_command.go
│   │   └── collaboration_charge.go
│   └── internal/
│       ├── domain/collaboration/
│       ├── handler/collaboration/
│       ├── repository/collaboration_*.go
│       ├── server/routes/collaboration.go
│       └── service/collaboration/
├── clients/
│   ├── mobile/
│   │   ├── android/
│   │   ├── ios/                       # 保留但 MVP 不发布
│   │   ├── lib/
│   │   │   ├── app/
│   │   │   ├── core/
│   │   │   ├── features/
│   │   │   │   ├── site/
│   │   │   │   ├── auth/
│   │   │   │   ├── api_keys/
│   │   │   │   ├── chat/
│   │   │   │   ├── image_generation/
│   │   │   │   ├── announcements/
│   │   │   │   ├── devices/
│   │   │   │   ├── codex_sessions/
│   │   │   │   └── admin_placeholder/
│   │   │   └── l10n/
│   │   └── test/
│   └── codex-pc/
│       ├── src/                        # React UI
│       │   ├── features/
│       │   ├── lib/api/
│       │   └── types/
│       ├── src-tauri/src/
│       │   ├── commands/
│       │   ├── services/
│       │   ├── adapters/codex_app_server/
│       │   ├── realtime/
│       │   ├── auth/
│       │   ├── storage/
│       │   └── redaction/
│       └── tests/
├── docs/mobile-codex/
└── protocol/
    ├── collaboration-events.schema.json
    └── collaboration-openapi.yaml
```

如果拆成三个仓库，目录层次保持不变，并将 `protocol/` 发布为版本化 artifact；不要手工维护三份 DTO。

## 10. 可观测性

必须有以下无敏感信息指标：

- 在线设备数、WS 连接/重连数。
- session/thread sync 延迟、超时率和结果数量。
- command 各状态数量与状态停留时间。
- 重复幂等请求次数、duplicate charge 次数、command/charge 不一致次数。
- app-server 启动失败、协议不兼容、thread busy external 次数。
- 普通聊天和图片仍使用现有 usage/billing 指标，不混入协同固定费账本。

日志关联键：`request_id`、`sync_id`、`command_id`、`device_id`、`thread_id_hash`。禁止记录原始 thread ID 之外的本机路径，生产日志可进一步 hash thread ID。
