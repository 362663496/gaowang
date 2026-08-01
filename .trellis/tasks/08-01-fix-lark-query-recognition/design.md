# 技术设计：飞书自然语言查询与自适应卡片

## 总体边界

继续使用一条受控链路：

```text
飞书文本 → DeepSeek 查询计划 → 严格校验 → 本地参数化查询 → 服务端卡片渲染 → 回复
```

模型不执行 SQL、不读取数据库、不生成最终数字，也不直接生成飞书卡片 JSON。每次模型调用只生成查询语义和展示意图；查询执行后不追加模型调用，首次计划校验冲突时可在执行前纠正一次。

## 扩展查询计划

在现有 `larkAnalyticsPlan` 上增加最少的明确字段：

```text
domain: product | inventory | shop | movement | operator
metric: inventory_quantity | inventory_value | movement_quantity | movement_count | movement_value
operation: total | details | ranking | trend | share | comparison
group_by: [] | [product|shop|operator] | 最多两个唯一维度
movement_type: all | inbound | sales_outbound | adjustment
time_range: current | today | yesterday | current_week | previous_week |
            current_month | previous_month | current_year | last_n_days |
            date_range | all
days: 仅 last_n_days
date_from/date_to: 仅 date_range，YYYY-MM-DD，含首尾日期
time_bucket: day | month，仅 trend
compare_to: previous_period | previous_year，仅 comparison
sort: asc | desc
limit: 1..10
presentation: auto | summary | detail | ranking | matrix | trend | share | comparison
product_keyword/shop_keyword/operator_keyword: 最长 100 字符的筛选词
```

`movement_value` 保留内部键以减少迁移范围，但展示语义固定为“按当前采购价估算的采购/出库成本”，不是销售额。

## 计划校验矩阵

- 所有 JSON 继续 `DisallowUnknownFields`，所有枚举逐项校验。
- 聚合计划的 domain 可由 metric 推导并校验一致性；`details` 必须明确 domain。
- 分组维度不得重复，最多两个；数组顺序决定卡片主/次维度。
- `total` 不分组；`ranking` 支持一到二维；`share` 只支持一个维度。
- `trend` 使用日或月时间桶，可再带最多一个业务维度。
- `comparison` 可带最多一个业务维度，并且必须指定上期或去年同期。
- `details` 可列出商品、当前库存、店铺、匹配流水或历史操作人，最多 10 条；单个商品详情和简单当前库存继续复用已有受控 intent。
- 当前库存指标只允许当前时间、商品维度和 `total/ranking/share`；库存并非按店铺拆分，不能由流水推断店铺库存。
- `last_n_days` 仍限制 1–365；任意更长范围使用明确日期或全部历史。
- 明确日期按上海时区解析为 `[start, end)` UTC 边界，`from <= to`。
- 当 metric/group_by 已能唯一确定总计或排行时，服务端纠正模型给出的 `details/total/ranking` 标签；展示类型冲突归一为 `auto`。筛选、维度和时间条件不得被纠正流程丢弃。

## 查询实现

不引入通用查询引擎。复用现有 GORM 模式并拆成几个固定分支：

- 基础过滤器只按所需维度关联 `products`、`shops`、`users`，所有输入使用绑定参数。
- 指标表达式来自常量映射，不拼接模型输出。
- 一维/二维排行按固定 ID、名称字段分组，指标排序后加稳定次序，取 `limit+1` 判断截断。
- 商品/库存明细复用当前商品及快照查询；店铺明细只返回名称、启用状态和业务备注；操作人明细从流水历史去重，只返回姓名；流水明细复用现有查询和关联预加载并增加计划筛选。
- 趋势按 SQLite/PostgreSQL 小型日期表达式辅助函数分日或分月，返回最近 10 个可展示桶并标记截断。
- 占比先查询完整过滤总量，再查询最多 10 个分组；服务端使用整数基点计算比例，避免浮点金额。
- 对比对当前区间和参考区间各运行一次同形聚合，在 Go 中按稳定维度键合并，计算当前值、参考值、差值和百分比。
- 统计总量覆盖完整过滤数据；10 行限制只约束飞书卡片展示。

