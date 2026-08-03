# 开发进度

最后更新：2026-08-03

本文只记录已经落到工作树的实现和可复现证据。设计文档中的计划不等于已完成。

## 当前阶段

| 里程碑 | 状态 | 当前证据 |
|---|---|---|
| M0 协议与三端骨架 | 已完成 | CI 已通过协议、Go、Flutter、PC Web 与 Tauri Rust 原生壳检查 |
| M1 后端设备与账务 | 已完成 | CI run `30792876681` 的 unit、真实 PostgreSQL integration、lint 与生成检查全部通过 |
| M2 实时中继 | 已完成 | presence、跨实例事件总线、JWT WebSocket、连接租约、短期 payload、sync/command 中继、限流、撤销断线、token 到期和 fake PC 跨实例流程均已通过 CI，并有进程内运行指标 |
| M3 PC Codex Adapter | 进行中 | app-server stdio adapter、原生安全凭据库、Panel 登录/2FA/刷新恢复和真实会话查询已实现；后端 WebSocket relay 与完整 item 规范化仍待接入 |
| M4 Flutter 认证与目录 | 部分提前实现 | 四项导航、共享 Key/分组选择、个人中心交互、admin 深链守卫和固定充值外链已写；真实 API/安全存储尚未接入 |
| M5 聊天与图片 | 部分提前实现 | 聊天详情、共享当前 Key、消息输入和生图参数页已写；网关、流式响应与本地持久化尚未接入 |
| M6 移动端协同 | 部分提前实现 | 极简查询、会话列表/详情、手动同步和任务输入已写；当前仍为界面数据，尚未接实时 API |
| M7–M8 管理员/发布 | 未开始 | 只有管理员占位路由和 CI 初始任务 |

## 已实现

### 协议

- OpenAPI 3.1 覆盖 health、device、session sync、thread sync 和 command 接口。
- JSON Schema 固定 WebSocket v1 envelope 和允许事件。
- 脱敏 fixtures 不包含真实 Token、API Key、prompt 或本机绝对路径。
- 单一生成器输出 Go、Dart、Rust DTO，并复制到 Flutter/PC 消费目录。
- fake relay 支持带 Panel JWT 占位凭据的 REST health、command accepted 和 WebSocket upgrade/首帧。

验证：

```text
node protocol/scripts/generate.mjs
  generated 5 DTO files
node protocol/scripts/validate.mjs
  validated 4 collaboration fixtures
node protocol/scripts/mock-smoke.mjs
  fake relay REST and WebSocket smoke test passed
```

### Go 后端

- 新增默认关闭的 `collaboration` 配置域，包括协议版本、心跳、后台费率、TTL 和限制。
- 新增受现有 Panel JWT 中间件保护的 health 与设备注册、列表、改名、撤销 REST；设备接口在 feature flag 关闭时返回稳定错误。
- 新增 devices、sync requests、commands、charges 四张 Ent schema 和 SQL migration；长期数据不保存 prompt 正文。
- 新增设备、同步和任务状态机及合法迁移单测；`revoked` 和终态不可逆。
- 新增 `command + charge + users.balance` 单事务直接扣费仓储。费率只取服务端配置，使用精确 decimal，`frozen_balance` 不参与，且不存在 hold、refund 状态。
- 相同 `(user_id, idempotency_key)` 的并发请求复用同一 command/charge；变更后的请求体会返回幂等冲突。
- 提交成功后失效余额与 API Key 鉴权快照；后台 sweeper 只把超时任务/同步改为 `expired`，不会退款。
- Wire 生成图已纳入可复现 workflow，生产路由、服务、仓储、清理器和关闭流程均已接线。
- 新增 Redis Pub/Sub 跨实例事件总线和按用户递增序列；WebSocket 使用现有 Panel JWT、协议/客户端/设备头校验、单 writer、事件大小限制、慢消费者关闭和安全 Origin 策略。
- PC 心跳刷新 Redis TTL 与数据库投影，并向同用户手机广播在线状态；Redis Lua 连接租约原子限制单用户和单设备连接数。
- session/thread sync REST 会在设备在线且 capability 匹配时创建带请求哈希的幂等记录，通过 Redis 中继到 PC，并把规范化结果短期保存到用户隔离的 Redis payload key；数据库只保存状态和计数。
- command REST 在后端完成原子直接扣费后保存短期 prompt、派发到目标 PC，并接收 `received/started/item/completed/failed`；状态更新使用 user/device scope 与旧状态条件，冲突重放不会覆盖终态。
- command cancel 会向目标 PC 发送 `command.cancel_requested`；终态重复取消保持幂等，不新增扣费、不修改账务记录。
- 移动端 command DTO 不返回 fee、currency、charge 或余额字段；同步/流式 payload 会拒绝 token、API Key、账务字段、原始路径和未规范化 item。
- Redis sliding-window limiter 已接入 command 提交；设备撤销会跨实例发布 `server.shutdown(device_revoked)` 并以 4403 断开 PC，Access Token 到期会以 4401 主动关闭。
- fake PC 集成测试覆盖跨后端实例注册、上线、session sync、command 派发与状态回传；进程内指标覆盖 WS、presence、sync、command、charge、限流和 relay failure。

### Flutter Android

