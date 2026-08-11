# 开发进度

最后更新：2026-08-11

本文只记录已经落到工作树的实现和可复现证据。设计文档中的计划不等于已完成。

## 当前阶段

| 里程碑 | 状态 | 当前证据 |
|---|---|---|
| M0 协议与三端骨架 | 已完成 | CI 已通过协议、Go、Flutter、PC Web 与 Tauri Rust 原生壳检查 |
| M1 后端设备与账务 | 已完成 | CI run `30792876681` 的 unit、真实 PostgreSQL integration、lint 与生成检查全部通过 |
| M2 实时中继 | 已完成 | presence、跨实例事件总线、JWT WebSocket、连接租约、短期 payload、sync/command 中继、限流、撤销断线、token 到期和 fake PC 跨实例流程均已通过 CI，并有进程内运行指标 |
| M3 PC Codex Adapter | 已完成 | app-server adapter、安全登录/设备注册、WebSocket relay、session/thread sync、command/cancel 和规范化 item 均已接入，Rust 检查与 20 项测试通过 |
| M4 Flutter 认证与目录 | 已完成 | 真实站点登录/2FA/JWT 续期、安全存储、Key/OpenAI 分组、公告、兑换与个人资料均已接入 |
| M5 聊天与图片 | 已完成 | 真实模型列表、SSE chat completions、停止生成、images generations，以及不含 API Key 的本地历史持久化/列表/删除均已接入并通过 Flutter 测试 |
| M6 移动端协同 | 进行中 | 真实设备/会话/消息同步、任务下发与手动同步已通；移动端 WebSocket 实时更新和恢复测试待完成 |
| M7 管理员 | 按需预留 | 已有角色守卫、管理员专属入口与占位页，业务界面待后续需求 |
| M8 打包发布 | 基础产物已完成 | Android debug APK、Linux Debian 和 Windows NSIS 安装包已实际产出；正式签名/macOS 公证待发布环境 |

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
- 邮箱密码登录、TOTP 2FA、JWT + Refresh Token 轮换与启动恢复已接入；远程站点强制 HTTPS。
- Access Token 仅在内存，Refresh Token 和站点使用 `flutter_secure_storage`；注销时同时清除内存中的 API Key。
- 秘钥卡片读取真实 `/keys` 和 `/groups/available`，仅呈现可用 OpenAI 分组；展示名称、脱敏 Key、用量、分组下拉和“当前聊天使用”单选。
- 秘钥目录和聊天页读取同一 Riverpod 状态；切换 Key 或分组会同步更新聊天入口，Key 明文不进入 Widget State 或持久化存储。
- 聊天已接入选中 Key 对应的 `/v1/models`、`/v1/chat/completions` 和 `/v1/images/generations`；chat completions 使用 SSE 逐段渲染并可停止，只有允许图片的分组才开启生图入口。
- 聊天历史保存在 App 私有 SQLite，按 Panel origin + user ID 隔离；历史表不含 API Key、Key ID 或 Key 名称字段，重新打开会话后仍只使用当前内存中选中的 Key 发起下一次请求。
- 历史写入前会脱敏 `sk-*`、JWT、Authorization/API Key 形态文本，并移除远程图片 URL 的 query/fragment；列表支持标题、预览、更新时间、消息数、打开和本地删除，最多保留每账号最近 100 个会话。
- 协同页已接入真实 PC 设备、session sync、thread sync 和 command 下发/状态查询；界面不展示费用、退款或 PC 审批。
- command 首次请求遇到网络错误时只用同一幂等键自动重试，避免响应丢失导致重复任务或重复扣费。
- “我的”已接入真实用户资料、兑换和公告；未读 `popup` 公告会弹窗并在关闭后标记已读，充值只打开固定地址 `https://pay.ldxp.cn/shop/codecodeai`。
- 默认角色为普通用户，`/admin` 深链会重定向到“我的”；普通用户不渲染管理员入口。
- Widget tests 已覆盖四项底栏、无费用/退款/审批文案、Key/分组联动、个人中心入口、固定充值外链和公告弹窗。
- 使用 imagegen 生成蓝白钥匙/终端启动图标并转为 Android 多密度资源；应用禁止系统备份，明文 HTTP 仅对 loopback 调试地址放行。
- release 不再回退使用 debug key，只从专用环境变量读取正式签名；CI 实际构建 debug APK 并保留 7 天产物。

### PC Companion

