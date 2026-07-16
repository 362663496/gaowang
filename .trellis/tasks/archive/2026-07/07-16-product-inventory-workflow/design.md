# 完善商品库存操作体验：技术设计

## Scope and invariants

- 库存快照继续按商品全局汇总；店铺只作为流水属性，不新增门店库存、调拨或门店成本。
- 库存流水不可修改或级联删除；商品归档不得改变历史销售额、成本、毛利和排行。
- 归档商品禁止新的入库、出库和调整；禁用商品沿用现有规则，不在本任务改变其语义。
- 复用现有 Gin、GORM、React 本地状态和 API 客户端；前端整体迁移到 Ant Design 6，不保留原生 `datalist`、Radix/Tailwind UI primitives 或第二套组件基础设施。

## Product archive model

在 `models.Product` 增加可空的 `ArchivedAt *time.Time`，由现有 `AutoMigrate` 添加 `products.archived_at`：

- 不使用 `gorm.DeletedAt`。GORM 的默认软删除作用域会让历史流水的 `Preload("Product")` 丢失归档商品；显式字段只在运营查询中按需过滤。
- 既有商品无需回填，`NULL` 即未归档。
- 归档同时把 `Enabled` 设为 `false`，保证回滚到旧版本时新库存操作仍不会选中该商品。
- 唯一商品编码保持不变并继续占用；归档图片保留供历史流水展示。

### Query behavior

- `GET /products` 默认增加 `archived_at IS NULL`；`GET /products?include_archived=true` 返回全部商品，仅供流水历史筛选。
- `GET /inventory` 关联商品并过滤 `products.archived_at IS NULL`，因此当前库存、库存品类、低库存、仪表盘库存指标和报表库存区块同步隐藏归档商品。
- `GET /stock-movements` 继续预加载完整商品和店铺；显式归档字段不会阻断历史关联。
- 销售汇总和趋势查询保持不变。商品排行继续聚合全部历史销售，并新增 `archived` 布尔字段供前端标注。

## Product deletion contract

`DELETE /api/v1/products/:id` 保持同一路由和 `204` 成功响应，内部在显式事务中完成判断：

```text
锁定商品行
  ├─ 不存在/已归档 → 404 PRODUCT_NOT_FOUND
  ├─ 当前库存 != 0 → 409 PRODUCT_HAS_STOCK
  ├─ 无库存快照且无流水 → 硬删除 → product.delete → 尽力清理图片
  └─ 有零库存快照或流水 → ArchivedAt=now, Enabled=false → product.archive
```

- 删除事务与库存事务都先锁定商品行，再锁定库存快照，避免删除检查与并发入库/调整之间产生归档后新增库存的竞态。
- 在 `InventoryService` 的三个写入口共用一个“锁定并确认商品未归档”检查；归档商品返回可识别错误并映射为 `409 PRODUCT_ARCHIVED`。
- 数据库现有 `OnDelete:RESTRICT` 继续作为硬删除的最终保护。
- 前端使用 Ant Design `Modal.confirm` 二次确认。失败时读取现有 JSON error envelope，把服务端 `message` 作为固定错误提示展示；失败不触发 reload，也不改变本地商品列表。
- 操作记录新增 `product.archive` 显示标签；硬删除继续使用 `product.delete`。

## Optional inbound shop

- `inboundRequest` 增加非必填 `shop_id`；空字符串不解析，非空值在 HTTP 边界校验 UUID。
- `services.InboundInput` 增加 `ShopID *uuid.UUID`，创建入库流水时直接写入既有可空字段。
- 入库库存数量、移动平均成本和金额计算不读取店铺，保持现有全局算法。
- 入库审计仅在选择店铺时增加 `shop_id` metadata。

## Frontend interactions

### Low-stock drill-down

- 当前库存页基于已加载的 `inventory` 计算一次低库存数组。
- 低库存汇总卡改为可聚焦按钮并用 `aria-pressed` 表示筛选状态；激活后表格接收低库存数组，并显示“显示全部”按钮。
- 不增加 API、路由、弹窗或重复表格。

### Product search

- 三个库存表单和流水筛选共用一个 Ant Design `Select` 商品组件，把搜索与选择合并为单个控件。
- 选项显示 `名称 · 编码`；`showSearch` 按名称和编码做不区分大小写的本地过滤，流水页归档选项追加“已归档”。
- 搜索和选择状态保持页面/表单本地；不增加远程搜索或缓存。

## Ant Design frontend platform