操作人仅按姓名筛选和展示。已软删用户因历史外键仍可参与明细和统计，但邮箱、角色、权限、删除状态不进入计划或卡片。

## 时间语义

- 周从上海时区周一开始。
- 上周、上月使用完整自然周期；本周、本月、本年截至当前查询时间但 SQL 上界仍使用当前周期结束，未来无数据不影响结果。
- `previous_period`：自然周期使用对应上一周期；最近 N 天/明确日期使用等长紧邻前一段。
- `previous_year`：当前区间首尾各向前一年，闰日按 Go 日历规则处理。
- `all` 不支持趋势对比的隐含参考期；超出时返回清晰能力提示。

## 自适应卡片

使用已安装的飞书 Go SDK 组件，不增加前端或图表依赖：

- `summary`：突出单个 KPI，并列时间、筛选条件和成本口径。
- `detail`：复用商品图片、类型、数量、店铺、操作人和时间的流水块。
- `ranking`：序号、核心数值、辅助笔数/成本；商品保留图片。
- `matrix`：按第一维度分段，第二维度作为子项，适合商品 × 店铺。
- `trend`：日期/月标签、真实数值和服务端生成的紧凑文本条形，不以颜色单独传递信息。
- `share`：分组数值、百分比和文本比例条；分母来自完整结果。
- `comparison`：当前、参考、差值和百分比并列，零基数显示“无可比基数”。

标题、颜色、字段、图片、单位、截断提示和空状态全部由服务端从已校验计划与真实结果生成。卡片顶部固定显示时间范围和有效筛选，避免结果脱离问题语境。

## 重试与错误诊断

将统一哨兵错误细分为不含原文的结构化诊断：

```text
stage: config | marshal | request | transport | http_status |
       response_decode | finish_reason | plan_decode | plan_validate
status_code: 可选 HTTP 状态
retryable: bool
attempts: 1..2
elapsed_ms: 总耗时
```

- `Parse` 建立 10 秒总上下文；每次 HTTP 调用仍受 6 秒客户端超时限制。
- transport/timeout、429、5xx 重试一次，短暂等待使用标准库 timer 并响应 context 取消。
- `plan_validate` 可携带上一份计划向同一模型请求一次纠正；第二份计划仍走完整数据边界和参数校验。响应 JSON、finish reason、`plan_decode` 和其他 4xx 只请求一次。
- 网络重试与计划纠正共享最多两次请求和 10 秒总预算，不叠加第三次调用。
- 最终失败和重试后恢复均记录 `message_id`、stage、status、attempts、elapsed；不记录输入文本、模型响应、API Key 或 Authorization。
- 上游/配置故障回复“智能服务暂时异常”；计划解析或能力不支持回复“这次没有理解清楚/请缩小问题”，两者都明确本次未执行查询。
- 成功审计只保存已校验计划字段、尝试次数和耗时，不保存原始消息或供应商内容。

## 兼容与回滚

- 保持现有所有非空文本走 DeepSeek，不恢复固定命令优先级。
- `lark.query_analytics` 审计 action 和现有简单 intent 保持不变。
- 无数据库 schema 变更；回滚只切回上一应用版本。
- 结果限制从 5 提升为 10，只影响卡片展示；队列大小和 20 秒外层任务超时保持不变。

## 测试重点

- 查询计划枚举、二维保真、日期边界、非法组合和 10 行限制。
- 商品 × 店铺、明确日期、周/月/年、明细、趋势、占比、上期/同期对比的 SQLite 查询。
- 500/429/超时后成功，`plan_validate` 纠正后成功，第二份坏计划停止，以及 4xx/坏响应 JSON/`plan_decode` 不重试和 10 秒总截止。
- operation/presentation 冲突本地归一、非法枚举二次纠正，以及连续两份非法计划停止执行。
- 日志/错误对象不包含输入、API Key 或响应正文。
- 七种卡片语义、空数据、截断和真实数值；模型输入中的伪卡片内容不得进入输出。
- 删除用户的历史操作人统计仍显示姓名而不显示账号字段。
