# AI Coding 开发指南

## 1. 使用原则

这份文档用于把大型产品拆成可评审的小任务。每个 AI Coding 任务必须：

1. 先读取本目录的需求、架构和协议文档。
2. 只实现一个里程碑或一个明确子任务。
3. 先检查现有约定、相邻代码和测试模式，再编辑。
4. 不重写现有认证、公告、网关和计费逻辑；通过接口复用。
5. 任何涉及余额的改动都先写失败测试、事务测试和幂等测试。
6. 不把 token、Key、prompt、源码或本机路径写入日志、fixture 和 snapshot。
7. 交付时给出改动文件、迁移、验证命令、未覆盖风险和回滚方法。

## 2. 建议依赖

### Flutter

核心：

- `flutter_riverpod`、`riverpod_annotation`、`riverpod_generator`
- `dio`
- `go_router`
- `freezed`、`json_serializable`
- `flutter_secure_storage`
- `web_socket_channel`
- `drift`、`sqlite3_flutter_libs`
- `uuid`、`intl`、`path_provider`
- Markdown/代码高亮组件（选一个维护活跃且许可兼容的库）
- 图片：`image_picker`、`permission_handler`、平台相册保存适配

不要把 Refresh Token/API Key 放入 Drift；只保存 Secure Storage alias。

### PC Companion

Rust：

- `tokio`、`serde`、`serde_json`
- `reqwest`（HTTPS panel client）
- `tokio-tungstenite` 或 Tauri 兼容 WS client
- `keyring`、`secrecy`、`zeroize`
- `rusqlite`（本地 metadata/spool）
- `uuid`、`chrono`、`sha2`
- `tracing` + 自定义 redact layer
- `thiserror`

React：

- TypeScript、TanStack Query、Zod、react-hook-form、i18n。
- UI 可采用项目既定组件库；所有 Tauri invoke 封装在 `src/lib/api`。

### Go Backend

优先复用现有：Gin、Ent、Redis、Wire、`github.com/coder/websocket`/现有 WS 组件、`github.com/shopspring/decimal`。不要再引入第二个 decimal 或 WebSocket 库，除非现有库无法满足并经过 ADR 说明。

Go 版本以 `backend/go.mod` 为准，不以 README 中可能滞后的版本文案为准。

## 3. 代码生成与协议单一来源

新增：

```text
protocol/
├── collaboration-openapi.yaml
├── collaboration-events.schema.json
├── examples/
└── generated/
    ├── dart/
    ├── rust/
    └── go/
```

原则：

- REST DTO 以 OpenAPI 为权威。
- WS envelope/event payload 以 JSON Schema 为权威。
- Go/Dart/Rust 类型由 CI 生成或至少做 Schema compatibility test。
- 客户端可以有 domain model，但 wire DTO 不手抄三份。
- Schema 版本只增不破坏；删除字段必须跨两个大版本弃用。

Codex app-server Schema 不纳入本仓库固定真相。PC 端使用本机 `codex app-server generate-json-schema` 生成的版本做适配和 capability detection。

## 4. 里程碑计划

### M0：协议与骨架

目标：没有业务执行，但三个工程能编译，协议有可运行 mock。

交付：

- `protocol/` 的 REST 和 WS v1 Schema。
- 后端 collaboration route group、feature flag 和 health 占位。
- Flutter Feature-First 骨架、主题、l10n、路由壳。
- PC Tauri/React 骨架、OS Keyring facade、脱敏日志。
- fake relay server 与 fake app-server JSONL fixture。

验证：

- Schema 示例全部通过 validation。
- Dart/Rust/Go DTO round-trip golden test。
- 普通 user 深链不能进入 `/admin`。

不做：数据库迁移、真实扣费、真实 Codex。

### M1：后端设备与账务领域

目标：完成 PostgreSQL 权威状态机，暂不接 WebSocket。

交付：

- Ent schemas/migrations：devices、sync requests、commands、charges。
- Domain enums 和合法 transition 表。
- Repository 事务：register/revoke device、reserve、settle、refund。
- `shopspring/decimal` 或明确最小单位策略。
- 配置：feature enabled、fee、currency、TTL、rate limit。
- 后台 sweeper：超时 held 自动退款。

关键测试：

