# 流水编辑与商品图片优先交互技术设计

## 1. 设计结论

- 使用“仅编辑该商品最新流水”限制，从当前快照反向扣除原流水已保存的效果，再重新应用一笔操作。
- 原 `stock_movements` 行原地更新，ID、原操作人和 `created_at` 不变；审计日志保存完整修订历史。
- 新增服务端预览与保存接口，共用同一套重算逻辑；保存在事务内重新做最新性、版本和库存校验。
- 抽取现有三种库存操作的状态转换逻辑，让新增与编辑共享计算，避免两套移动平均成本和财务公式漂移。
- 商品交互复用两个组件：直接改造现有 `ProductCombobox` 负责图片宫格选择，`ProductIdentity` 负责列表、表格和确认内容。
- 不引入依赖、不增加通用账本框架、不扫描或缓存全量历史流水。

## 2. 组件边界

### 后端

- `internal/models`：扩展 `StockMovement` 修订字段与最后编辑人关联。
- `internal/services/inventory.go`：集中三类状态转换；新增最新流水预览、反向恢复、保存和审计事务。
- `internal/services/permissions.go`：增加 `movement.update` 及其依赖。
- `internal/http/handlers/movements.go`：返回最新性，解析类型化更新请求，实现预览和保存。
- `internal/http/router.go`：显式给预览和保存路由绑定 `movement.update`。
- `internal/http/handlers/products.go`：新建商品强制主图，更新商品仍允许保留已有图片。
- `internal/http/handlers/reports.go`：商品排行返回当前图片路径。

### 前端

- `features/product-combobox.tsx`：保留现有受控值接口，内部改为图片宫格、搜索、选择、清空与缺图展示。
- `features/product-identity.tsx`：统一图片在前、名称/编码在后的商品身份展示。
- `features/product-image.tsx`：保留实际图片/失败回退，补充可见的待补图状态所需能力。
- `app/(app)/stock-movements/page.tsx`：编辑入口、类型化表单、影响预览、二次确认和修订信息。
- 现有库存操作表单、商品/库存/仪表盘/报表页面：替换文字选择或文字商品项。
- `lib/api.ts`：复用现有 `request`；不增加请求库。

## 3. 数据模型

`StockMovement` 增加：

| 字段 | 约束 | 用途 |
| --- | --- | --- |
| `revision` | `bigint not null default 1` | 乐观并发版本；每次成功编辑加一 |
| `updated_at` | 非空时间 | GORM 维护，修订后表示最后修改时间 |
| `last_edited_by_id` | 可空 UUID、索引、外键 | 最后一次编辑人；从未编辑时为空 |
| `last_edited_by` | 关联，不单独持久化 | 流水列表展示编辑人 |

列表响应额外提供服务端计算的 `IsLatest`。它不入库；编辑按钮由 `movement.update` 权限与 `IsLatest` 共同决定。`Revision == 1` 表示从未修改，历史迁移行默认版本为 1。

不修改商品图片数据模型。历史响应通过当前 `Product.ImagePath` 或排行查询中的当前 `products.image_path` 获取图片。

## 4. 权限目录与路由

- 新权限：`movement.update`，staff 可配置，依赖 `movement.read`；依赖闭包继续自动补齐 `product.read` 和 `shop.read`。
- 路由：

| 方法与路径 | 权限 | 行为 |
| --- | --- | --- |
| `GET /stock-movements` | `movement.read` | 返回分页流水、关联对象、修订信息和 `IsLatest` |
| `POST /stock-movements/:id/preview` | `movement.update` | 校验并返回影响，不写库 |
| `PATCH /stock-movements/:id` | `movement.update` | 事务性保存修订 |

管理员仍自动拥有全部目录权限。业务 handler 不自行判断角色。

## 5. HTTP 契约

### 更新请求

预览与保存使用同一 JSON 结构，前端按流水类型发送完整可编辑字段：

```json
{
  "expected_revision": 1,
  "quantity": 8,
  "quantity_delta": null,
  "unit_cents": 350,
  "shop_id": "uuid-or-null",
  "note": "业务备注",
  "change_reason": "录入数量错误"
}
```