- 新增 `clients/mobile` Feature-First 工程源码、Material 3 主题、中文/英文 l10n 输入和 GoRouter 壳。
- 底部导航严格为“聊天、协同、秘钥、我的”。
- 已实现 UI 参考对应的聊天、协同、秘钥卡片和我的页面。
- 秘钥卡片区分分组下拉与“当前聊天使用”单选。
- 秘钥目录和聊天页改为读取同一 Riverpod 状态；切换 Key 或分组会同步更新聊天入口，不在 Widget State 中保存 Key 明文。
- 新增普通聊天详情、消息输入、图片参数 Bottom Sheet，以及协同查询、搜索、会话详情、手动同步和任务输入界面。
- “我的”包含兑换 Dialog、公告 Bottom Sheet、固定外部充值地址与管理员占位入口。
- 公告列表支持全部/未读筛选；“我的”按参考图改为资料卡和分组菜单，充值仍只调用固定外部 HTTPS 地址。
- 默认角色为普通用户，`/admin` 深链会重定向到“我的”；普通用户不渲染管理员入口。
- Widget tests 已覆盖四项底栏、无费用/退款/审批文案、Key/分组联动、个人中心入口与固定充值外链；Flutter analyze/test 已在 CI 通过。

### PC Companion

- 新增 Tauri 2 + React 工程与“概览、会话、实时任务、设置”四项桌面导航。
- 界面没有费用、退款、审批队列或电脑确认。
- 新增 Codex app-server stdio adapter，完成 `initialize/initialized`、CLI thread 列表、thread read/resume、turn start/interrupt；未知 server request 返回 method-not-found，审批和 elicitation 请求直接拒绝，不提供 PC 审批流程。
- app-server stderr 仅排空不记录；会话路径只保留末级标签，事件缓冲固定 256，溢出时丢弃 delta 并等待后续同步恢复。
- 使用系统原生安全凭据库保存 Refresh Token：Windows Credential Manager、macOS Keychain、Linux Secret Service；账号标签使用站点+邮箱 SHA-256，不暴露原始身份，密码不持久化。
- 新增原生 Panel 登录、TOTP 2FA、Refresh Token 轮换、进程内 Access Token、启动恢复和注销；远程站点强制 HTTPS，仅允许 loopback 使用 HTTP，HTTP redirect 被禁用，响应体上限为 1 MiB。
- React 登录页和设置页已连接原生命令；登录成功后自动启动 app-server，会话“查询”读取真实 CLI thread，不再展示静态会话。
- 面向前端的 session/status DTO 不包含 access/refresh token、密码或账务字段，原生错误只返回稳定错误码。
- React/TypeScript 生产构建已通过：`vite v8.2.0`，18 modules transformed。
- 使用内置 imagegen 生成 PC Companion 图标，转换为 Tauri 所需 RGBA PNG 后，Linux 原生 `cargo check` 已通过。
- `npm audit`：0 vulnerabilities。

## CI 证据与验证缺口

当前开发机没有 Flutter/Dart、Go、Rust/Cargo 工具链，根分区只剩约 0.7GB。为避免耗尽磁盘，没有在本机下载数 GB SDK。因此：

- CI run `30792876681` 已通过 protocol、Go unit/integration/lint、Flutter analyze/test、PC Web 和 Tauri Rust。真实 PostgreSQL 场景覆盖 50 并发幂等、余额不足回滚、跨用户隔离、撤销设备、过期收敛且不退款。
- CI run `30793535264` 已通过 presence、Wire 可复现检查和全部三端任务，并产出已提交的 Cargo/Flutter lockfile。
- CI run `30795527650` 已通过重设计后的 Flutter 四栏界面、交互 Widget tests 和全部三端任务。
- CI run `30798309619` 已通过最新 protocol、Go unit、真实 PostgreSQL integration、golangci-lint、Flutter、PC Web 与 Tauri Rust；覆盖 sync 幂等/CAS/租户隔离、command 状态回传和移动端账务字段隔离。Wire 生成 run `30798165025` 也已通过并提交产物。
- CI run `30799267889` 已通过 Redis command sliding-window limiter；run `30800107425` 已通过设备撤销跨实例主动断线；run `30800794592` 已通过 fake PC 跨实例完整协同流程；run `30801045629` 已通过 JWT 到期主动断线。
- CI run `30802124650` 已通过协同运行指标，run `30802152905` 已通过 Codex app-server adapter 和 Rust 单测；run `30802317722` 已通过有界事件缓冲，run `30802567107` 已通过 command cancel 中继。
- CI run `30802921268` 已通过原生 keyring 三平台依赖的 Linux Rust 构建与单测；run `30803405542` 已完整通过 Panel 登录、2FA、refresh/恢复与安全 DTO 实现。
- 本机仍只执行轻量 Node 协议验证和 PC Web 构建；Go/Flutter/Rust 的权威结果以 GitHub Actions 为准。

任何 run 尚未结束时，本文件只记为“等待 CI”，不把“已配置 workflow”当作测试通过。

## 下一步

1. 完成 M3：PC 注册设备并连接 collaboration WebSocket，把 session/thread sync、command、cancel 与 app-server 双向映射。
2. 把 Flutter 静态数据替换为真实站点认证、Key/分组、公告、兑换、聊天、图片与协同接口。
3. 增加 PC 安装级身份、断线重连、token 到期刷新和 relay 恢复测试，再进入 Android 端到端联调。