- 相同 `(user_id,idempotency_key)` 并发 50 次只冻结一次。
- settle 事件重放不重复减少 frozen balance。
- refund 事件重放不重复增加 balance。
- reserve 与用户余额更新是原子事务。
- 余额不足不产生 command/charge。
- 用户删除/设备撤销/服务重启后的状态可收敛。
- 用户 A 不能读取用户 B 的 UUID 资源。

完成定义：所有 repository integration test 使用真实 PostgreSQL 或现有项目约定的 test container 通过。

### M2：实时中继

目标：PC 和 Mobile mock 能跨多个后端实例收发事件。

交付：

- JWT WebSocket upgrade、client type/device header 校验。
- presence heartbeat、Redis connection routing、用户事件 stream。
- REST session/thread sync request 与 Redis payload cache。
- command dispatch、received/accepted/started/failed 事件。
- 断连处理、背压、payload/速率限制、metrics。

关键测试：

- PC 连接到实例 A、Mobile 连接到实例 B 仍可中继。
- token 过期 4401，刷新重连后状态恢复。
- 设备撤销立即断线，未 started command 退款。
- 慢消费者不拖垮 hub；溢出时可重建状态。
- Redis 发布失败不会留下永远 held 的余额。

完成定义：使用 fake PC 完成 session sync 和 command started/settled 的端到端后端测试。

### M3：PC Codex Adapter

目标：PC 可列、读、恢复和写入本机 Codex thread。

交付顺序：

1. Binary discovery + `codex --version`。
2. 子进程 supervisor 和 stderr 安全日志。
3. initialize/initialized handshake。
4. request ID multiplexer 与 timeout/cancel。
5. thread/list + thread/read。
6. thread/resume + turn/start。
7. item/turn notification mapper。
8. approval request center。
9. Schema/version capability cache。
10. app-server 不可用时的 JSONL 只读降级。

必须模拟：

- JSON-RPC response 乱序。
- notification 穿插 response。
- 一行部分读取、超大行、无效 JSON、stderr 噪声。
- app-server 中途退出和重启。
- thread `-32600` 外部占用。
- `canAcceptDirectInput=false`。
- turn/start 成功后立即断线，仍能凭 turn_id 对账。
- 审批 request 未响应时不阻塞整个 reader loop。

完成定义：真实本机 Codex smoke test 能创建临时 thread、列出、读回、启动一轮并收到 completed；测试不得改动用户生产 thread。

### M4：Flutter 基础、认证与目录

目标：完成可安全登录的 Android 基础 App。

交付：

- Site URL 解析/探测与 HTTPS 策略。
- 登录、TOTP、恢复会话、登出。
- Dio panel client 的单飞 refresh 和 token rotation。
- Secure Storage key namespace。
- API Key/分组目录、Key 详情脱敏、可用性校验。
- 公告列表、未读角标、popup 队列。
- user/admin 路由分支和 admin placeholder。

关键测试：

- 20 个并发 401 只发起一次 refresh。
- refresh 轮换后旧 token 不再存储。
- 登录失败/站点切换不串用旧 token。
- redirect 不携带 Authorization 到其他 host。
- inactive/expired/non-openai Key 不可进入聊天。
- popup 公告在 mark-read 重试期间不重复轰炸。

完成定义：Android debug build 在真实 Sub2API 测试站点完成登录、Key 列表和公告。

### M5：普通聊天与图片

目标：交付不依赖 PC 的完整用户价值。

交付：

- Gateway Dio client 与 API Key credential provider。
- `/v1/models` server-driven 模型目录。
- Responses SSE parser、停止、重试、Markdown/代码渲染。
- 本地 conversation/message 数据库。
- 异步 image generation/edit、轮询、结果保存和相册导出。
- 分组图片权限与错误提示。

关键测试：

- SSE chunk 任意切分、多字节 UTF-8、错误 event、网络中断。
- 停止生成会释放连接并收敛 message 状态。
- API Key 不进入数据库、日志或 crash breadcrumb。
- image task 在 App 进程重启后能恢复轮询。
- base64/URL 图片响应、超大图片和无图 200 均安全处理。

完成定义：选中 OpenAI Key 完成一轮流式聊天和一张异步图片生成。

