# 飞书机器人查询体验升级：技术设计

## 1. 边界

- 继续使用现有 `services.RunLarkBot`、固定群策略、后台队列和飞书长连接。
- 不增加 HTTP 路由、数据库表、前端页面或第三方 Go 依赖。
- 业务改动集中在 `apps/api/internal/services`；配置仍由 `internal/config` 读取。
- DeepSeek 只做无状态意图分类，数据库查询、价格计算、排序、上限和卡片内容均由本地代码控制。

## 2. 命令与意图

去除机器人提及后，任何非空文本都进入 DeepSeek；本地只处理非文本与空文本边界，不再解析首词或固定命令。DeepSeek 结果只接受：`inventory`、`low_stock`、`out_of_stock`、`inventory_summary`、`inventory_value_ranking`、`movements`、`today_changes`、`today_sales_ranking`、`help`、`unknown`，以及可选 `keyword`。本地校验把结果转换为现有命令结构，白名单外字段或动作直接视为解析失败。

验收语料至少覆盖：

- 商品：还有多少、有没有货、编码查询、当前价格、库存金额、商品状态。
- 预警：哪些快没了、哪些需要补货、低于库存线的商品、低库存数量。
- 概览：总数量、总金额、商品数、无库存数、整体库存情况。
- 流水：最近变化、最近操作人、指定商品最近流水、指定商品是否发生过出入库。
- 今日：今日总变动、今日入库量、今日出库量、今日调整次数。
- 新功能：缺货/没货清单、库存金额最高/价值排行、今日最好卖/销售排行。

## 3. DeepSeek 客户端

- 新增一个小型标准库 HTTP 客户端，不引入 SDK。
- 请求 `POST https://api.deepseek.com/chat/completions`，使用 `Authorization: Bearer ...`。
- 默认模型 `deepseek-v4-flash`，显式关闭思考模式，设置 `response_format: {"type":"json_object"}` 和较小输出上限。
- 系统提示只描述白名单意图、参数约束和 JSON 示例；用户消息只包含移除机器人提及后的文本。
- `DEEPSEEK_API_KEY` 为空时不创建 AI 解析器；`DEEPSEEK_MODEL` 为空时使用默认模型。
- HTTP 客户端使用短超时。非 2xx、空内容、截断、非法 JSON、未知意图和关键词超限统一返回受控错误，不读取或记录响应正文。
- 测试通过 `httptest.Server` 注入 endpoint，不新增生产配置项。

DeepSeek 官方文档：

- [Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion)
- [JSON Output](https://api-docs.deepseek.com/guides/json_mode/)
- [模型与价格](https://api-docs.deepseek.com/quick_start/pricing/)

## 4. 查询数据流

```text
飞书消息
  → 固定群/@/去重策略
  → 非文本/空文本边界检查
  → DeepSeek → 白名单校验 → 查询
  → GORM 只读查询
  → 对每条具体商品解析并缓存飞书 image_key
  → 一张交互卡片回复
  → 写审计
```

### 商品和低库存

- `products LEFT JOIN inventory_snapshots`，`COALESCE(quantity, 0)`。
- 排除归档商品，名称/编码大小写不敏感模糊匹配。
- 低库存使用 `threshold > 0 AND COALESCE(quantity, 0) <= threshold`。
- 取 6 条判断是否超过上限，最终展示 5 条。
- 缺货沿用商品 LEFT JOIN，筛选数量小于等于 0。
- 库存金额排行筛选数量大于 0，按 `数量 × 当前采购价` 倒序并使用名称、编码、ID 作为稳定次序。

### 库存概览

单条聚合查询返回未归档商品数、总数量、`数量 × 当前采购价` 总额、低库存数和无库存数。使用 `CASE` 与 `COALESCE`，保持 PostgreSQL/SQLite 测试兼容。

### 流水与今日变动

- 流水预加载 Product、Shop、Operator，按 `created_at DESC, id DESC`，最多 5 条；关键词通过关联商品名称/编码过滤。
- 今日边界以 `Asia/Shanghai` 当日零点计算后转 UTC 查询。
- 今日汇总分别计算入库、销售出库、调整的笔数与数量；不把历史流水中的旧价格作为金额来源。
- 今日销售排行只聚合上海当日销售出库数量，并 LEFT JOIN 当前库存快照；返回当前库存、采购价、库存金额和今日售出数量。

## 5. 卡片与图片

- 抽取一个最小的“按元素构建卡片”入口，现有卡片继续复用。
- 具体商品使用 `div` 主体，商品图放进 `extra` 作为右侧紧凑缩略图，支持点击预览；不再使用顶部 `fit_horizontal` 全宽大图。
- 商品列表和流水卡片按“商品字段 + 右侧缩略图”重复排列，最多 5 组，组间使用分隔线。
- 图片继续通过现有上传器和进程内 `image_key` 缓存获取；逐图失败返回空 key，仅省略该图片。
- 多商品展示与单商品使用同一商品 Markdown 生成函数，避免字段差异。
- 商品库存数量移到主 Markdown 区，用 `📦`/`⚠️`/`⛔`、粗体和“件”单独强调；短字段只保留状态、采购价、库存金额及排行指标。
- 商品卡片不再显示售价，所有商品金额相关展示只使用当前采购价。
- 通知标题使用语义色：入库/增加为绿色，销售出库/减少为红色，库存调整为蓝色，进入低库存为橙色，恢复正常为绿色；关键数量配合 `⬆️`、`⬇️`、`⚠️` 等符号增强识别。
- 不依赖仅部分客户端支持的复杂富文本颜色语法；视觉重点由标题语义色、粗体、字段布局和符号共同完成。

## 6. 失败与兼容

- AI 失败：回复“智能识别暂时不可用”，明确本次未执行查询或库存操作，不做本地猜测，不影响后续消息。
- 图片失败：仅无图降级。
- 查询或卡片失败：沿用当前日志边界并返回查询失败卡片。
- 飞书回复失败：记录一次错误，不影响库存系统。
- “查库存”“查商品”“帮助”等短句与普通句子一样由 DeepSeek 统一识别。
- 不配置 DeepSeek Key 时通知仍可用，但所有文本查询统一返回暂不可用；回滚整个功能只需部署上一 API 版本，无数据库回滚。

## 7. 安全

- Key 只存在服务器环境变量；错误、审计、测试快照和日志都不包含 Key。
- 不把模型输出当 SQL、字段名、排序表达式或工具调用执行。
- 不把库存明细发给 DeepSeek，也不保留聊天上下文。
- 固定群与 @策略在 AI 之前执行，AI 不扩大访问范围。
