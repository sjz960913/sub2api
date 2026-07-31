# UI 界面图集

本目录保存依据 [`../05-ui-generation-prompts.md`](../05-ui-generation-prompts.md) 生成的 Sub2API Mobile 与 Codex PC Companion 高保真界面图。

## 图集清单

| 文件 | 覆盖页面 |
|---|---|
| `mobile-01-auth-chat.png` | M-01 站点配置、M-02 登录、M-03 对话首页 |
| `mobile-02-chat-image-announcement.png` | M-04 普通聊天、M-05 图片生成、M-06 公告 |
| `mobile-03-codex-discovery.png` | M-07 PC 设备、M-08 会话查询与离线状态 |
| `mobile-04-codex-task-billing.png` | M-09 会话详情、PC 审批等待、M-10 任务退款 |
| `mobile-05-profile-admin.png` | M-11 我的、管理员入口与 AdminShell |
| `desktop-01-login-overview.png` | P-01 登录与环境检查、P-02 桌面概览 |
| `desktop-02-sessions-tasks.png` | P-03 会话管理、P-04 实时任务 |
| `desktop-03-approval-settings.png` | P-05 审批中心、P-06 设置与诊断 |

## 生成约束

- 协同任务固定显示 `本次任务费用 $0.10 USD`；不得出现 `¥`、CNY、汇率换算或其他协同费率。
- 普通聊天与 GPT Image 使用站点渠道费率，不显示协同任务固定费用。
- 图片中的 Key、邮箱、ID 和路径必须是脱敏假数据。
- 移动端采用 Material 3、390×844 artboard；桌面端采用紧凑型生产力布局、1440×900 artboard。
- 图片只用于视觉和布局参考，最终组件仍由 Flutter/Tauri 实现并接受可访问性测试。

生成任务的精确 JSONL 提示词位于 `tmp/imagegen/sub2api-ui-prompts.jsonl`，使用技能自带的 `image_gen.py generate-batch` 执行。
