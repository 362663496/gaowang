# 技术设计：移除商品销售价

## 运行时数据契约

从 Go 模型和前端共享类型移除：

- `Product.DefaultSaleCents`
- `StockMovement.SaleUnitCents`
- `StockMovement.RevenueCents`
- `StockMovement.GrossProfitCents`
- 流水预览中的收入、毛利及其变化字段

生产数据库中的 `default_sale_cents`、`sale_unit_cents`、`revenue_cents`、`gross_profit_cents` 旧列不执行 DROP。GORM `AutoMigrate` 不会删除未映射列，因此旧值保留；新运行时代码完全忽略它们。

继续保留并使用：

- 商品当前默认采购价 `DefaultPurchaseCents`
- 入库采购金额 `PurchaseAmountCents`
- 销售出库/调整的当前采购成本 `CostAmountCents`
- 当前库存价值 `InventoryValueCents`

## 商品接口与页面

- multipart 创建/更新只解析 `name`、`code`、`default_purchase_cents`、`low_stock_threshold`、`note` 和图片。
- 旧客户端额外提交的 `default_sale_cents` 被忽略，不影响过渡期；响应不再包含该字段。
- 商品列表 `summary` 只返回 `total`、`enabled`；页面展示商品数、已启用、已禁用。
- 商品表格和表单去掉销售价，页面说明改为采购价和低库存阈值。

## 库存流水

`applySalesOutbound` 只接收采购价：校验库存、扣减数量、重算库存价值，并计算 `quantity × current default purchase price` 的销售出库成本。不生成售价、收入或毛利。

`CurrentPriceMovement` 继续按当前采购价推导：

- 入库：采购金额。
- 销售出库：销售成本。
- 调整：成本影响。

流水列表响应移除收入和毛利，只保留采购/成本金额。最新流水修订的 before/after、impact 和审计数据也只包含数量、库存价值、采购金额与成本变化。更新旧流水时不触碰生产库保留的旧财务列。

## 报表 API

保留现有路由和权限键，避免持久化权限迁移；收敛响应：

```text
sales-summary: quantity_sold, movement_count, cost_cents
sales-trend: day, quantity_sold, movement_count, cost_cents
product-ranking: product identity, quantity_sold, movement_count, cost_cents
shop-ranking: shop identity, quantity_sold, movement_count, cost_cents
```

- 排行按 `quantity_sold DESC`，再按流水笔数和稳定名称/ID 排序。
- 成本明确标注为“按当前采购价估算”，不表示真实订单收入或利润。
- 历史商品仍通过无归档过滤的关联参与报表。

## 仪表盘、报表与流水页面

- 仪表盘销售指标改为累计销售数量、销售出库笔数和估算采购成本；最近流水金额只显示采购/成本金额。
- 报表摘要、趋势和排行以数量为主指标，以笔数和估算成本为辅助。
- 流水表删除收入/毛利列，保留一个“采购/成本金额”列，并将“日期”紧跟在该金额列之后；修订预览删除收入/毛利变化。
- 库存价值、低库存和采购价相关展示不变。

## 飞书依赖

飞书只允许库存数量、库存价值、流水数量、流水笔数和按当前采购价估算的流水成本。标题和帮助文案不得出现销售额、收入或毛利。该契约由后续飞书子任务消费。

## 迁移与回滚

- 正向发布无破坏性 DDL；旧列和旧值保留。
- 新商品在生产旧列中使用数据库默认零值，新销售流水的旧收入/毛利列同样为零。
- 回滚到旧版本时 `AutoMigrate` 可在全新数据库补回旧列；生产旧列本来就存在。旧版本可能重新展示其旧估算逻辑，这是应用回滚语义，不影响新版本写入的库存数量与采购成本。

## 测试重点

- 商品创建/更新无需销售价，额外旧字段被忽略，响应和汇总无销售价。
- 销售出库与最新流水修订只更新数量、库存价值和采购成本。
- 报表数量、笔数、成本与稳定排序在 SQLite 测试中正确。
- Web 类型、商品、流水、仪表盘和报表不再引用销售额/毛利。
- 流水表静态列顺序确认成本后立即显示日期。
- `apps/api` 与 `apps/web` 字段扫描无运行时销售价、收入或毛利依赖。
