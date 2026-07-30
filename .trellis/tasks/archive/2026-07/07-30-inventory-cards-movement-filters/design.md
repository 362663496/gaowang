# 库存卡片与流水筛选：技术设计

## Scope and invariants

- `Product.DefaultPurchaseCents` 与 `Product.DefaultSaleCents` 是唯一价格源；库存快照的数量和流水的 `QuantityDelta` 是数量事实。
- 库存仍按商品全局汇总，店铺仅是流水属性；负库存保护、商品归档保护、最新流水限制、版本检查和事务内审计保持不变。
- 前后端、报表和导出必须在同一版本原子切换，避免旧客户端继续提交操作单价。
- 不新增依赖、价格表、批次表或数据库迁移。

## Impact analysis

GitNexus 在当前提交重新索引后显示：

- `applyInbound`、`applySalesOutbound`、`applyAdjustment` 各有 2 个直接调用方，覆盖新建操作与最新流水修订；风险均为 LOW。
- `calculateMovementUpdate` 影响预览、保存及对应服务/路由/报表测试；风险为 LOW。
- `newMovementResponse` 直接影响流水列表和流水更新响应；风险为 LOW。
- `BuildInventoryWorkbook`、`InventoryPage`、`StockMovementsPage` 及各库存写入口为 LOW/MEDIUM。
- 共享查询 `salesReportBase` 为 HIGH，直接影响销售趋势、商品排行和店铺排行；编辑前已向用户说明，并通过统一公式和报表回归测试覆盖三个调用方。
- 最终 `gitnexus detect-changes` 因金额口径横跨 22 个文件、24 条库存/流水/报表/页面执行流而标记为 CRITICAL；逐项核对后均属于本需求确认的全链路切换，没有额外业务域。

## Price authority and compatibility

### Runtime source

所有公式只读取关联商品的当前默认价格：

```text
purchase = Product.DefaultPurchaseCents
sale     = Product.DefaultSaleCents

inbound_amount  = inbound_quantity × purchase
outbound_income = outbound_quantity × sale
outbound_cost   = outbound_quantity × purchase
gross_profit    = outbound_income − outbound_cost
adjustment      = quantity_delta × purchase
inventory_value = inventory_quantity × purchase
```

商品价格为 0 时公式自然得到 0，不阻断任何库存操作。

### Legacy persistence

现有数据库中的移动平均、操作单价和派生金额列暂时保留，以兼容现有表结构和旧版本回滚；它们不再是读取时的业务来源：

- 新建或修订流水时，可继续把由商品当前价格算出的值写入旧列作为兼容镜像。
- 库存、流水响应、修订预览、导出和报表每次读取都重新按商品当前价格投影，不能信任历史镜像值。
- 商品调价不批量更新历史流水，避免大范围写放大、更新时间污染和并发锁；下一次读取立即反映新价格。
- 本次不物理删除旧列；后续确认无回滚需求后可单独清理。

## Inventory transaction flow

`InventoryService` 在既有 product → snapshot 锁顺序内加载商品价格：

1. 入库、出库和调整请求只携带商品、数量及既有店铺/备注字段。
2. 锁定未归档商品并读取当前默认价格。
3. 纯转换函数按数量更新快照，并用当前进货价计算兼容金额镜像。
4. 在同一事务保存快照并追加流水。

最新流水修订继续采用“反转最新一笔 → 应用新数量”的模型，但反转只依赖 `QuantityDelta`：

- 先用当前商品进货价把当前快照金额标准化。
- 通过当前数量减去原流水 `QuantityDelta` 恢复修订前数量。
- 使用当前商品价格重新应用修订后的数量。
- 请求和严格 JSON 解码均移除 `unit_cents`；包含该字段的修订请求返回 `400 VALIDATION`。
- 预览保留数量、库存金额、收入、成本和毛利影响，删除单价及移动平均成本字段。

## HTTP contracts

### Inventory mutations

```json
POST /inventory/inbound
{"product_id":"...","shop_id":"...","quantity":10}

POST /inventory/sales-outbound
{"product_id":"...","shop_id":"...","quantity":2}

POST /inventory/adjustments
{"product_id":"...","quantity_delta":-1,"reason":"盘点"}
```

入库和销售出库请求类型删除价格字段及旧的驼峰兼容解码。Gin 对旧创建请求中的额外价格字段可忽略，但服务层从不读取它们。

### Inventory reads

