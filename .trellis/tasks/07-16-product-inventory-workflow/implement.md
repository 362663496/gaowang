# 完善商品库存操作体验：实施计划

## 1. Backend archive and query contracts

- [x] 在 `apps/api/internal/models/models.go` 为 `Product` 增加可空 `ArchivedAt`；复用现有 `AutoMigrate`，不新增迁移框架。
- [x] 在商品列表增加默认排除归档和 `include_archived=true` 历史查询；状态修改只作用于未归档商品。
- [x] 把商品删除改为事务内的硬删除/零库存归档/有库存拒绝三分支，并记录 `product.delete` 或 `product.archive`；硬删除才清理图片。
- [x] 在 `InventoryService` 三个库存写入口统一锁定并校验商品未归档，再锁库存快照，消除并发竞态；映射稳定的归档商品错误。
- [x] 当前库存查询排除归档商品；流水预加载保持包含归档商品。
- [x] 商品排行增加归档标记但不排除历史销售，销售汇总、趋势、成本和毛利口径保持不变。
- [x] 扩展商品生命周期、库存和报表测试，覆盖 AC1、AC2、AC3 的后端状态和持久化结果。

## 2. Optional inbound shop

- [x] 入库请求增加可选 `shop_id`，非空时校验 UUID；服务输入和入库流水写入既有可空 `ShopID`。
- [x] 入库审计在已选择店铺时记录 `shop_id`，未选择时不写伪值。
- [x] 扩展库存服务/路由测试，分别断言有店铺、无店铺流水及全局库存数量和成本。

## 3. Frontend behavior

- [x] 更新 `Product`、商品排行等 TypeScript 契约；商品页删除失败使用可见的错误提示展示服务端消息，成功后保持现有局部移除。
- [x] 当前库存低库存卡支持原地筛选和“显示全部”，复用现有低库存口径与表格。
- [x] 三个库存表单的共享商品选择器增加名称/编码搜索；流水筛选增加相同搜索并加载、标注归档商品。
- [x] 入库表单增加默认空、非必填店铺选择并发送可选 `shop_id`。
- [x] 流水商品列增加 40px 图片/占位与归档徽标；商品排行标注归档，仪表盘图片保持不变。
- [x] 更新审计动作标签，并为商品匹配逻辑保留一个最小单元测试。

## 4. Local quality gate

- [x] `gofmt -w` 所有改动的 Go 文件。
- [x] 后端：`cd apps/api && go test ./... && go vet ./...`。
- [x] 前端：`cd apps/web && npm run lint && npx tsc --noEmit --incremental false && npm test && npm run build`。
- [x] 运行 `git diff --check`，检查完整 diff、API/TypeScript 字段、历史财务口径和无关文件。
- [x] 使用真实浏览器验证商品删除错误、低库存筛选、四处商品搜索、可选入库店铺、流水图片和归档标签。

## 5. Release to `ssh aliyun`

- [x] 记录部署前 `/opt/gaowang/current`、服务状态和健康检查；在 schema 变更前生成并核验数据库备份。
- [x] 本地交叉编译 Linux amd64 API；上传 API 与排除 `.next`/`node_modules` 的 Web 源码到新 release。
- [x] 在服务器用现有 Node/npm 构建 Web standalone；构建成功前不切换 `current`。
- [x] 原子切换软链，重启 API/Web 服务，让 API 启动执行可空列迁移。
- [x] 验证两个服务 active、API/Nginx 健康、登录及五项相关页面可访问、上传图片可读取、`archived_at` 已存在且服务日志无新增错误。
- [x] 做非污染性线上冒烟：只创建并硬删除一个未使用临时商品；归档、入库店铺和历史财务分支依赖本地自动化测试与线上只读 UI/API 检查，不向生产流水写测试数据。
- [x] 若检查失败，恢复旧 release 软链并重启；保留新增可空列和备份，不触碰共享上传、备份及 PostgreSQL 卷。

## 6. Pagination and product editing

- [x] 新增共享分页参数/响应 helper，并让商品、店铺、当前库存、流水、操作记录和用户接口复用；保留 `all=true` 选项数据通道。
- [x] 当前库存支持同口径 `low_stock=true` 分页筛选；仪表盘和报表显式读取完整库存，最近流水只读取前 8 条。
- [x] 新增 `PATCH /products/:id` multipart 修改接口，保留或替换图片并记录 `product.update`；补充路由测试。
- [x] 新增共享前端分页控件，接入六个管理列表并在筛选变化时重置页码。
- [x] 用单一 `ProductCombobox` 替换四处“搜索框 + select”，继续复用商品匹配逻辑且不增加依赖。
- [x] 商品页增加修改弹窗并复用创建表单；修改成功刷新当前页，失败展示服务端消息。
- [x] 重新执行全量检查、浏览器验收和 aliyun 原子发布，确认分页、下拉搜索、商品修改及旧功能正常。

## Risk and rollback points

- 商品归档与库存写入必须按相同“商品行 → 库存快照”顺序加锁，否则并发操作可能在归档后留下库存。
- 历史报表查询不得加 `archived_at IS NULL`；只有运营商品/库存查询过滤归档。
- 归档保留图片，硬删除才清理图片，避免流水图片失效。
- 回滚旧版本后归档商品可能在运营列表中显示为禁用，但不会丢历史；重新发布新版本后恢复正确隐藏。
