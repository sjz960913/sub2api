# 开发进度

最后更新：2026-08-03

本文只记录已经落到工作树的实现和可复现证据。设计文档中的计划不等于已完成。

## 当前阶段

| 里程碑 | 状态 | 当前证据 |
|---|---|---|
| M0 协议与三端骨架 | 已完成 | CI 已通过协议、Go、Flutter、PC Web 与 Tauri Rust 原生壳检查 |
| M1 后端设备与账务 | 进行中 | 四表迁移、状态机、设备 REST、原子直接扣费、缓存失效与过期清理已落盘；等待最新 PostgreSQL 集成 CI |
| M2 实时中继 | 未开始 | 只有无依赖 fake relay；尚无生产 WS/Redis presence |
| M3 PC Codex Adapter | 未开始 | 只有 Tauri/React 壳、SecretStore 边界和脱敏函数 |
| M4 Flutter 认证与目录 | 部分提前实现 | 四项导航、页面视觉、普通用户 admin 深链守卫和固定充值外链已写；真实 API/安全存储尚未接入 |
| M5 聊天与图片 | 未开始 | 当前聊天页只有界面数据 |
| M6 移动端协同 | 未开始 | 当前协同页只有界面数据 |
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

### Flutter Android

- 新增 `clients/mobile` Feature-First 工程源码、Material 3 主题、中文/英文 l10n 输入和 GoRouter 壳。
- 底部导航严格为“聊天、协同、秘钥、我的”。
- 已实现 UI 参考对应的聊天、协同、秘钥卡片和我的页面。
- 秘钥卡片区分分组下拉与“当前聊天使用”单选。
- “我的”包含兑换 Dialog、公告 Bottom Sheet、固定外部充值地址与管理员占位入口。
- 默认角色为普通用户，`/admin` 深链会重定向到“我的”；普通用户不渲染管理员入口。
- 已写 Widget tests，尚未在本机运行 Flutter analyze/test。

### PC Companion

- 新增 Tauri 2 + React 工程与“概览、会话、实时任务、设置”四项桌面导航。
- 界面没有费用、退款、审批队列或电脑确认。
- Rust 端预留 SecretStore facade、字段级脱敏和最小 Tauri command。
- React/TypeScript 生产构建已通过：`vite v8.2.0`，16 modules transformed。
- 使用内置 imagegen 生成 PC Companion 图标，转换为 Tauri 所需 RGBA PNG 后，Linux 原生 `cargo check` 已通过。
- `npm audit`：0 vulnerabilities。

## CI 证据与验证缺口

当前开发机没有 Flutter/Dart、Go、Rust/Cargo 工具链，根分区只剩约 0.7GB。为避免耗尽磁盘，没有在本机下载数 GB SDK。因此：

- CI run `30792095918` 已通过 protocol、Go lint、Flutter analyze/test、PC Web 和 Tauri Rust；该 run 的 test job 只因 integration build 使用了当前 decimal 版本不存在的测试常量而失败，生产代码和 unit suite 已通过。
- 测试常量已改为显式 `decimal.NewFromInt(1)`；包含 M1 新增设备 REST、sweeper 和 PostgreSQL 集成场景的后续 run 正在验证。
- 本机仍只执行轻量 Node 协议验证和 PC Web 构建；Go/Flutter/Rust 的权威结果以 GitHub Actions 为准。

任何 run 尚未结束时，本文件只记为“等待 CI”，不把“已配置 workflow”当作测试通过。

## 下一步

1. 等待最新 CI 完成 M1 unit/integration、Wire 可复现性和 Tauri 回归验证；修复任何真实失败。
2. CI 全绿后进入 M2：JWT WebSocket、Redis presence、跨实例中继、sync payload TTL 与命令 dispatch。
3. M2 稳定后实现 M3 Codex app-server adapter 和系统 keyring，再把 Flutter 静态数据替换为真实站点认证与业务接口。