- 入库/销售出库使用正数 `quantity` 与非负 `unit_cents`，`quantity_delta` 必须为空。
- 调整使用非零 `quantity_delta`，`quantity`/`unit_cents` 必须为空。
- 入库 `shop_id` 可空；销售出库必填；调整必须为空。
- `note` 对入库/出库可空、对调整必填，最长 500 字。
- `change_reason` 对所有编辑必填，最长 500 字。
- 不接受商品、类型、操作人、原始时间或任何派生金额字段。

### 预览响应

```json
{
  "before": {"quantity_delta": -3, "sale_unit_cents": 500},
  "after": {"quantity_delta": -4, "sale_unit_cents": 550},
  "impact": {
    "current_quantity": 7,
    "result_quantity": 6,
    "current_inventory_value_cents": 700,
    "result_inventory_value_cents": 600,
    "revenue_delta_cents": 700,
    "cost_delta_cents": 100,
    "gross_profit_delta_cents": 600
  },
  "expected_revision": 1
}
```

字段不适用时返回零，保持固定响应结构。预览不更新版本、不写审计；保存成功返回更新后的 `item` 和同一 `impact`。

### 错误矩阵

| 场景 | 状态/代码 |
| --- | --- |
| ID、字段组合、备注或修改原因非法 | `400 VALIDATION` |
| 流水不存在 | `404 MOVEMENT_NOT_FOUND` |
| 缺少编辑权限 | `403 FORBIDDEN` |
| 目标不再是最新或版本不匹配 | `409 MOVEMENT_STALE` |
| 新出库超过操作前库存 | `409 INSUFFICIENT_STOCK` |
| 调整后库存为负 | `409 INSUFFICIENT_STOCK` |
| 归档商品尝试数字修改 | `409 PRODUCT_ARCHIVED` |
| 状态恢复、保存或审计失败 | `500 INTERNAL`，事务回滚 |

## 6. 库存恢复与保存数据流

### 恢复目标操作前状态

1. 最新性约束保证当前快照就是目标流水执行后的状态，因此无需读取更早流水。
2. 入库用当前数量/金额减去原 `QuantityDelta`/`PurchaseAmountCents`；重新入库只依赖恢复后的数量和精确库存金额。
3. 销售出库用当前数量/金额加回原出库数量/`CostAmountCents`，并用原 `CostUnitCents` 恢复当时移动平均成本。
4. 调整用当前数量减去原 `QuantityDelta`，并使用原 `CostUnitCents`；调整转换本身会按该成本重算结果金额。
5. 用共享转换函数应用新输入，生成结果快照和全部派生字段；任何负数、溢出或状态不一致都拒绝保存。

该实现每次编辑为 O(1) 数据恢复。若未来允许编辑任意历史流水，再改为从检查点重放目标之后的流水；当前范围不预建该机制。

### 预览

1. 读取目标、商品和当前修订版本。
2. 确认目标是该商品最新流水。
3. 校验类型化字段、归档限制和结果库存。
4. 恢复目标前状态，应用新值，返回前后值与影响。
5. 不写任何数据库行。

### 保存

1. 先读取目标商品 ID，再开启事务。
2. 按现有库存写入一致的顺序锁定商品行，再锁定库存快照和目标流水。
3. 重新查询最新流水，比较 ID 和 `expected_revision`；任一变化返回 `MOVEMENT_STALE`。
4. 对数字编辑恢复目标前状态并应用新操作；元数据编辑保持快照原值。
5. 保存库存快照，更新原流水可编辑及派生字段，`revision + 1`，设置最后编辑人/时间。
6. 同一事务写 `movement.updated` 审计，metadata 为排序稳定的 `before`、`after`、`impact` 与 `change_reason`。
7. 提交后返回带 Product、Shop、Operator、LastEditedBy 和 `IsLatest=true` 的流水。

所有新增库存操作继续先锁商品行，因此保存编辑与并发新增会串行化；锁后最新性复核决定谁成功。

## 7. 共享状态转换

