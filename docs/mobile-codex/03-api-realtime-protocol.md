# API 与实时协议

## 1. 协议约定

- REST base：`{site_origin}/api/v1`。
- AI gateway base：`{site_origin}/v1`。
- WebSocket：`wss://{site_host}/api/v1/collaboration/ws`。
- JSON 字段使用 `snake_case`，Flutter/Rust 内部类型可映射为各自惯例。
- 时间使用 RFC 3339 UTC；金额在 JSON 中使用十进制字符串，例如 `"0.100000"`。
- MVP 协同任务后台费率固定为每次 `0.10 USD`；App 不提交、不换算也不展示金额，command 的移动端 DTO 不返回账务字段。
- ID 使用 UUID v4/v7 或 Codex 返回的不透明字符串；客户端不得解析 thread/turn/item ID。
- REST 成功继续使用 Sub2API envelope：`{"code":0,"message":"success","data":...}`。
- 错误继续使用 HTTP status + `code/message/reason/metadata`；新增稳定 `reason` 作为客户端分支依据。
- 修改/提交接口必须支持 `Idempotency-Key` header；请求 body 中的同名字段仅为兼容，不得与 header 冲突。

## 2. 直接复用的现有接口

### 2.1 Panel JWT 接口

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/auth/login` | 邮箱密码登录 |
| POST | `/api/v1/auth/login/2fa` | TOTP 登录完成 |
| POST | `/api/v1/auth/refresh` | Token pair 轮换 |
| POST | `/api/v1/auth/logout` | 撤销指定 Refresh Token |
| GET | `/api/v1/auth/me` | 当前用户、角色、run mode |
| GET | `/api/v1/keys` | 当前用户 API Key 分页列表 |
| PUT | `/api/v1/keys/{id}` | 更新 Key 名称/分组；秘钥卡片分组下拉使用 |
| GET | `/api/v1/groups/available` | 当前用户可用分组 |
| POST | `/api/v1/usage/dashboard/api-keys-usage` | 批量查询秘钥卡片用量 |
| GET | `/api/v1/announcements` | 可见公告；支持 `unread_only=true` |
| POST | `/api/v1/announcements/{id}/read` | 标记已读 |
| POST | `/api/v1/redeem` | 提交兑换码 |
| GET | `/api/v1/redeem/history` | 兑换记录 |

登录成功示意：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "<jwt>",
    "refresh_token": "<opaque>",
    "expires_in": 900,
    "token_type": "Bearer",
    "user": { "id": 42, "email": "user@example.com", "role": "user" }
  }
}
```

客户端必须同时兼容登录返回 `requires_2fa=true` 的分支，不能假定每次都直接返回 token pair。

### 2.2 API Key 网关接口

以下请求使用 `Authorization: Bearer <sub2api-api-key>`，不是 JWT：

| Method | Path | 用途 |
|---|---|---|
| GET | `/v1/models` | 选中 Key 可用模型 |
| POST | `/v1/responses` | 文本/多模态 Responses |
| POST | `/v1/chat/completions` | 兼容聊天，作为 adapter 备用 |
| POST | `/v1/images/generations` | 同步生图 |
| POST | `/v1/images/edits` | 同步编辑 |
| POST | `/v1/images/generations/async` | 异步生图 |
| POST | `/v1/images/edits/async` | 异步编辑 |
| GET | `/v1/images/tasks/{task_id}` | 异步状态 |
| GET | `/v1/sub2api/billing` | Key 账务信息（可选展示） |

网关返回遵循 OpenAI 兼容格式，不使用 Panel 的 `code/data` envelope；因此移动端必须使用独立 Gateway client 和错误 parser。

## 3. 新增 REST API

统一前缀：`/api/v1/collaboration`。所有接口都要求 Panel JWT；后端只从 JWT subject 取 `user_id`。

### 3.1 设备

#### POST `/devices/register`

PC 登录后注册/更新当前安装。

```json
{
  "installation_id_hash": "sha256:...",
  "name": "AGX Workstation",
  "platform": "linux",
  "platform_version": "Ubuntu 24.04",
  "companion_version": "0.1.0",
  "codex_version": "codex-cli 0.x.y",
  "protocol_version": 1,
  "capabilities": {
    "app_server": true,
    "thread_read": true,
    "thread_write": true,
    "image_input": true
  }
}
```

