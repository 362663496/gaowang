# 飞书能力与现有系统接入调研

调研日期：2026-07-31

## 结论

产品已决定首期使用一个企业自建应用机器人，并且只服务一个固定库存群：

- 系统通过机器人向固定群主动发送通知。
- 机器人接收该群内的 @消息，查询商品或库存后回复。
- 首期忽略其他群和私聊消息。
- 不再配置群自定义机器人 Webhook。

## 飞书官方能力

### 自定义机器人 Webhook

- 只能在创建它的当前群聊中使用。
- 通过 Webhook 单向推送文本、富文本、图片和飞书卡片。
- 不具有数据访问权限，不能承担 @机器人查询。
- 支持关键词、IP 白名单、签名校验。
- 单租户单机器人限制为 100 次/分钟、5 次/秒，请求体不超过 20 KB。

官方文档：

- [自定义机器人使用指南](https://open.feishu.cn/document/ukTMukTMukTM/ucTM5YjL3ETO24yNxkjN?lang=zh-CN)

### 企业自建应用机器人

- 可按 `chat_id` 向群聊发消息，也可按 `open_id` 等用户标识发单聊消息。
- 订阅 `im.message.receive_v1` 后，可接收机器人私聊和群内 @机器人消息。
- 可回复原消息，并支持话题回复。
- 支持文本、富文本、图片、文件和交互卡片。
- 主动发送和回复接口支持 UUID 去重；同一 UUID 在一小时内至多成功一次。

官方文档：

- [机器人使用指南](https://open.feishu.cn/document/client-docs/bot-v3/how-to-use-bot-in-feishu)
- [接收消息事件](https://open.feishu.cn/document/server-docs/im-v1/message/events/receive?lang=zh-CN)
- [发送消息](https://open.feishu.cn/document/server-docs/im-v1/message/create?lang=zh-CN)
- [回复消息](https://open.feishu.cn/document/server-docs/im-v1/message/reply?lang=zh-CN)

### 事件订阅与卡片交互

- 事件订阅支持长连接和公网 HTTP 回调两种方式。
- 长连接仅支持企业自建应用，不要求公网 IP 或域名，只要求服务端能访问公网；SDK 内置鉴权和加密处理。
- 长连接每个应用最多 50 条连接，多实例为集群消费而非广播。
- 事件与卡片回调都要求在 3 秒内完成确认，否则会重推或在客户端显示失败。
- 消息事件特殊情况下会重复，消息处理按 `message_id` 去重。
- 卡片按钮、选择器等操作可回传服务端，适合后续增加筛选、翻页和需要确认的操作。

官方文档：

- [事件概述](https://open.feishu.cn/document/server-docs/event-subscription-guide/overview?from=from_parent_docs)
- [使用长连接接收事件](https://open.feishu.cn/document/server-docs/event-subscription-guide/event-subscription-configure-/request-url-configuration-case?lang=zh-CN)
- [配置卡片交互](https://open.feishu.cn/document/common-capabilities/message-card/add-card-interaction/interaction-module)
- [频控策略](https://open.feishu.cn/document/server-docs/api-call-guide/frequency-control?lang=zh-CN)

## 现有系统可复用点

- `InventoryHandler.ListCurrent` 已支持关键词、低库存过滤、商品预加载和当前库存金额计算。
- `ProductHandler.List` 已支持按商品名称或编码模糊查询。
- 受保护路由已有商品读取、库存读取和流水读取权限。
- 入库、销售出库、库存调整和商品变更都会写审计日志。
- API 当前是单个 Go 进程，配置来自环境变量，尚无通用消息队列或事件总线。

## 最小首期方案

1. 创建一个企业自建应用机器人，并加入固定库存群。
2. 向该群通知成功入库、销售出库、库存调整，以及低库存进入/恢复。
3. 接收该群内的 @消息，提供商品、库存和低库存只读查询。
4. 库存业务成功后异步发送通知，飞书故障不影响原业务结果。
5. 首期不开放私聊、其他群聊或飞书内库存写操作。

## 技术选型补充

- 使用官方 Go SDK 的长连接订阅，不新增公网 HTTP 回调。
- 只配置应用 ID、应用密钥和固定群 `chat_id`；长连接不需要 Verification Token 或 Encrypt Key。
- 使用进程内有界队列承接查询和通知，不增加 Redis、消息中间件或数据库 outbox。
- 图片按需上传飞书并做进程内缓存，失败时发送无图卡片。
- 新增最小权限：发送机器人消息、接收群内 @机器人消息、上传消息图片。
