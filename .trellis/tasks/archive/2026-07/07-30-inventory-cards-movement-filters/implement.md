# 库存卡片与流水筛选：实施计划

## 1. Pre-change gates

- [x] 加载 `trellis-before-dev` 及前后端相关规范。
- [x] 对将修改的每个函数/方法运行 GitNexus upstream impact；`salesReportBase` 为 HIGH，编辑前已向用户说明。
- [x] 识别并保留用户已有的 `AGENTS.md`、`.claude/`、`CLAUDE.md` 变更，本任务未覆盖这些文件。

## 2. Make product price the only runtime price

- [x] 修改 `apps/api/internal/services/inventory.go`：服务输入与修订输入移除操作单价，三个库存转换及最新流水修订统一读取锁定商品的当前默认价格。
- [x] 保留数量、负库存、归档、版本、事务和审计不变量；把旧金额列仅作为当前商品价格的兼容镜像。
- [x] 修订预览删除单价和移动平均成本契约，保留数量、库存金额、收入、成本、毛利变化。
- [x] 更新 `apps/api/internal/services/inventory_test.go`，覆盖零价格、商品调价、三种转换、修订、回滚和溢出保护。

Rollback point: 此步失败时只回退库存服务与对应测试，数据库无迁移。

## 3. Update API projections, reports, export, and filters

- [x] 修改 `apps/api/internal/http/handlers/inventory.go`：创建请求移除价格，库存读取按当前进货价投影，名称/编码稳定排序后分页，导出沿用同序。
- [x] 修改 `apps/api/internal/http/handlers/movements.go`：新增 `from/to` 的 `CreatedAt` 筛选，修订请求移除 `unit_cents`，流水金额按关联商品当前价格投影。
- [x] 修改 `apps/api/internal/http/handlers/reports.go`：汇总、趋势、商品排行和店铺排行按数量与商品当前价格计算。
- [x] 修改 `apps/api/internal/services/inventory_export.go`：导出显示商品进货价和动态库存金额，不再显示移动平均成本。
- [x] 更新 handler、report、movement、export 测试，覆盖动态调价、组合日期筛选、北京时间 UTC 交界、包含结束日、全量排序和旧价格字段退出。

Rollback point: API 契约与 Web 尚未发布前，可整体回退本节与第 2 节。

## 4. Replace the inventory table and remove price controls

- [x] 修改 `apps/web/src/features/inventory/action-forms.tsx`：删除入库/出库价格字段、转换和请求属性。
- [x] 修改 `apps/web/src/app/(app)/stock-movements/page.tsx`：删除修订单价字段/请求/预览，增加日期范围状态与筛选控件。
- [x] 修改 `apps/web/src/app/(app)/inventory/page.tsx`：使用响应式卡片网格、三级数量颜色、现有状态徽标和独立分页替代表格。
- [x] 修改 `apps/web/src/features/types.ts` 与最少量共享状态逻辑，保持 API 类型一致。
- [x] 在 `apps/web/src/styles/globals.css` 只增加卡片网格和数量状态所需样式；复用现有设计 token、图片和 Ant Design 组件。
- [x] 添加最小前端检查，确保零库存/低库存/正常状态判断不漂移。

Rollback point: Web 可与对应 API 变更一起整体回退；不保留新旧价格表单双轨。

## 5. Automated validation

- [x] `cd apps/api && gofmt -w <changed-go-files>`
- [x] `cd apps/api && go test ./...`
- [x] `cd apps/api && go vet ./...`
- [x] `cd apps/web && npm run lint`
- [x] `cd apps/web && npx tsc --noEmit --incremental false`
- [x] `cd apps/web && npm test`
- [x] `cd apps/web && npm run build`

## 6. Browser acceptance

- [x] 桌面宽屏验证卡片自动多列、排序、搜索/低库存/分页链路、图片回退和导出契约。
- [x] 500px 视口验证单列卡片、标题操作换行、数量/状态文本与可点击控件。
- [x] 验证入库、出库、调整及最新流水修订均无单价字段；零价格三类操作由服务回归测试覆盖。
- [x] 通过陈旧快照浏览器夹具及库存、流水、报表、导出测试验证商品调价后实时重算。
- [x] 验证流水日期范围按北京时间包含起止整天，可与类型/商品/店铺组合，清除后恢复全部记录；并修复清除时的空值异常。

## 7. Final gates

- [x] 运行 `npx gitnexus detect-changes -r gaowang`；22 个任务文件影响 24 条预期库存/流水/报表/页面执行流，整体标记 CRITICAL。
- [x] 加载 `trellis-check` 完成全量检查；功能门禁全部通过，另记录本任务未引入的 Next.js/sharp 依赖公告。
- [x] 对照 AC1–AC10 完成 PRD 收敛复核并勾选证据。
- [x] 更新相关库存规范。
- [ ] 提交代码并归档任务（等待用户决定提交）。
