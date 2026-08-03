# 测试、安全与发布清单

## 1. 测试策略

采用测试金字塔：领域/协议单元测试最多，Repository/Adapter 集成测试覆盖边界，少量真实 Android + PC + Sub2API + Codex 端到端测试覆盖关键链路。

任何测试不得使用真实生产 token、API Key、用户目录或会话。测试日志进入 CI artifact 前再跑 secret scanner。

## 2. 后端测试矩阵

### 2.1 Domain 与 Repository

- device register 对同一 user + installation hash 幂等。
- 同 installation 重新登录不同 user 不继承设备。
- 所有 device/sync/command/charge 查询都带 user scope。
- command 状态只允许文档定义的转换；非法/乱序转换不改变数据库。
- 50 个并发相同 Idempotency-Key 只产生一个 command、一个 charged 记录和一次余额扣减。
- 不同用户可使用相同 Idempotency-Key，互不影响。
- accepted 同事务更新 balance、command、charge。
- 余额等于 fee 时允许 accepted，结果 balance=0。
- 余额小于 fee 时完全不写 command/charge。
- command/charge 重放和并发竞争均最多扣费一次。
- accepted 后的 failed/interrupted 保持 charged，不退款。
- dispatch timeout 只标记失败，不修改已扣余额。
- 进程在 accepted 提交后、Redis publish 前崩溃，恢复任务只更新失败状态且不重复扣费。
- 设备撤销和用户禁用会关闭/拒绝新任务；已 accepted 任务不退款。
- Billing cache/auth cache 在余额变化后按现有机制失效。

### 2.2 REST

- JWT 缺失/过期/错误角色。
- UUID 格式、分页边界、prompt byte 上限、未知字段。
- header/body Idempotency-Key 冲突返回 400。
- 标准 Sub2API response envelope 与稳定 reason。
- 响应 metadata 不含 secret 或本机路径。
- user A 猜中 user B 的 device/sync/command UUID 仍返回安全 404/403。
- rate limit 和 Retry-After。

### 2.3 WebSocket

- 无 Authorization、query token、过期 token、错误 client type。
- PC 的 device ID 不属于当前 user。
- revoked device 被 4403 关闭。
- heartbeat TTL、移动网络短断、指数重连。
- PC 和 Mobile 连接到不同后端实例，通过 Redis 正确路由。
- 重复 event_id、旧 sequence、乱序 status、伪造 user/device 字段。
- 慢消费者队列满时不阻塞 hub；客户端可用 REST 重建。
- 超大 frame、压缩炸弹、无效 UTF-8/JSON、未知 event version。
- Redis pub/sub/stream 故障、重连和恢复。
- 服务端滚动升级的 server.shutdown/reconnect。

## 3. PC Companion 测试矩阵

### 3.1 app-server Codec

使用可编程 fake child process 覆盖：

- stdout 每次只返回半行、多行一次返回、CRLF。
- response 与 notification 任意穿插、response 乱序。
- server-initiated request 与普通 response 使用相同 reader loop。
- stderr 有普通日志、超长行、疑似 secret；均安全截断/脱敏。
- stdout 非 JSON、缺少 id/method、未知 notification。
- request timeout 后迟到 response 不污染其他 request。
- process exit、restart、backoff、shutdown 时清理 pending oneshot。
- bounded line/frame，拒绝无限内存增长。

### 3.2 Codex 行为

- initialize 前调用被禁止；重复 initialize 处理清楚。
- thread/list 分页和 filters。
- thread/read(includeTurns) 映射所有已支持 item。
- 普通 CLI thread 可列出；subagent/只读状态正确。
- thread/resume 外部占用 `-32600` 映射为 `THREAD_BUSY_EXTERNAL`。
- canAcceptDirectInput=false 禁止 turn/start。
- turn 已运行时按 MVP 排队/拒绝，不误用 steer。
- turn/start 返回 turn ID 后才上报 started。
- item delta 累积，item completed 覆盖最终值。
- turn completed 后执行最终轻量 sync。
- command/file/permission approval request 立即拒绝/unsupported，并映射为 `approval_required` 普通失败；没有审批界面。
- app-server 版本缺 method 时 capability 降级，不崩溃。
- app-server 不可用时只读 JSONL，不显示发送能力。

