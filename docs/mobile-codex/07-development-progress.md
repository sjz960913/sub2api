# 开发进度

最后更新：2026-08-03

本文只记录已经落到工作树的实现和可复现证据。设计文档中的计划不等于已完成。

## 当前阶段

| 里程碑 | 状态 | 当前证据 |
|---|---|---|
| M0 协议与三端骨架 | 进行中 | 协议、后端接缝、Flutter 源码、PC Web 构建已落盘；Flutter/Go/Rust 编译等待 CI |
| M1 后端设备与账务 | 未开始 | 尚无 Ent schema、migration 或原子扣费 repository |
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
- 新增受现有 Panel JWT 中间件保护的 `GET /api/v1/collaboration/health`。
- 新增 route contract test；尚未在本机运行 Go test。

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
- `npm audit`：0 vulnerabilities。

## 环境与验证缺口

当前开发机没有 Flutter/Dart、Go、Rust/Cargo 工具链，根分区只剩约 0.7GB。为避免耗尽磁盘，没有在本机下载数 GB SDK。因此：

- Go route/config 需要 GitHub Actions 的 Go 1.26.5 job 编译和测试。
- Flutter 源码需要 CI 的 stable Flutter job 执行 pub get、gen-l10n、analyze、test。
- Tauri Rust 壳尚未执行 cargo check；PC Web 已独立构建通过。

`.github/workflows/backend-ci.yml` 已新增 protocol、PC Web 和 Flutter jobs。首次推送后必须查看所有 job 的真实结果并修复，不能把“已配置 CI”当作测试通过。

## 下一步

1. 提交 M0 当前改动并观察远程 CI，修复 Go/Flutter/协议/PC Web 的实际编译错误。
2. CI 全绿后补 Tauri Rust cargo check 环境和 OS keyring 实现。
3. 进入 M1：Ent 四表、合法状态迁移、设备 repository、command + charge + balance 原子直接扣费和并发幂等测试。
4. 后端 M1 稳定后，把 Flutter 的静态数据替换为站点、认证、Key/分组、公告与兑换真实接口。