响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "device_id": "5d3...",
    "heartbeat_interval_seconds": 20,
    "event_protocol_version": 1,
    "server_time": "2026-07-31T12:00:00Z"
  }
}
```

同一用户和 installation hash 应幂等更新。installation hash 不得成为 bearer credential。

#### GET `/devices`

返回当前用户未撤销设备：

```json
{
  "items": [
    {
      "id": "5d3...",
      "name": "AGX Workstation",
      "platform": "linux",
      "companion_version": "0.1.0",
      "codex_version": "codex-cli 0.x.y",
      "status": "online",
      "last_seen_at": "2026-07-31T12:00:00Z",
      "capabilities": { "thread_write": true }
    }
  ]
}
```

`status` 由心跳实时投影，数据库状态只作最后已知值。

#### PATCH `/devices/{device_id}`

只允许用户修改 `name`。

#### DELETE `/devices/{device_id}`

逻辑撤销。服务端必须关闭现有 WS、拒绝后续事件并将正在中继的 command 标记失败；已扣费用不退款。

### 3.2 会话列表同步

#### POST `/devices/{device_id}/session-syncs`

Header：`Idempotency-Key: <uuid>`

```json
{
  "search_term": "sub2api",
  "cwd": null,
  "archived": false,
  "cursor": null,
  "limit": 50
}
```

响应 HTTP 202：

```json
{
  "code": 0,
  "message": "accepted",
  "data": {
    "sync_id": "8e1...",
    "status": "pending",
    "expires_at": "2026-07-31T12:00:10Z"
  }
}
```

离线设备直接返回 409 `COLLAB_DEVICE_OFFLINE`，不要创建会长时间等待的任务。

#### GET `/session-syncs/{sync_id}`

完成示意：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "sync_id": "8e1...",
    "status": "completed",
    "device_id": "5d3...",
    "snapshot_version": 4,
    "items": [
      {
        "thread_id": "019...",
        "title": "实现 Sub2API 手机协同",
        "preview": "已完成认证模块...",
        "cwd_display": "~/works/sub2api",
        "created_at": "2026-07-30T02:00:00Z",
        "updated_at": "2026-07-31T11:58:00Z",
        "status": "not_loaded",
        "archived": false,
        "write_state": "writable_resumable",
        "write_state_reason": null
      }
    ],
    "next_cursor": null
  }
}
```

`cwd_display` 由 PC 脱敏生成，例如用 `~` 替换 home；后端和 App 不应收到 rollout `source_path`。

### 3.3 Thread 消息同步

#### POST `/devices/{device_id}/threads/{thread_id}/syncs`

```json
{
  "after_item_id": "item_abc",
  "cursor": null,
  "limit": 100,
  "include_tool_output": "summary"
}
```

响应 202，结构与 session sync 类似。

#### GET `/thread-syncs/{sync_id}`

```json
{
  "sync_id": "2a4...",
  "status": "completed",
  "thread": {
    "thread_id": "019...",
    "title": "实现 Sub2API 手机协同",
    "status": "idle",
    "write_state": "writable_loaded"
  },
  "items": [
    {
      "item_id": "item_1",
      "turn_id": "turn_1",
      "sequence": 1,
      "type": "user_message",
      "role": "user",
      "content": [{ "type": "text", "text": "检查登录模块" }],
      "status": "completed",
      "created_at": "2026-07-31T11:00:00Z"
    },
    {
      "item_id": "item_2",
      "turn_id": "turn_1",
      "sequence": 2,
      "type": "command_execution",
      "title": "cargo test",
      "summary": "exit 0 · 142 tests passed",
      "status": "completed",
      "created_at": "2026-07-31T11:00:04Z"
    },
    {
      "item_id": "item_3",
      "turn_id": "turn_1",
      "sequence": 3,
      "type": "agent_message",
      "role": "assistant",
      "content": [{ "type": "markdown", "text": "检查完成，未发现问题。" }],
      "status": "completed",
      "created_at": "2026-07-31T11:00:12Z"
    }
  ],
  "next_cursor": null
}
```