### 3.3 桌面安全

- Password/Refresh Token 只进 OS Keyring。
- 复制脱敏诊断不含 token、prompt、路径用户名。
- Deep Link/IPC 参数不能触发任意进程或 shell。
- React 层不能直接调用通用 `execute(command)` Tauri command。
- 自动更新包签名校验失败时拒绝安装。
- 设备撤销/退出登录后清 Keyring、关闭 WS、停止 app-server 子进程。

## 4. Flutter 测试矩阵

### 4.1 认证与网络

- Site URL 大小写、尾斜杠、端口、IDN、userinfo、path、fragment。
- release 拒绝 HTTP 和无效证书；debug 开关只允许明确 host。
- 20 个并发 401 只触发一次 refresh，其余请求排队重放。
- refresh 失败统一登出；不无限刷新自身 `/auth/refresh`。
- Refresh Token 轮换原子写入；App 在写入中被杀死仍保留一套有效 pair 或重新登录。
- HTTP redirect 到不同 origin 时剥离 Authorization。
- Panel JWT client 与 Gateway API Key client 不串 header。
- release 日志无 request/response body。

### 4.2 Key、聊天与图片

- inactive、expired、无 group、非 OpenAI group Key 不可选。
- 秘钥卡片分组下拉调用 `PUT /api/v1/keys/{id}`；成功刷新卡片，失败不污染本地状态。
- 当前聊天 Key 是单选偏好；聊天和生图请求都使用该 Key，失效时清除选择。
- Key 明文只在 Secure Storage，界面/无障碍 label 均脱敏。
- SSE 任意 chunk、UTF-8 边界、JSON event、错误、EOF、取消。
- App 被杀死/恢复后本地 message 状态收敛。
- 变更 model/key 时创建新 conversation 或明确确认。
- 图片任务进程重启恢复轮询；URL/base64、无图成功、超大图片、权限拒绝。
- 导出相册前请求平台权限，拒绝后仍保留 App 私有文件。

### 4.3 公告

- silent 不弹窗，popup 未读弹窗。
- 同一进程同一 popup 只显示一次。
- mark-read 失败后台重试，不连续弹窗。
- 超长 Markdown、链接、代码、恶意 HTML 安全渲染。
- targeting 由服务端决定，客户端不自行判断资格。
- 公告入口只从“我的”打开，未读 badge 与列表一致。

### 4.4 我的与主导航

- 底部菜单始终只有“聊天、协同、秘钥、我的”，顺序和选中状态正确。
- 兑换 Dialog 校验空值、提交中、成功刷新余额、失败重试。
- 充值使用系统外部浏览器打开且仅打开 `https://pay.ldxp.cn/shop/codecodeai`。
- 充值 Intent 不携带 JWT、Refresh Token、API Key 或其他 query 参数；无法打开时显示错误 Toast。

### 4.5 协同

- 多 PC 同 thread ID 使用 device ID 组合主键。
- session 查询时 PC 离线/中途离线/超时。
- thread items 以稳定 item ID 去重，delta 与 completed 收敛。
- 重连错过事件后通过 GET command + sync 补齐。
- 连点发送与 Dio retry 使用同一 Idempotency-Key。
- App 不显示 fee、charge、held、settled、refunded 或二次确认。
- 外部占用/只读状态禁用输入。
- `approval_required` 显示为普通任务失败，不出现等待电脑确认。

## 5. 端到端场景

每次候选发布至少通过：