- 新增 Tauri 2 + React 工程与“概览、会话、实时任务、设置”四项桌面导航。
- 界面没有费用、退款、审批队列或电脑确认。
- 新增 Codex app-server stdio adapter，完成 `initialize/initialized`、CLI thread 列表、thread read/resume、turn start/interrupt；未知 server request 返回 method-not-found，审批和 elicitation 请求直接拒绝，不提供 PC 审批流程。
- app-server stderr 仅排空不记录；会话路径只保留末级标签，事件缓冲固定 256，溢出时丢弃 delta 并等待后续同步恢复。
- 使用系统原生安全凭据库保存 Refresh Token：Windows Credential Manager、macOS Keychain、Linux Secret Service；账号标签使用站点+邮箱 SHA-256，不暴露原始身份，密码不持久化。
- 新增原生 Panel 登录、TOTP 2FA、Refresh Token 轮换、进程内 Access Token、启动恢复和注销；远程站点强制 HTTPS，仅允许 loopback 使用 HTTP，HTTP redirect 被禁用，响应体上限为 1 MiB。
- React 登录页和设置页已连接原生命令；登录成功后自动启动 app-server，会话“查询”读取真实 CLI thread，不再展示静态会话。
- 登录后使用同一 Panel 身份注册 PC 设备，建立 collaboration WebSocket 并维持心跳/重连。
- session sync 使用 app-server `thread/list`，thread sync 使用 `thread/read(includeTurns)` 并支持 `after_item_id` 增量截取与最近项兜底。
- command 使用后端 command ID 作为 client message ID 启动 turn，转发规范化 item 和终态；cancel 映射到 `turn/interrupt`。
- 面向前端的 session/status DTO 不包含 access/refresh token、密码或账务字段，原生错误只返回稳定错误码。
- React/TypeScript 生产构建已通过：`vite v8.2.0`，18 modules transformed。
- 使用内置 imagegen 生成 PC Companion 图标，转换为 Tauri 所需 RGBA PNG 后，Linux 原生 `cargo check` 已通过。
- 已用 Tauri 官方工具生成 Linux PNG、Windows ICO/Appx 与 macOS ICNS 资源，CI 实际构建并保留 7 天 Debian 和 Windows NSIS 安装包。
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
- CI run `30805578423` 已通过 PC relay `cargo check` 和 20 项 Rust 测试；run `30806296466` 已通过真实 Key/OpenAI 分组移动端实现，run `30806416651` 已通过公告/兑换实现。
- CI run `30807013054` 已通过真实 OpenAI 聊天/生图与移动端 Codex 协同实现；run `30807431092` 已通过公告自动弹窗、Flutter analyze 和 Widget tests。
- CI run `30807617598` 已通过移动端 SSE 流式聊天、停止生成与跨分片 SSE 解码测试；run `30808145172` 已通过 command 幂等网络重试测试。
- CI run `30808447083` 已生成 `sub2api-codex-pc-deb` 产物（约 6.2 MB）；run `30808907588` 已生成 `sub2api-mobile-debug-apk` 产物（约 81.7 MB）并通过 Flutter analyze/test。
- CI run `30809289492` 已再次通过 Android/Linux 打包，并首次生成 `sub2api-codex-pc-windows` NSIS 产物（约 3.6 MB）。
- CI run `31467467284` 已通过新增本地历史后的 Flutter 依赖解析、l10n、analyze、全部测试和 Android debug APK 构建；同一 run 的 Tauri Rust 测试、Ubuntu Debian 包与 Windows NSIS 安装包也全部成功。三类产物已下载到指定构建服务器并通过 ZIP/DEB/PE 格式检查与 SHA-256 记录。
- 指定服务器已使用 Flutter 3.44.9 / Dart 3.12.2、Java 17 和 Android SDK 35/36 独立完成格式检查、analyze、15 项测试及 debug APK 构建；服务器 APK 已通过 ZIP 完整性检查并记录 SHA-256。
- 本机仍只执行轻量 Node 协议验证和 PC Web 构建；Go/Flutter/Rust 的权威结果以 GitHub Actions 为准。

任何 run 尚未结束时，本文件只记为“等待 CI”，不把“已配置 workflow”当作测试通过。

## 下一步

1. 为移动端协同增加 WebSocket 实时 item/终态更新、断线恢复和幂等端到端测试。
2. 在安全的发布环境配置 Android/Windows 正式签名与 macOS 公证，完成真实设备联调。