- `GET /inventory` 继续返回分页库存快照及关联商品。
- 每条响应的价格派生字段在返回前用当前默认进货价重算；页面不再消费或显示移动平均成本与更新时间。
- 数据查询在分页前按 `products.name ASC, products.code ASC, inventory_snapshots.product_id ASC` 排序。
- `GET /inventory/export` 使用同一排序和当前商品价格；“移动平均成本”列改为“商品进货价”，库存金额实时计算。

### Movement reads and edits

- `GET /stock-movements` 新增可选 `from=YYYY-MM-DD` 与 `to=YYYY-MM-DD`。
- 后端用固定 UTC+8 的 `Asia/Shanghai` 边界解析日期，不依赖服务器本地时区。
- `from` 使用 `created_at >= 北京时间当日 00:00`；`to` 使用 `created_at < 北京时间次日 00:00`，两端日期均包含整天。
- 日期条件在分页前与类型、商品、店铺条件组合；排序仍为 `created_at DESC, id DESC`。
- `IsLatest` 继续基于该商品的完整流水历史判断，不受当前筛选范围影响。
- 流水响应的采购金额、收入、成本和毛利按关联商品当前价格重新投影。
- 修订请求仅接受数量/调整数量、店铺、备注、修改原因和版本；未知 `unit_cents` 被严格拒绝。

## Reports and dashboard

- 销售汇总、趋势、商品排行和店铺排行查询都关联 `products`，以销售数量乘当前默认销售价/进货价计算收入、成本和毛利。
- 日期分组、排行数量、流水数、归档商品和店铺维度保持现状。
- 仪表盘复用动态库存、流水和汇总响应，无需单独计算价格。
- 报表页面复用动态库存与报表响应，布局保持现状。

## Frontend

### Inventory cards

- 保留页面头部搜索、库存操作、导出、三项统计和低库存切换。
- 用 CSS Grid `repeat(auto-fill, minmax(...))` 与 Ant Design `Card` 替代 `Table`；无额外布局库。
- 卡片复用 `ProductIdentity`/`ProductImage`，显示图片、名称、编码、数量、库存金额和 `StockBadge`。
- 数量与状态共享同一库存状态判断：0 为红色，大于 0 且触发阈值为黄色，其余为绿色；文本确保不只依赖颜色。
- 使用 Ant Design `Pagination` 复用现有分页元数据，加载、错误和空状态保持可见。

### Inventory actions and movement editor

- 入库、销售出库表单删除单价状态、默认值、联动和输入框，请求不再传价格。
- 调整表单本来没有价格字段，保持数量与原因。
- 流水编辑器删除单价初值、字段、请求字段和预览行；数量、店铺、备注、原因及归档限制保持不变。
- 共享 TypeScript 类型同步删除已退出的修订单价/移动平均预览字段；保留页面仍需的派生金额字段。

### Date filter

- 流水筛选区增加 Ant Design `DatePicker.RangePicker`，不引入日期库。
- 选择或清除范围时更新 `from/to` 字符串并回到第一页；空值不发送参数。
- 四项筛选在桌面均衡排布，在窄屏按现有 `Row/Col` 规则换行。

## Error handling and edge cases

- 数量校验、负库存、归档商品、过期修订和权限错误沿用现有错误码。
- 价格为 0 是有效配置，不产生新的错误。
- 仅有起始或结束日期时支持单边筛选；空范围返回全部时间。
- UTC 日期交界处按北京时间归属，例如北京时间 7 月 1 日 00:00 对应 UTC 6 月 30 日 16:00。
- 低库存筛选和搜索为空时显示现有空状态；卡片分页越界继续由共享分页逻辑收敛。
- 历史归档商品仍通过关联商品最后保存的默认价格参与流水和报表计算。

## Rollout and rollback

- 无数据库迁移，现有列与数据保留。
- 部署需同时更新 API 与 Web；若验证失败，整体回滚两个进程到上一版本。
- 回滚后旧版本仍可读取遗留列；新版本写入的兼容镜像来自商品价格，但商品调价期间未批量刷新的旧流水仍会恢复旧版本的历史口径，这是已知回滚限制。

## Validation strategy

- 服务测试覆盖三种库存转换、零价格、负库存、归档和最新流水修订，确认价格只来自商品。
- 路由测试覆盖无价格请求、严格拒绝修订单价、动态流水金额和日期范围组合筛选。
- 报表测试在已有流水后修改商品价格，确认四类报表立即重算。
- 库存/导出测试覆盖全量排序、当前进货价、动态库存金额、低库存和图片。
- 前端执行 lint、严格类型检查、Vitest 与生产构建，并在真实浏览器验证响应式卡片、三级状态、无价格表单、修订预览和日期筛选。