1. Android 配置测试站点 → 登录 → token 自动刷新。
2. 选择 OpenAI Key → `/v1/models` → 流式聊天完成。
3. 提交异步图片 → App 重启 → 恢复轮询 → 保存图片。
4. 发布 popup 公告 → App 拉取 → mark read → 不再弹。
5. PC 登录同一用户 → App 显示 online。
6. 查询 100+ Codex threads → 搜索 → 读取大 thread 分页历史。
7. 向空闲 thread 发送 → accepted/后台直接扣费 → started → completed → 消息补齐。
8. 相同 Idempotency-Key 重放 10 次，余额只变化一次。
9. PC 在 accepted 后、dispatch 前退出，命令失败且不退款、不重复扣费。
10. thread 在预检时发现被另一个 Codex 占用，返回 busy 且不创建 charge。
11. turn started 后执行失败，charged 记录不变，App 只显示业务失败。
12. 命令触发 approval request，Companion 自动拒绝并返回 `approval_required`，没有 PC 确认界面。
13. 服务端滚动升级，App/PC 重连并从 REST 状态恢复。
14. user A 尝试访问 user B 的所有资源均失败。

## 6. 威胁模型

| 威胁 | 例子 | 缓解 |
|---|---|---|
| Spoofing | 攻击者伪装 PC device | JWT + 设备归属 + 撤销状态；installation hash 不是凭证 |
| Tampering | 伪造 command/started 状态 | 事件必须来自设备连接，command 状态/CAS/turn_id 校验，审计异常；扣费只发生在 REST accepted 事务 |
| Repudiation | 用户否认发送/扣费 | command/charge 状态、idempotency、时间和 prompt hash 审计 |
| Information disclosure | Key/token/prompt/源码泄漏 | Secure Storage/Keyring、日志脱敏、正文 Redis TTL、默认不进 PostgreSQL |
| Denial of service | 海量 sync/WS/frame | 用户/设备限流、大小上限、bounded queue、timeout、backpressure |
| Elevation of privilege | 普通用户进 admin、远程任务扩大本机权限 | 服务端 role guard；Companion 使用预配置安全策略并拒绝意外 approval request |

### 必测攻击面

- SSRF：恶意 site URL、redirect、图片 URL。
- WebSocket CSRF/Origin 与 browser client 误用。
- IDOR：device/sync/thread/command/charge UUID。
- 重放：Idempotency-Key、WS event_id、旧 device token。
- 路径泄露与 path traversal：thread ID、cwd display、JSONL fallback。
- 命令注入：设备名、thread 标题、resume/open terminal 辅助功能。
- Markdown/HTML/XSS：公告、模型输出、命令输出、diff。
- 压缩/内存攻击：大 JSONL、大 SSE、大 WS frame、大 base64 图片。
- 供应链：Flutter/Rust/npm 依赖、Tauri updater、安装包签名。

## 7. 数据与隐私清单

### PostgreSQL 允许长期保存

- user/device ID、设备展示信息和版本。
- command/charge ID、状态、时间、金额、错误码。
- prompt SHA-256 和字节长度。
- thread ID 或其 hash、标题短快照（可配置关闭）。

### Redis 短期保存

- 在线 presence。
- session/thread snapshot 正文。
- 待派发 prompt payload。
- 短期用户事件 stream。

### 默认不在服务器持久保存

- Codex 会话全文、源码、diff 全文、命令输出全文。
- 普通聊天历史。
- API Key、JWT、Refresh Token、PC Keyring 内容。
- 本机绝对 rollout 路径。

必须为 TTL 和清理任务加指标；“设置了 TTL”不等于清理永远成功。

## 8. 性能与负载测试

建议场景：

- 10,000 PC presence、每 20 秒 heartbeat，跨 3 个后端实例。
- 1,000 Mobile 同时订阅，10% 正在接收 Codex delta。
- 单 user 2 台 PC、10,000 thread 元数据分页。
- 100 个并发相同 idempotency direct-charge 竞争。
- 1% Redis publish 丢失/延迟、后端实例 kill -9、PostgreSQL 主从切换模拟。
- 256 KiB item、32 KiB prompt、最大 WS frame 和截断路径。

发布 SLO 建议：

- 在线 PC command dispatch P95 < 1 s，P99 < 3 s。
- session sync P95 < 3 s，P99 < 8 s。
- duplicate charge count = 0。
- accepted command 与 charged 记录不一致计数 = 0。
- Hub queue overflow < 0.1%，且所有 overflow 可通过 REST/sync 恢复。

