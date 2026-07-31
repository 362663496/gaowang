# 飞书库存集成技术设计

## 1. Summary

在现有 Go API 进程中接入飞书官方 Go SDK：

- 使用 SDK 长连接接收固定群内的 `im.message.receive_v1`。
- 使用飞书消息 API 回复查询并主动发送库存操作通知。
- 不新增公网回调路由、不修改 Nginx、不新增数据库表。
- 使用进程内有界队列和有限重试，保证飞书故障不阻断库存主流程。

```text
Web 库存操作
  → InventoryService 事务
  → InventoryChange
  → HTTP 成功/审计
  → 通知队列
  → 飞书固定群

飞书固定群 @机器人
  → SDK 长连接
  → 群/消息校验与去重
  → 查询队列
  → PostgreSQL 只读查询
  → 审计
  → 回复原消息
```

## 2. Architecture and boundaries

### 2.1 Configuration

在 `config.Config` 与 `config.Load` 增加：

- `LarkAppID` ← `LARK_APP_ID`
- `LarkAppSecret` ← `LARK_APP_SECRET`
- `LarkChatID` ← `LARK_CHAT_ID`

启用规则：

- 三项全空：集成关闭。
- 三项全有：集成开启。
- 部分配置：返回稳定配置错误，不包含配置值。

长连接内置鉴权和传输保护，因此首期不需要 Verification Token、Encrypt Key 或公网回调 URL。

### 2.2 Process startup

`cmd/api/main.go` 在数据库和迁移成功后启动飞书监听 goroutine；监听错误只记录一次进程日志，不终止 API。API 退出时取消监听 context。

选择长连接的原因：

- 企业自建应用官方支持并推荐。
- 当前部署只需要主动访问公网，无需增加公开回调入口或调整 Nginx。
- SDK 负责连接鉴权、加密和重连。
- 避免修改高影响的 `NewRouter`。GitNexus 显示该符号为 HIGH，存在 26 个直接调用/测试点。

### 2.3 Service ownership

飞书逻辑放在现有 `internal/services`，不新增 repository、接口或工厂：

- `RunLarkBot(ctx, cfg, db)`：建立长连接、接收消息、去重、排队和回复。
- `LarkNotifier`：由 `mountProtected` 创建并注入 `InventoryHandler`，负责非阻塞排队通知。
- 共享的卡片构建、图片上传缓存、消息发送和重试均为 `services` 内部实现。

本设计允许监听端与通知端各持有一个 SDK client。它们职责独立，避免改变 `NewRouter` 签名或引入全局单例。

## 3. Contracts

### 3.1 Inventory change result

三种库存创建方法调整为返回：

```go
type InventoryChange struct {
    Product        models.Product
    Movement       models.StockMovement
    QuantityBefore int64
    QuantityAfter  int64
}
```

结果在原事务成功提交后才交给 HTTP handler；任何飞书调用都不得进入库存事务。

状态变化由结果纯计算：

- `beforeLow = threshold > 0 && before <= threshold`
- `afterLow = threshold > 0 && after <= threshold`
- `false → true`：进入低库存。
- `true → false`：恢复正常。
- 其余情况：无状态告警。

### 3.2 Incoming message

只接受同时满足以下条件的事件：

- `chat_id == LARK_CHAT_ID`
- `chat_type == group`
- 消息类型为文本
- 消息包含对当前机器人的 @
- `message_id` 未处理

事件 handler 只完成校验、去重和入队，随后立即返回。队列 worker 解析命令、查库、写审计并回复。

命令解析只使用标准库字符串/JSON处理：

- 去掉 @机器人标记和首尾空白。
- 首个空格前为命令，其余内容为关键词。
- `低库存`、`帮助` 不接收关键词。
- 不做同义词、分词、拼音或智能纠错。

### 3.3 Query semantics

- 商品和库存均排除 `archived_at IS NOT NULL`。
- 关键词匹配 `name ILIKE` 或 `code ILIKE`。
- 查询最多取 6 条：前 5 条用于展示，第 6 条只用于判断“结果过多”。
- 排序沿用库存页面：名称、编码、商品 ID。
- 低库存沿用现有阈值规则。
- 库存金额调用 `CurrentInventoryValue`，保持 `数量 × 当前采购价` 的唯一算法。

`查库存` 以库存快照为主；`查商品` 以商品为主并补充快照。`低库存` 返回最多 5 个低库存商品。

