# 实施计划：飞书自然语言查询与自适应卡片

## 1. 前置与影响分析

- [x] 确认 `08-01-remove-product-sale-price` 已完成，飞书可用指标中没有销售额/毛利。
- [x] 对 `deepSeekIntentParser.Parse`、`deepSeekResultCommand`、`normalizeLarkAnalyticsPlan`、`queryLarkAnalytics`、`queryLarkMovementAnalytics`、`larkAnalyticsRange`、`larkAnalyticsCard`、`larkBot.handle/recordAudit` 执行 GitNexus upstream impact。
- [x] 对 HIGH/CRITICAL 结果先报告影响流程和风险。

## 2. 失败分类与有限重试

- [x] 增加仅含阶段、状态、重试性、次数和耗时的内部诊断错误；保留 `errors.Is` 所需兼容语义。
- [x] 把单次 HTTP 封装为最小 attempt 函数，正确关闭响应体。
- [x] 使用 10 秒总 context 和最多两次请求；仅网络/超时/429/5xx 重试。
- [x] 在 handle 层带 `message_id` 记录最终失败或重试恢复，确保不记录原文/响应/密钥。
- [x] 区分上游暂时异常卡片与计划未理解/不支持卡片。

## 3. 查询计划与查询分支

- [x] 扩展 DeepSeek 提示、严格 JSON 结构和最大 token，加入 domain、operation、二维数组、日期、趋势/对比和 presentation 示例。
- [x] 扩展 `larkAnalyticsPlan` 与 `auditMetadata`，实现完整白名单校验矩阵。
- [x] 实现上海时区的本/上周、本/上月、本年、明确日期和比较区间。
- [x] 复用固定 GORM 表达式实现商品/库存/店铺/流水/操作人明细，以及总计、一/二维排行、趋势、占比和比较；所有输入参数绑定。
- [x] 将结果展示上限提升为 10，完整总量/占比分母不受展示限制。
- [x] 操作人只使用姓名，保留软删用户历史关联。

## 4. 自适应卡片

- [x] 由 `presentation=auto` 或已校验展示意图分派 summary/detail/ranking/matrix/trend/share/comparison 渲染。
- [x] 复用现有 `larkCard`、`larkDetailElement`、图片解析和 Markdown 转义。
- [x] 加入时间/筛选上下文、成本口径、空状态和截断提示。
- [x] 所有比例、差值、条形和标题由服务端结果计算；忽略任何模型自由文本。
- [x] 更新帮助卡片，加入二维、日期、趋势、占比和对比示例。

## 5. 自动验证

- [x] 扩充 `lark_ai_test.go`：宽范围合法计划、未知字段、维度上限、日期校验、重试分类、总截止和敏感信息。
- [x] 扩充 `lark_analytics_test.go`：二维、趋势、占比、比较、明确日期、稳定排序、截断和删除操作人历史。
- [x] 扩充 `lark_query_test.go` / `lark_test.go`：七种卡片、错误卡片、审计元数据、无固定兜底。
- [x] `cd apps/api && gofmt -w <changed-go-files> && go test ./internal/services ./internal/http/...`。
- [x] `cd apps/api && go test ./... && go vet ./...`。

## 6. 手工验收与回滚点

- [ ] 在目标群验证商品库存、店铺排行、商品 × 店铺、明确日期、趋势、占比、同期对比和明细问题。
- [x] 注入一次可重试上游失败，确认第二次成功、日志含阶段/次数但无原文。
- [x] 注入坏计划，确认只调用一次并返回“未理解/不支持”而非服务暂时异常。
- [x] 检查卡片最多 10 行、完整分母、图片降级、时间与筛选上下文。
- [x] 回滚无需数据迁移；切回上一发布即可恢复旧查询计划。
