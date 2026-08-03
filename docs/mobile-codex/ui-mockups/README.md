# UI 界面图集（简约版）

本目录保存依据 [`../05-ui-generation-prompts.md`](../05-ui-generation-prompts.md) 和用户提供的视觉参考图生成的 Sub2API Mobile 界面图。

## 当前图集

| 文件 | 覆盖页面 |
|---|---|
| `mobile-navigation-v2.png` | 四项底部菜单：聊天、协同、秘钥、我的；包含秘钥卡片和我的菜单 |
| `mobile-chat-v2.png` | 普通聊天详情、当前聊天秘钥、图片入口与 Markdown 回复 |
| `mobile-collaboration-v2.png` | 极简协同首页、PC 在线状态、同步与最近 Codex 会话 |
| `mobile-collaboration-thread-v2.png` | 协同会话详情、手动同步、折叠工具活动与任务输入 |
| `mobile-key-profile-interactions-v2.png` | 秘钥卡片、分组选择、公告 Bottom Sheet、兑换 Dialog 与充值入口 |

## 已确认的界面约束

- 底部菜单只有“聊天、协同、秘钥、我的”。
- App 不展示协同价格、账务状态、冻结/结算/退款或电脑审批。
- 秘钥卡片展示名称、脱敏 Key、分组下拉、用量和当前聊天选择。
- “公告”从“我的”打开；“兑换”使用 Dialog；“充值”由系统外部浏览器打开 `https://pay.ldxp.cn/shop/codecodeai`。
- 图片中的 Key、邮箱、ID 和路径必须是脱敏假数据。

图片由内置 `image_gen` 生成，只用于视觉与布局参考；最终组件由 Flutter 实现并接受功能、可访问性与安全测试。