### M6：移动端 Codex 协同

目标：端到端满足会话查询、同步、发送和计费。

交付：

- Device list + online status。
- Session query/search/filter/list。
- Thread item timeline 和增量 sync。
- Command compose、二次价格确认、状态轨迹、退款反馈。
- 当前 thread 的实时 item/turn event 消费。
- PC approval 等待状态。
- 断线重连后 command GET + thread sync 收敛。

关键测试：

- App 连点发送/HTTP retry 只创建一次 command。
- PC 离线、外部占用、turn 正忙、余额不足的 UI 和账务正确。
- Mobile 掉线期间 turn 完成，重连后能补齐最终消息。
- 两台 PC 有相同 thread ID 时仍按 device 隔离。
- 工具输出折叠且有长度上限。

完成定义：真实 Android + PC + 测试站点完成一次收费任务，账单与余额/frozen balance 对账一致。

### M7：管理员资源预留与运维

目标：为后续管理功能留稳定边界，不在 MVP 伪造业务。

交付：

- AdminShell、导航槽、权限 guard、占位页、Design Token、i18n keys。
- 后端 admin collaboration route group 空壳，仅 health/summary feature flag；普通用户 403。
- Metrics dashboard 定义、日志字段和告警阈值。
- 费率配置的后端设置项；如暂不提供 UI，则只允许配置文件/受审计 admin API。

完成定义：角色和路由测试通过，普通用户不可访问；占位资源将来可替换而不改主导航架构。

### M8：安全、灰度与发布

按 [测试、安全与发布清单](06-test-security-release.md) 完成威胁模型、渗透场景、负载、崩溃恢复、安装包签名和灰度。

## 5. 推荐 Flutter 目录细化

```text
clients/mobile/lib/
├── app/
│   ├── app.dart
│   ├── bootstrap.dart
│   ├── router.dart
│   └── theme/
├── core/
│   ├── auth/
│   ├── config/
│   ├── errors/
│   ├── logging/
│   ├── network/
│   │   ├── panel_dio.dart
│   │   ├── gateway_dio.dart
│   │   ├── token_refresh_coordinator.dart
│   │   └── sse/
│   ├── realtime/
│   ├── storage/
│   └── widgets/
└── features/<feature>/
    ├── data/
    │   ├── datasources/
    │   ├── dto/
    │   ├── mappers/
    │   └── repositories/
    ├── domain/
    │   ├── entities/
    │   ├── repositories/
    │   └── usecases/
    └── presentation/
        ├── controllers/
        ├── pages/
        └── widgets/
```

规则：

- Feature 之间不互相 import `data/` 或 `presentation/`；通过 domain/public providers 协作。
- DTO 不直接进 UI；必须 mapper 到 domain entity。
- Riverpod provider 放在其拥有状态的 feature，不建一个全局巨型 providers 文件。
- 页面处理展示意图，Use Case 处理业务决定。

## 6. 推荐 PC 目录细化

```text
clients/codex-pc/
├── src/
│   ├── app/
│   ├── features/auth/
│   ├── features/device/
│   ├── features/sessions/
│   ├── features/live_thread/
│   ├── features/approvals/
│   ├── features/settings/
│   ├── lib/api/
│   ├── lib/query/
│   └── types/
└── src-tauri/src/
    ├── commands/
    ├── services/
    │   ├── auth_service.rs
    │   ├── device_service.rs
    │   ├── session_service.rs
    │   └── approval_service.rs
    ├── adapters/
    │   └── codex_app_server/
    │       ├── process.rs
    │       ├── codec.rs
    │       ├── client.rs
    │       ├── capabilities.rs
    │       ├── mapper.rs
    │       └── error.rs
    ├── realtime/
    ├── repositories/
    ├── secure_store/
    ├── redaction/
    └── state.rs
```

Rust service 层不依赖 Tauri Window；这样可在无 GUI 测试中运行 fake relay 和 fake app-server。

## 7. Backend 实现顺序

建议一次 PR 不超过一个可评审阶段：

1. Domain types + transition unit tests。
2. Ent schema/migration + repository integration tests。
3. Device REST。
4. Billing reserve/settle/refund REST-internal API。
5. WebSocket auth/presence。
6. Sync request relay。
7. Command relay + sweeper。
8. Metrics/admin guard。

