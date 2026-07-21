# 流水编辑与商品图片优先交互实施计划

## 实施原则

- 单一跨层任务：流水编辑表单直接依赖新的图片商品选择/身份组件，API 与 Web 同批发布。
- 不增加 Go/npm 依赖，不引入 repository、通用账本或全局状态库。
- 先集中库存转换和后端事务，再接 UI；任一中间阶段都不单独部署。
- 每完成一个阶段运行最小检查，最后运行全量质量门。

## 1. 模型、迁移与权限

- [x] 扩展 `StockMovement`：`Revision`、`UpdatedAt`、`LastEditedByID`、`LastEditedBy`，增加商品+时间查询所需索引。
- [x] 确认 AutoMigrate 对旧行给出 `revision=1`，最后编辑人为空；测试 SQLite 迁移形状。
- [x] 权限目录增加 `movement.update`，staff 可配置且依赖 `movement.read`。
- [x] 更新权限闭包测试和路由覆盖测试，确保零权限 staff 对两个编辑路由均为 `403`。

检查点：`go test ./internal/services ./internal/http/...`。

## 2. 统一库存状态转换

- [x] 从三种 Create 方法提取最小纯转换函数；保留现有事务、锁和写入结构。
- [x] 入库转换计算数量、采购金额、库存金额和移动平均成本。
- [x] 出库转换校验库存并计算成本、收入和毛利，覆盖全量出库取整边界。
- [x] 调整转换校验非零、负库存和备注，按当时均价计算金额。
- [x] 让现有新增操作调用共享转换，现有行为测试保持通过。
- [x] 增加确定性转换测试，证明同一前置快照与输入得到稳定结果。

检查点：`go test ./internal/services -run 'Inventory|Movement'`。

## 3. 最新流水恢复、预览与保存服务

- [x] 实现从当前快照反向扣除最新流水已存效果的 helper，不扫描更早流水。
- [x] 实现最新流水查询和稳定的同时间 ID 次序。
- [x] 定义类型化编辑输入、影响 DTO 与稳定错误：not found、stale、insufficient、archived、validation。
- [x] 实现 preview：版本/最新性/字段/库存校验与影响计算，不写库。
- [x] 实现 update：商品→快照→流水锁序，锁后复核，更新快照/原流水/版本/最后编辑人，事务内写完整审计。
- [x] 元数据-only 分支不改库存或派生金额；归档商品只允许该分支。
- [x] 测试入库数量/单价、出库数量/售价、调整数量、店铺/备注、归档、库存不足和完整回滚。
- [x] 用顺序版本与新增流水模拟 stale；断言旧编辑失败。
- [x] 断言预览与保存影响一致，且预览不产生写入。

检查点：`go test ./internal/services/...`。

## 4. HTTP 契约与路由

- [x] 扩展流水列表：预加载最后编辑人并返回 `IsLatest`、Revision 与修订信息。
- [x] 在 `MovementHandler` 增加 preview 和 update；严格拒绝与目标类型无关的字段。
- [x] 把服务错误映射为设计中的稳定 JSON 错误码。
- [x] 在 `router.go` 给两条路由显式挂 `movement.update`。
- [x] 增加 route/service 测试：权限、请求矩阵、锁定字段不可提交、原时间/操作人不变、响应关联完整。
- [x] 审计元数据测试断言 before/after/impact/change_reason，且不包含密码、Cookie 或内部错误。

检查点：`gofmt -w <changed-go-files> && go test ./internal/http/... ./internal/services/... && go vet ./...`。

## 5. 新建商品图片必填

- [x] 后端 Create 对缺少 `image` 返回 `400 VALIDATION` 或稳定图片必填码；Update 无新文件时继续保留已有图片。
- [x] 图片保存成功而商品写入失败时清理文件，保持现有回滚行为。
- [x] 新增 handler 测试：缺图、合法图、非法类型、超限及数据库失败清理。
- [x] 前端新增表单把图片设为必填并显示预览；编辑表单维持替换可选。
- [x] 历史无图商品编辑其他字段不被图片必填规则拦截。

检查点：`go test ./internal/http/handlers -run Product && npm test -- src/features/product*`。

## 6. 图片商品选择器与身份组件

- [x] 新建 `ProductIdentity`，复用 `ProductImage`，统一名称、编码、归档/禁用/待补图文字。
- [x] 直接改造 `ProductCombobox`：AntD Popover 图片宫格、搜索、选择、清空、无结果、禁用和响应式宽度。
- [x] 保留 `product-options` 名称/编码搜索纯函数测试；状态展示由组件直接负责，不增加渲染测试框架。
- [x] 让库存三种操作与流水筛选继续复用改造后的 `ProductCombobox`。
- [x] 确保图块和触发器可键盘聚焦，有选中状态文本/aria，颜色不是唯一信息。

检查点：`npm test -- src/features/product-combobox.test.ts && npx tsc --noEmit --incremental false`。

## 7. 全局商品图片优先

- [x] 商品、库存和流水表格改用 `ProductIdentity`；保留表格横向滚动。
- [x] 仪表盘最近流水、低库存列表和报表低库存改用 `ProductIdentity`。
- [x] 商品排行后端增加 `product_image_path` 并更新 Group/DTO/测试；前端排行显示 40–48px 当前主图。
- [x] 商品删除确认内容加入图片身份块。
- [x] 搜索所有纯文字商品展示，逐处判断并替换直接商品项；审计原始资源 ID 不伪装成商品选择项。
- [x] 图片加载失败和历史无图在每种展示密度下都有可读占位。