允许的规范化 item type：

- `user_message`
- `agent_message`
- `reasoning_summary`
- `command_execution`
- `file_change`
- `tool_call`
- `tool_result`
- `plan`
- `error`

未知 app-server item 映射为 `unsupported` + 安全摘要，不能把原始 JSON 无限制转发到 App。
`approval/request_user_input` 不属于可同步 item：Companion 收到后立即拒绝并把 command 映射为 `approval_required` 普通失败，不写入移动端时间线。

### 3.4 创建协同任务

#### POST `/commands`

Header：`Idempotency-Key: 2f2...`

```json
{
  "device_id": "5d3...",
  "thread_id": "019...",
  "input": [
    { "type": "text", "text": "请修复登录页重复刷新 Token 的问题，并运行相关测试。" }
  ],
  "client_context": {
    "locale": "zh-CN",
    "source": "android"
  }
}
```

MVP 只允许一个 text input，总 UTF-8 长度上限建议 32 KiB。图片/文件输入以后按 capability 开启。

成功响应 202：

```json
{
  "code": 0,
  "message": "accepted",
  "data": {
    "command_id": "be7...",
    "status": "accepted",
    "created_at": "2026-07-31T12:10:00Z"
  }
}
```

重复提交同一 Idempotency-Key 返回同一 command 和当前状态；后台不得再次扣费。

#### GET `/commands/{command_id}`

返回 command、turn 和 error 状态，不返回原始 prompt 或账务字段。

#### POST `/commands/{command_id}/cancel`

- `accepted/dispatched`：尝试取消后续中继；已扣费用不退。
- `started`：映射为 app-server `turn/interrupt`；已扣费用不退。
- terminal state：幂等返回现状。

协同账单只保留后台审计/管理员接口，移动端 MVP 不提供收费记录端点和界面。

## 4. WebSocket 协议

### 4.1 连接与认证

请求 header：

```text
Authorization: Bearer <access-jwt>
X-Sub2API-Client-Type: mobile | pc
X-Sub2API-Device-ID: <device-id>   # PC 必填，Mobile 省略
X-Sub2API-Protocol-Version: 1
```

不要使用 `?token=`。服务端升级连接后再次校验设备归属和撤销状态。

Access Token 失效：服务端以 close code `4401` 关闭；客户端通过 HTTPS 刷新 Token 后重连。设备撤销使用 `4403`。协议不兼容使用 `4406`。

### 4.2 通用 envelope

```json
{
  "v": 1,
  "type": "command.dispatched",
  "event_id": "01J...",
  "request_id": "be7...",
  "sequence": 1842,
  "occurred_at": "2026-07-31T12:10:00.123Z",
  "payload": {}
}
```

- `event_id` 全局唯一，用于去重。
- `request_id` 关联 sync_id/command_id/app-server request。
- `sequence` 是用户事件流的单调序号，不作为账务依据。
- 客户端忽略未知顶层字段与未知事件；不能因新字段断线。

### 4.3 心跳

应用层心跳用于 presence，不能只依赖 TCP ping：

```json
{ "v": 1, "type": "device.heartbeat", "event_id": "...", "payload": {
  "device_id": "5d3...",
  "app_server_status": "ready",
  "active_thread_count": 1
} }
```

建议 20 秒一次，45 秒 TTL。服务端回 `server.heartbeat_ack` 并带 server time。

### 4.4 Server → PC

| Event | 关键 payload | 说明 |
|---|---|---|
| `session.list.requested` | sync_id + filters | 请求 thread/list |
| `thread.sync.requested` | sync_id + thread_id + cursor | 请求历史增量 |
| `command.dispatched` | command_id + thread_id + input + expires_at | 下发后台已接收任务 |
| `command.cancel_requested` | command_id + optional turn_id | 取消/打断 |
| `server.shutdown` | retry_after_seconds | 优雅迁移连接 |

PC 收到 command 后，先发送 `command.received`，再进行 thread 校验。后台已经在 REST accepted 事务中完成一次扣费，WS 事件不得再次修改余额。

### 4.5 PC → Server

