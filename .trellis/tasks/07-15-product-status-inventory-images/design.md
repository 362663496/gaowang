# 商品状态管理与库存图片：技术设计

## Scope and invariants

- 复用现有 `Product.Enabled`、`Product.ImagePath` 和 `InventorySnapshot.Product`，不新增模型、迁移或前端数据类型。
- 商品状态与删除接口沿用现有商品创建接口的认证边界：所有已登录用户可用，不新增权限分支。
- 禁用只改变 `products.enabled`；商品列表、库存快照和库存操作仍按现有方式返回和工作。
- 库存快照、流水与财务历史不可因删除商品而丢失。

## API contracts

### 更新商品状态

`PATCH /api/v1/products/:id/enabled`

Request:

```json
{"enabled": false}
```

- `enabled` 必须显式提供布尔值。
- 成功返回 `200` 和更新后的 `{"item": Product}`。
- 非法 ID 或请求体返回 `400`；商品不存在返回 `404 PRODUCT_NOT_FOUND`；数据库更新失败返回 `500`。
- 成功后写入 `product.enable` 或 `product.disable` 审计记录。

### 删除商品

`DELETE /api/v1/products/:id`

- 先加载商品，再分别检查 `inventory_snapshots` 与 `stock_movements` 的引用。
- 只允许删除从未产生库存快照或流水的商品。
- 成功返回 `204` 并写入 `product.delete` 审计记录。
- 商品不存在返回 `404 PRODUCT_NOT_FOUND`；有关联记录返回 `409 PRODUCT_IN_USE`，提示改为禁用；查询或删除失败返回 `500`。
- 数据库删除成功后，在已配置上传目录且路径存在时，使用安全文件名做一次尽力而为的图片清理；图片清理失败不恢复已删除的数据库记录。

## Frontend behavior

### 商品列表

- 复用 `Badge`：启用使用 `success`，禁用使用 `error`，并始终保留文字标签。
- 增加“操作”列：当前状态的反向操作（启用/禁用）和删除。
- 直接复用底层 `request` 发起 `PATCH`/`DELETE`，不为两个调用增加新的 API 包装层。
- 商品操作期间统一锁定操作按钮，防止并发请求覆盖单一加载状态；成功后就地更新或移除页面状态，使汇总数量同步变化。
- 删除使用浏览器原生 `window.confirm`，避免引入额外对话框状态；失败信息通过现有 `ErrorBlock` 展示。

### 当前库存

- `/inventory` 已通过 GORM `Preload("Product")` 返回 `Product.ImagePath`，不新增 API 请求。
- 商品单元格在名称前渲染约 40px 的 `next/image` 缩略图；无图时复用 `ImageIcon` 语义和现有边框/背景样式。
- 不增加图片预览交互。

## Data flow

```text
商品页操作 → PATCH/DELETE → Gin ProductHandler → PostgreSQL → 审计日志 → 页面局部状态

PostgreSQL InventorySnapshot + Product.ImagePath
  → GET /inventory（既有 Preload）
  → InventorySnapshot TypeScript 类型（既有）
  → 当前库存商品单元格缩略图
```

## Compatibility and rollout

- API 只新增路由，既有调用不变；序列化字段与数据库结构不变。
- 版本发布到 `/opt/gaowang/releases/<timestamp>`，成功构建后原子切换 `/opt/gaowang/current`，再重启两个 systemd 服务。
- `/opt/gaowang/shared/app.env`、`shared/uploads`、`shared/backups` 和 `gaowang_postgres_data` 不进入版本目录、不复制、不删除。
- 发布失败时把 `current` 恢复到部署前软链目标并重启服务；本次无数据库迁移，不需要数据库回滚。