检查点：`rg -n 'Product\.Name|product\.Name|product_name' apps/web/src --glob '*.tsx'` 人工审查剩余项；运行 lint/type/test。

## 8. 流水编辑界面

- [x] 更新 `StockMovement` 类型：修订版本、最后编辑人/时间、最新标记。
- [x] 流水页按 `movement.update` 控制操作列；非最新按钮禁用并带 Tooltip。
- [x] 新建页面内类型化编辑表单；商品用固定 `ProductIdentity`，商品/类型/原人/原时间不可编辑。
- [x] 店铺和备注遵循三种类型规则；归档商品数字输入禁用。
- [x] 调用 preview 展示前后值与库存/财务影响，数字保存前二次确认。
- [x] PATCH 携带 `expected_revision` 和必填修改原因；处理 `MOVEMENT_STALE` 时保留输入并提供刷新。
- [x] 成功后关闭表单、提示、重载当前页；行内显示已修改、编辑人和编辑时间。
- [x] 防止 preview/save 双击，按钮具备 loading/disabled 状态，错误保留在表单内。

检查点：`npm run lint && npx tsc --noEmit --incremental false && npm test`。

## 9. 文档与最终验证

- [x] 更新任务 PRD/设计中任何实施发现，移除已解决问题。
- [x] 用 `trellis-update-spec` 记录流水修订契约、图片必填与图片优先组件规范。
- [x] 检查 README；当前仅承担安装/部署说明，不写操作员功能文档，因此无需改动。

后端（`apps/api`）：

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

前端（`apps/web`）：

```bash
npm run lint
npx tsc --noEmit --incremental false
npm test
npm run build
npm audit --omit=dev
```

根目录：

```bash
docker compose config
git diff --check
```

人工验收：

- [ ] admin 与获授权 staff 分别编辑三种最新流水，结果与影响预览一致。
- [ ] 非最新、并发过期、库存不足和归档数字编辑均保留原状态。
- [ ] 仅备注/店铺编辑不改变任何库存或财务数字。
- [ ] 历史报表按原日期反映修正后的销售结果。
- [ ] 新商品无图提交被阻止；旧无图商品仍可选并清晰标为待补图。
- [ ] 桌面和 500px 宽度下，图片宫格、表格、列表、确认框和编辑表单可用。
- [ ] 商品换图后，流水、仪表盘和报表使用新图。

## 风险与回滚点

| 风险位置 | 风险 | 控制/回滚 |
| --- | --- | --- |
| `inventory.go` 共享转换 | 重构改变原新增库存结果 | 先让现有测试覆盖共享函数，再增加编辑；失败时回退该阶段 |
| 流水反向恢复 | 旧数据或取整规则不一致 | 使用已保存采购/成本金额和成本单价，增加多步序列测试；发现不一致时拒绝编辑并保留原状态 |
| 并发锁 | 新增与编辑互相覆盖或死锁 | 沿用商品→快照锁顺序，锁后复核版本/最新 ID |
| 审计事务 | 审计失败但业务已改 | 审计与快照/流水同事务，任一步失败整体回滚 |
| 新建图片必填 | 发布后操作员没有可上传图片 | 上线前告知；旧商品不受影响；回滚只需恢复 Create 的可选规则 |
| 全局组件替换 | 小屏选择器或表格回归 | 保留 AntD 原语、横向滚动和 500px 人工验收 |
| API/Web 契约 | 旧前端缺少新字段/入口 | API 与 Web 同批发布；新增响应字段保持向后兼容 |

数据库回滚不删除新增列。若编辑功能异常，先隐藏/移除 preview 和 PATCH 路由及按钮，保留新列、权限行和已有审计记录。

## 开始实施前复核

- [x] 用户已确认本需求访谈结论。
- [x] 用户已评审 `prd.md`、`design.md`、`implement.md` 并明确批准开始实施。
- [x] 批准后运行 `task.py start movement-edit-image-first`。
- [x] Phase 2 开始时运行 `trellis-before-dev` 重新加载 backend/frontend 规范。

## 自动化验证记录（2026-07-22）

- [x] 后端：`go test ./...`、`go vet ./...`。
- [x] 前端：lint、严格 TypeScript、18 个 Vitest、生产构建、`npm audit --omit=dev`（0 漏洞）。
- [x] 根目录：`docker compose config --quiet`、`git diff --check`、未合并冲突检查。
- [ ] 浏览器人工验收：自动化与线上 HTTP 冒烟已通过；完整登录态业务交互仍按下方清单人工执行。

## 阿里云发布记录（2026-07-22）

- [x] 功能提交 `81ed7ae` 已推送到 `origin/master`。
- [x] 发布前数据库备份已生成并通过 `gzip -t`。
- [x] API 与 Web 均在本地构建为 linux/amd64 成品；Web standalone 本地 `/login` 冒烟为 `200`。
- [x] 服务器仅校验、解压、切换和重启，没有执行源码构建。
- [x] 原子发布 `/opt/gaowang/releases/20260722013700`，回滚点为 `/opt/gaowang/releases/20260720162414`。
- [x] API/Web active，健康、登录、静态资源、上传图片与新增数据库列检查通过。