从 `CreateInbound`、`CreateSalesOutbound`、`CreateAdjustment` 提取最小的纯转换函数，输入“操作前快照 + 类型化输入”，输出“操作后快照 + 流水派生数字”。新增和编辑都调用它们：

- 入库：数量、采购金额、库存金额、移动平均成本。
- 出库：数量、成本单价/金额、收入、毛利；全量出库使用完整剩余库存金额。
- 调整：数量、当时成本、调整金额。

数据库锁、持久化和审计仍由各自服务方法负责，不创建 repository/interface/factory 层。

## 8. 前端交互

### 商品图片组件

- `ProductIdentity` 接收名称、编码、图片、归档状态和可选启用状态，组合 `ProductImage`、名称、编码和状态标签。
- `ProductCombobox` 使用 Ant Design `Popover`、`Input`、`Button`/`Flex` 实现可聚焦图片宫格，避免在库存操作 Modal 内再嵌套 Modal。
- 宫格图 88px（允许在 80–96px 范围内响应式调整）；普通身份块默认 44px。
- 搜索继续匹配名称和编码；选中、清空、无结果、缺图、归档和禁用状态都有文字表达。
- 删除旧文字下拉实现，保留现有纯函数搜索测试并扩充选择状态测试；不增加组件测试框架。

### 流水编辑

- 流水页读取 `movement.update`；没有权限时不显示编辑列。
- 最新流水显示编辑按钮，非最新显示禁用按钮和解释 Tooltip；归档商品打开表单时数字字段禁用。
- 表单按类型只渲染有效字段，商品身份块固定展示且不可切换。
- 数字或价格变化时先调用 preview，弹出影响确认；仅备注/店铺变化仍展示前后差异，但库存影响为零。
- 确认后 PATCH；`409 MOVEMENT_STALE` 保留表单并提示刷新，成功后刷新当前页及服务端数据。
- 行内显示“已修改”标签、最后编辑人和时间，同时保留原操作人和原时间。

### 全局图片优先

- 入库、出库、调整与流水筛选复用改造后的 `ProductCombobox`。
- 商品、库存、流水、仪表盘最近流水、低库存和商品排行改用 `ProductIdentity`。
- 商品删除确认框展示 `ProductIdentity`，不只在标题中写名称。
- 商品排行 DTO 增加 `product_image_path`，前端组合成 `ProductIdentity` 所需形状。
- 新建商品表单把图片移到醒目位置并做必填校验；后端仍是最终边界。编辑旧商品时没有替换图则保留原图，历史无图商品仍可保存其他字段。

## 9. 兼容、迁移与回滚

- AutoMigrate 只为 `stock_movements` 增加修订字段和索引；旧行 `revision=1`、最后编辑人为空，不重写业务数字。
- 新建商品从发布起要求图片；已有无图商品不回填数据库，以统一占位明确提示补图。
- 商品排行新增 JSON 字段对现有网页客户端为兼容扩展，API 与 Web 同批发布。
- 回滚代码时新增列可保留；未使用的权限行会被旧目录忽略。数据库不执行 DROP。
- 若流水编辑发布异常，可临时移除编辑路由与按钮，原新增库存流程和已保存流水仍可使用。

## 10. 关键取舍

- 最新一条限制换取可验证的 O(1) 反向恢复，避免扫描和重算后续多笔成本与报表。
- 原行更新让业务流水保持一条；审计日志承担不可变修订历史。
- 服务器预览多一次请求，但避免客户端复制库存财务公式。
- 任意历史编辑才需要重放或检查点；首版不提前增加快照表和缓存失效机制。
- 单主图与共享身份组件直接解决识别问题；多图、裁剪和视觉识别没有当前必要性。

## 11. 验证重点

- 三种类型的新增与编辑调用同一转换逻辑，旧测试与新修订测试同时通过。
- 编辑任何失败点都不留下快照、流水、版本或审计的部分更新。
- 并发新增/编辑通过商品锁和版本检查只允许一个结果提交。
- 预览与紧接着的保存对同一版本返回相同影响。
- 前端所有直接商品选择和主要商品展示都包含当前主图或“待补图”占位。
- 新建商品图片必填在 HTTP 边界有测试，不能只靠前端规则。