## 9. 可观测性与告警

### Metrics

```text
collab_ws_connections{client_type,status}
collab_device_presence_total{status}
collab_sync_duration_seconds{kind,result}
collab_commands_total{status,error_code}
collab_charges_total{result}
collab_charge_command_mismatch_total
collab_app_server_errors_total{kind,codex_version}
collab_relay_queue_depth{instance}
collab_payload_truncated_total{item_type}
```

### 告警

- 任意 duplicate charge 或 command/charge 原子性 invariant 失败：P0。
- WS 认证失败突增、跨用户授权失败：P1 安全告警。
- Redis relay error > 1% 持续 5 分钟：P1。
- app-server incompatible > 5%：P2，检查 Codex 更新兼容。
- sync P95 > 8 s：P2。

日志只包含 request/command/sync/device 关联 ID、状态与规范错误码。

## 10. 迁移与回滚

### 数据库迁移

- 新表先以 collaboration feature flag disabled 发布。
- 索引与唯一约束在启用流量前完成。
- 余额字段复用前先验证现有数据库精度和负数历史。
- Migration 向前兼容上一版本服务；滚动部署期间旧实例应忽略新表。

### 回滚

- 关闭 `collaboration.enabled`：拒绝新 command，保持 status/charges 只读。
- 连接中的 PC/Mobile 收 server.shutdown 并断开。
- 后台 sweeper 继续清理过期 command/payload，不修改已扣余额。
- 已 started 的 turn 不强制停止本机 Codex。
- 不立即 drop 表；至少保留一个审计周期。

## 11. 灰度发布

1. 内部单用户、单 PC，task fee 设置为 0 但完整跑 direct-charge 状态机。
2. 内部多 PC/多实例，模拟断线和外部占用。
3. 小白名单启用真实低费率，人工每日对账。
4. 5% 用户开启，只允许 companion 最小支持版本。
5. 25%/50%/100%，每阶段至少观察一个 command TTL 周期。

Feature flags：

- `collaboration.enabled`
- `collaboration.mobile_command_enabled`
- `collaboration.billing_enabled`
- `collaboration.content_persistence_enabled`
- `collaboration.allowed_user_ids`（灰度）

费率为 0 时也要生成 charge 记录或明确的 zero-charge audit，保证路径一致。

## 12. 发布清单

### Backend

- [ ] migrations 已备份并在同版本 staging 验证。
- [ ] feature flag 默认关闭。
- [ ] Redis 多实例路由、sweeper leader/lease 验证。
- [ ] 账务 invariant dashboard 和 P0 告警已启用。
- [ ] Nginx/CDN 支持 WebSocket upgrade、idle timeout 和 body/frame 限制。
- [ ] HTTPS、trusted proxies、CORS/Origin 策略正确。

### Android

- [ ] release 使用 production HTTPS policy。
- [ ] Keystore/secure storage、备份排除策略已验证。
- [ ] APK/AAB 签名与版本升级路径。
- [ ] 隐私说明覆盖本地聊天、协同中继和短期缓存。
- [ ] Crash/analytics SDK 已配置字段脱敏和 body 禁采集。
- [ ] 大字体、TalkBack、深色模式、低网速测试。

### PC

- [ ] Windows/macOS/Linux 安装/卸载和 OS Keyring。
- [ ] 代码签名、公证/SmartScreen 策略、Tauri updater 签名。
- [ ] 开机启动与托盘退出行为明确。
- [ ] Codex 最小/最大已验证版本矩阵。
- [ ] App-server 子进程在登出、退出、崩溃时正确清理。
- [ ] 脱敏诊断包人工抽查无 secret/prompt/path username。

### 产品与客服

- [ ] 帮助文档说明协同任务由后台固定扣费，App 不显示金额或账务状态。
- [ ] 明确“后端成功接收后即收费，后续失败不退款”的规则。
- [ ] 明确外部活跃 Codex 会话无法强制接管。
- [ ] 提供 PC offline、Codex incompatible、thread busy 的用户帮助。