| Event | 关键 payload | 说明 |
|---|---|---|
| `device.hello` | 版本、capabilities | 建连声明 |
| `device.heartbeat` | 状态 | presence |
| `session.list.snapshot` | sync_id + items + cursor | 会话列表结果 |
| `thread.sync.snapshot` | sync_id + normalized items | 历史结果 |
| `command.received` | command_id | 传输 ACK |
| `command.started` | command_id + turn_id | turn 已启动 |
| `command.failed_to_start` | command_id + error_code | 标记任务失败，不退款 |
| `codex.item.event` | command_id/thread/turn/item | 流式 item |
| `codex.turn.completed` | command_id + status | terminal result |

服务端必须验证事件对应资源属于该 WS 的 device/user，且状态转换合法；错误或重放事件只记审计，不重复扣费。

### 4.6 Server → Mobile

| Event | 用途 |
|---|---|
| `device.status_changed` | 在线/离线/degraded |
| `session.list.updated` | session sync 完成，App 随后 GET 结果 |
| `thread.sync.updated` | thread sync 完成 |
| `command.status_changed` | accepted/dispatched/started/completed/failed |
| `codex.item.event` | 当前订阅 thread 的流式 UI |
| `codex.turn.completed` | turn 收尾 |
| `announcement.invalidated` | 可选，提示重新拉公告；不传完整公告 |

移动端重连后不能假定所有流式 delta 都可重放；应 GET command 状态并主动发 thread sync 补齐最终消息。

## 5. Codex app-server Adapter 契约

### 5.1 启动与握手

PC 端启动：

```text
codex app-server --stdio
```

wire 是每行一条省略 `jsonrpc` 字段的 JSON-RPC 消息。必须先：

```json
{
  "method": "initialize",
  "id": 1,
  "params": {
    "clientInfo": {
      "name": "sub2api_codex_pc",
      "title": "Sub2API Codex PC Companion",
      "version": "0.1.0"
    },
    "capabilities": { "experimentalApi": false }
  }
}
```

收到 result 后发送：

```json
{ "method": "initialized", "params": {} }
```

客户端名称上线前应评估 OpenAI 文档中对企业合规日志 client name 的要求。

### 5.2 方法映射

| Companion 用例 | app-server method |
|---|---|
| 查询会话 | `thread/list` |
| 查询单个会话元数据/历史 | `thread/read`，必要时 `includeTurns=true` |
| 大历史分页 | `thread/turns/list`、`thread/items/list`（能力检测后） |
| 加载可写会话 | `thread/resume` |
| 新建任务 | `turn/start` |
| 运行中追加输入 | `turn/steer`（MVP 不启用） |
| 取消运行 | `turn/interrupt` |
| 用户显式 fork | `thread/fork`（后续） |
| 取消订阅 | `thread/unsubscribe` |

`thread/list` 结果包含普通 CLI threads；只读操作不应抢占写锁。

### 5.3 发送算法

```text
handle_command(command):
  assert command not expired
  thread = thread/read(thread_id)
  if thread.canAcceptDirectInput == false:
      fail_to_start(THREAD_DIRECT_INPUT_BLOCKED)
  if thread not loaded by this app-server:
      result = thread/resume(thread_id)
      if JSON-RPC -32600 / owned by another process:
          fail_to_start(THREAD_BUSY_EXTERNAL)
  if an ordinary turn is already in progress:
      enqueue locally or fail TURN_IN_PROGRESS
  response = turn/start(threadId, input, clientUserMessageId=command_id)
  publish command.started(response.turn.id)
```

`command.started` 只能在 `turn/start` 返回成功之后发送；该事件只更新业务状态，不触发账务变化。

### 5.4 流式事件映射

- `turn/started` → command running。
- `item/started` → 创建/更新 item placeholder。
- `item/agentMessage/delta` → 仅对在线 App 流式；PC 同时累积最终文本。
- `item/completed` → 发送规范化完整 item，用完整值覆盖 delta buffer。
- `turn/completed` → completed/interrupted/failed；随后建议触发一次轻量 thread sync。
- `thread/status/changed` → 更新 session write/status。

所有 delta 有单项/单 turn 大小上限。超限后停止转发 delta，最终仅发送截断摘要和 `truncated=true`。

### 5.5 非交互安全策略