修改 Ent schema 后遵循仓库命令：

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

后端验证至少包括：

```bash
cd backend
go test ./internal/domain/... ./internal/service/... ./internal/repository/... ./internal/handler/...
go test ./internal/server/...
```

具体包名在实现后按实际 collaboration 目录收窄；不要为了一个小改动默认跑不相关的外部集成测试。

## 8. 配置建议

原需求中的“0.1 元”已确认指 `0.10 USD`，不是 CNY。以下金额是 MVP 协同任务的固定服务端费率；UI 必须显示 `$0.10 USD`，普通聊天和图片生成仍使用各自的站点渠道费率。

```yaml
collaboration:
  enabled: false
  protocol_version: 1
  task_fee:
    amount: "0.100000"
    currency: "USD"
  presence:
    heartbeat_seconds: 20
    ttl_seconds: 45
  sync:
    session_timeout_seconds: 10
    thread_timeout_seconds: 15
    snapshot_ttl_seconds: 600
  command:
    dispatch_ttl_seconds: 120
    max_prompt_bytes: 32768
    max_per_user_per_minute: 20
  content_persistence:
    postgres_enabled: false
```

环境变量使用项目既有命名映射。费率变更只影响新建 command；已 held command 保存自己的 amount/currency 快照。

## 9. AI Coding 任务模板

复制后只替换方括号：

```text
目标：实现 docs/mobile-codex/04-ai-coding-guide.md 的 [M1 子任务：reserve/settle/refund repository]。

开始前必须阅读：
- docs/mobile-codex/01-requirements-feasibility.md
- docs/mobile-codex/02-system-architecture.md 的计费事务设计
- docs/mobile-codex/03-api-realtime-protocol.md 的 command/charge 契约
- 相邻 repository/service 和测试约定

范围：
- [明确允许修改的目录]
- 不实现 Handler/UI/WebSocket
- 不改现有普通 API usage billing 行为

验收：
- [列出并发幂等、失败回滚、重放测试]
- 余额、frozen_balance、command、charge 在事务后完全一致
- 日志不包含 secret/prompt

交付：
1. 实现和测试
2. 运行最小相关验证
3. 报告文件、迁移、测试结果、风险和后续接口
```

## 10. Code Review 清单

### 所有改动

- 是否把协议错误变成稳定 reason，而不是匹配 message 文本？
- 是否有 bounded size、timeout、cancel 和 backpressure？
- 是否正确处理重复、乱序、断线和进程重启？
- 是否泄露 secret、prompt、本机绝对路径或命令输出？
- 是否新增了跨 feature 的反向依赖？
- 是否有用户隔离测试和失败路径测试？

### 计费

- 金额是否由服务端决定并使用 decimal/NUMERIC？
- reserve/settle/refund 是否 CAS 且幂等？
- Redis/WS 失败是否最终退款？
- turn/start 之前是否绝不 settle？
- started 后最终 Codex 失败是否保持 settled？

### Codex Adapter

- 是否只读 stdout JSONL、stderr 单独记录？
- 是否先 initialize？
- 是否处理 server-initiated request？
- 是否对 active external thread 返回 busy，而不是修改 JSONL？
- 是否根据实际 Codex 版本做 capability detection？

### Flutter

- Panel JWT 与 Gateway API Key 是否使用不同 client？
- Refresh 是否单飞且支持 token rotation？
- Key 是否只在 Secure Storage？
- Widget 是否绕过 repository/use case 直接联网？
- App 恢复/重连后状态是否能从 REST 收敛？

## 11. 不允许的捷径

- 用 `Process.run("codex resume ... < prompt")` 假装结构化协同。
- 直接修改 `~/.codex/sessions/*.jsonl` 追加用户消息。
- 把 PC 开一个公网 HTTP 端口让手机直连。
- 在 WebSocket query string 放 JWT。
- 只在客户端扣费或传入 `fee=0.1`。
- 以 `float64` 直接做余额精确相等判断。
- 在 Mobile 数据库保存全部 API Key/Refresh Token。
- 把 `approvalPolicy` 改成 never/danger full access 来避免审批 UI。
- 为了选择分组，静默修改用户现有 API Key 的 group。