- 使用 Ant Design 6.5.1、`@ant-design/icons` 与 `@ant-design/nextjs-registry`。根布局由 `AntdRegistry` 注入 App Router 首屏样式，客户端 Provider 统一挂载中文 locale、`ConfigProvider` 主题和 `App` 消息上下文。
- 应用外壳使用 `Layout`、`Sider`、`Menu`、`Header` 和移动端 `Drawer`；导航项及管理员可见性沿用现有规则。
- 六个管理列表使用 `Table` 的 `pagination` 配置直连服务端 `PaginationMeta`；筛选变化仍回到第一页，页面大小保持 20，不增加客户端假分页。
- 业务表单使用 `Form`、`Input`、`InputNumber`、`Select`、`Upload` 和 `Modal`；表单内用 `Alert` 固定展示服务端错误，操作结果用 `App.useApp().message`。
- 商品和流水缩略图使用 Ant Design `Image`，无图使用框架 `Avatar`/图标占位；状态统一使用 `Tag`，加载、空数据和页面错误使用 `Spin`、`Empty`、`Alert`/`Result`。
- CSS 只保留页面网格、外壳尺寸和报表条形图等框架未提供的布局样式，视觉以 `#5e6ad2` 主色和紧凑 B2B 后台密度为准。
- 迁移完成后删除旧 `components/ui/*`、`use-message`、原生商品组合框及 Radix、Lucide、Tailwind、class variance/merge 依赖。

## Pagination contract

- 商品、店铺、当前库存、流水、操作记录和用户集合接口接受 `page`、`page_size`，默认 `1/20`，`page_size` 最大 100。
- 响应保持 `items` 并新增 `pagination: {page, page_size, total, total_pages}`；越界页收敛到最后一页。
- `all=true` 仅供商品/店铺/用户选项和仪表盘/报表完整数据使用，仍返回同一响应形状；真实列表页始终使用分页。
- 筛选参数在 Count 和数据查询中复用同一 GORM query；当前库存的 `low_stock=true` 使用既有阈值口径。
- 备份接口只返回最近一条记录，不属于集合分页；报表排行继续使用有界 `limit`。

## Product update contract

- 新增 `PATCH /api/v1/products/:id` multipart 接口，沿用创建表单字段；只允许修改未归档商品。
- 未上传 `image` 时保留原 `ImagePath`；上传新图片时先保存新文件，数据库更新失败则清理新文件，成功后尽力删除旧文件。
- 更新使用 `WHERE id = ? AND archived_at IS NULL` 防止与并发归档竞态；成功返回 `200 {"item": Product}` 并记录 `product.update`。

### Movement and report labels

- 流水商品单元格复用库存页的 40px Ant Design `Image`/`Avatar` 展示规则，并在归档商品名称旁显示状态徽标；图片文件缺失时也降级为占位图标。
- 商品排行读取新增的 `archived` 字段并标注“已归档”；金额与排序仍来自全部历史流水。
- 仪表盘最近流水不增加图片，但仍能通过历史关联显示归档商品名称。

## Compatibility, rollout, and rollback

- 新增列可空且无需数据回填；旧 API 二进制会忽略该列，因此数据库结构向后兼容。
- 新版本启动后由现有 `AutoMigrate` 添加列。发布前先生成数据库备份并记录当前 release。
- 发布到 `/opt/gaowang/releases/<timestamp>`，远端构建完成后原子切换 `/opt/gaowang/current`，再重启 `gaowang-api.service` 和 `gaowang-web.service`。
- 保留 `/opt/gaowang/shared/app.env`、uploads、backups 和 PostgreSQL 卷，不复制或删除共享数据。
- 失败时恢复旧软链并重启服务；新列无需回滚。旧版本会把已归档商品显示为“禁用”，但因 `Enabled=false` 不会出现在现有库存操作选择器中，历史数据仍安全。

## Validation strategy

- 路由/服务测试覆盖硬删除、零库存归档、有库存拒绝、归档后库存写入拒绝、默认/包含归档列表和运营库存过滤。
- 入库测试覆盖有店铺与无店铺两种流水，并确认全局库存计算不变。
- 报表测试覆盖归档前后历史汇总与排行不丢失、`archived` 标记正确。
- 前端保留最小商品选项纯函数测试；真实浏览器验证 Ant Design 外壳、服务端分页、低库存切换、四处搜索、错误提示、流水图片、归档标签和移动导航。
- 分页路由测试覆盖页码、总数、`all=true` 和筛选；商品路由测试覆盖字段修改、保留/替换图片和审计。