app-server 会向客户端发起 JSON-RPC request，例如：

- `item/commandExecution/requestApproval`
- `item/fileChange/requestApproval`
- `item/permissions/requestApproval`
- MCP elicitation / request user input

MVP 不实现审批中心或电脑确认：

1. Companion 启动 turn 时使用用户在本机预先配置的非交互安全策略，不在任务中动态放宽权限。
2. 若仍收到 approval/request_user_input，Companion 立即返回拒绝或 method not supported，并上报稳定错误 `COLLAB_APPROVAL_REQUIRED`。
3. Mobile 把该错误作为普通任务失败展示，不出现“等待电脑审批”。
4. 禁止为了消除审批界面自动启用 danger full access 或未经用户配置的额外目录/网络权限。

### 5.6 Schema 与版本兼容

- Companion 首次见到某 Codex 版本时运行 `codex app-server generate-json-schema --out <cache>` 或 `generate-ts`，并缓存版本结果。
- 启动时检查最小 method 集；缺少写方法则设备 capability `thread_write=false`。
- app-server WebSocket transport 在当前源码中标为 experimental/unsupported，本方案使用 stdio。
- 未知 notification 忽略并计数；未知 server request 必须返回 method not supported，不能悬挂。
- 如果 app-server 不可用，可降级为 cc-switch 风格 JSONL 只读浏览；协同发送按钮必须禁用。

## 6. 稳定错误码

| HTTP | reason | UI 行为 | 计费 |
|---:|---|---|---|
| 400 | `COLLAB_INVALID_SITE` | 提示站点地址 | 无 |
| 400 | `COLLAB_PROMPT_TOO_LARGE` | 显示限制 | 无 |
| 401 | `AUTH_TOKEN_EXPIRED` | 刷新后重试一次 | 无 |
| 403 | `COLLAB_DEVICE_FORBIDDEN` | 返回设备列表 | 未接收，不扣费 |
| 403 | `COLLAB_THREAD_FORBIDDEN` | 刷新列表 | 未接收，不扣费 |
| 409 | `COLLAB_DEVICE_OFFLINE` | 显示离线 | 无 |
| 409 | `COLLAB_THREAD_BUSY_EXTERNAL` | 提示稍后重试 | 预检失败，不扣费 |
| 409 | `COLLAB_TURN_IN_PROGRESS` | 排队/稍后重试 | 预检失败，不扣费 |
| 409 | `COLLAB_INSUFFICIENT_BALANCE` | 展示余额和充值入口 | 无 |
| 410 | `COLLAB_COMMAND_EXPIRED` | 可重新发送 | 已接收则不退款 |
| 422 | `COLLAB_CODEX_INCOMPATIBLE` | 提示升级 Codex/Companion | 预检失败，不扣费 |
| 422 | `COLLAB_APPROVAL_REQUIRED` | 显示当前安全策略不允许该操作 | 已接收则不退款 |
| 429 | `COLLAB_RATE_LIMITED` | 倒计时重试 | 无 |
| 502 | `COLLAB_CODEX_START_FAILED` | 显示 PC 诊断 | 已接收则不退款 |
| 503 | `COLLAB_RELAY_UNAVAILABLE` | 稍后重试 | 已接收则不退款 |

错误响应 `metadata` 可包含安全字段：`retry_after_seconds`、`device_status`、`minimum_codex_version`、`available_balance`；不得包含 prompt、token、API Key、本机绝对路径或 stderr 全文。

## 7. 限制与默认值

| 项目 | 建议默认 |
|---|---:|
| Prompt UTF-8 大小 | 32 KiB |
| session list page | 50，最大 100 |
| thread item page | 100，最大 200 |
| session sync timeout | 10 s |
| thread sync timeout | 15 s |
| command dispatch TTL | 120 s |
| heartbeat | 20 s |
| presence TTL | 45 s |
| 单用户提交频率 | 20/min（另受余额限制） |
| 单 device 同时 sync | 2 |
| 单 thread active normal turn | 1 |
| 单 item 转发正文 | 256 KiB；更大需截断/按需拉取 |

所有值都应成为后端配置并由客户端读取 capability/config，客户端只保留保护性上限。