### 3.4 Cards and images

- 唯一结果：完整商品卡片。
- 多结果：一张紧凑卡片列出最多 5 条，并在需要时提示缩小关键词。
- 操作通知：一张卡片显示操作与结果；若发生库存状态切换，在同一卡片中改变主题色并增加状态文案。
- 图片通过 SDK 上传为消息图片并缓存 `ImagePath → image_key`。
- 缓存只存在于进程内；图片路径变化自然产生新键。
- 文件读取、上传或缓存失败时省略图片，继续发送卡片。

### 3.5 Audit

机器人查询写入 `models.AuditLog`：

- `ActorID = nil`
- `Action` 使用 `lark.query_inventory`、`lark.query_product`、`lark.query_low_stock` 或 `lark.help`
- `ResourceType = lark_message`
- `ResourceID = message_id`
- Metadata 只记录 `chat_id`、发送者 `open_id`、命令和非敏感关键词
- `IPAddress` 留空

审计失败记录进程日志，但不阻止查询回复，保持与普通 HTTP 审计的 best-effort 语义一致。

## 4. Reliability

### 4.1 Queues

- 查询和通知各使用一个小型有界 channel 与单 worker。
- 事件 handler 和库存 HTTP handler 均不得等待飞书网络请求。
- 队列满时丢弃该条任务并使用 `slog` 记录，不阻塞库存主流程。

首期不引入 Redis、消息中间件或数据库 outbox。已知上限是进程重启会丢失尚未发送的通知；当业务要求“保证投递”时再升级为持久化 outbox。

### 4.2 Retry and idempotency

- 主动发送与回复设置 UUID 幂等键。
- 对网络错误、HTTP 429 和 5xx 做少量有界重试，并在可用时遵循服务端建议等待时间。
- 非重试错误立即记录。
- 接收侧维护有界 `message_id` 去重集合，避免飞书重推造成重复回复。

### 4.3 Failure isolation

- 飞书未配置：所有组件为关闭状态。
- 长连接断开：SDK 重连；最终退出仅写日志，API 继续服务。
- 图片失败：无图降级。
- 回复失败：记录日志，不重跑查询写入。
- 通知失败：记录日志，不改变库存事务和 HTTP 响应。

## 5. Security and permissions

- 密钥只存在服务端环境。
- 日志禁止输出 app secret、tenant access token 或完整 SDK 请求头。
- 固定 `chat_id` 是首期授权边界。
- 机器人仅执行只读查询，不复用 Web session，也不伪造系统用户权限。
- 飞书应用只申请：
  - `im:message:send_as_bot`
  - `im:message.group_at_msg:readonly`
  - `im:resource`

## 6. Compatibility and migration

- 无数据库 schema 变化。
- 新增官方依赖 `github.com/larksuite/oapi-sdk-go/v3`。
- `.env.example` 增加三项空配置；现有环境默认关闭集成。
- Nginx 和 Web 前端不变。
- 商品价格、库存金额、低库存和归档语义继续复用现有后端规则。

## 7. Impact and risk

GitNexus 规划期分析：

- `Config.Load`：MEDIUM，8 个直接调用/测试。
- `InventoryService.CreateInbound`：MEDIUM，9 个直接调用/测试；销售出库和调整属于同一风险面。
- `InventoryHandler.CreateInbound`：LOW。
- `mountProtected`：LOW（1 个直接调用，间接覆盖现有路由测试）。
- `NewRouter`：HIGH，26 个直接调用/测试；设计刻意不修改其签名或主体。

实施前仍需按仓库规则对每个实际修改符号重新执行 upstream impact；若出现 HIGH/CRITICAL，先停下并报告。

## 8. Operations and rollback

上线步骤：

1. 飞书后台创建企业自建应用并启用机器人。
2. 开通最小权限并订阅“接收消息 v2.0”。
3. 选择长连接订阅方式并发布应用。
4. 将机器人加入固定库存群，取得该群 `chat_id`。
5. 在服务器环境文件加入三项配置并按 Aliyun 原子发布流程部署。
6. 验证帮助、商品/库存/低库存查询，以及三种库存操作通知。

回滚：

- 清空三项 `LARK_*` 环境变量并重启 API，即可关闭集成。
- 代码回滚不涉及数据库恢复；发布仍沿用现有版本 symlink 回滚。
