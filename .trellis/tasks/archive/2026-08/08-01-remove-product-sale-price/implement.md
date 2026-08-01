# 实施计划：移除商品销售价

## 1. 影响分析

- [x] 对 `models.Product`、`models.StockMovement`、`productFromForm`、`ProductHandler.List/Update`、`applySalesOutbound`、`priceMovement`、`calculateMovementUpdate`、`buildMovementEditResult`、四个报表处理器及相关页面函数执行 GitNexus upstream impact。
- [x] 对 HIGH/CRITICAL 结果先报告影响流程，再进入编辑。

## 2. 后端模型与库存服务

- [x] 从运行时模型移除商品销售价和流水售价/收入/毛利字段，保持生产旧列不删除。
- [x] 简化销售出库计算、当前价流水派生、流水修订复制/影响/审计字段，只保留采购成本。
- [x] 更新流水显式响应 DTO，避免模型兼容列意外重新暴露。
- [x] 保持库存锁顺序、负库存校验、版本校验和事务边界不变。

## 3. 商品与报表 API

- [x] 商品表单解析、更新映射、列表 summary 和响应去掉 `default_sale_cents`。
- [x] 四个报表改为数量、笔数和当前采购价估算成本，商品/店铺排行按数量稳定排序。
- [x] 保留原路由和权限键，更新 DTO 与测试契约。

## 4. Web 收敛

- [x] 从 `features/types.ts` 删除销售价、收入、毛利及相关 impact 字段。
- [x] 商品页删除售价表单、列、提交字段和售价汇总，改为总数/启用/禁用。
- [x] 流水页删除收入/毛利及其修订预览，只展示采购/成本金额，并把日期列移到成本列之后。
- [x] 仪表盘和报表改为销售数量、笔数、估算采购成本、库存价值和低库存。
- [x] 更新页面说明与标签，避免把估算成本写成收入或利润。

## 5. 自动验证

- [x] 更新 inventory、products、movements、reports、Lark 相关 Go 测试，保留溢出、库存不足和修订事务覆盖。
- [x] `cd apps/api && gofmt -w <changed-go-files> && go test ./... && go vet ./...`。
- [x] `cd apps/web && npm run lint && npx tsc --noEmit --incremental false && npm test && npm run build`。
- [x] `rg -n 'DefaultSaleCents|default_sale_cents|RevenueCents|revenue_cents|GrossProfitCents|gross_profit_cents' apps/api apps/web` 仅允许明确的迁移兼容注释；正常目标为零运行时匹配。

## 6. 手工验收与回滚点

- [x] 新建、编辑商品，确认页面及网络请求/响应无销售价。
- [x] 完成销售出库和最新流水修订，确认库存、数量和成本正确，页面无收入/毛利。
- [x] 检查仪表盘和报表的数量排行、趋势、成本说明和空状态。
- [x] 发布前确认生产旧列存在且有备份；回滚不执行反向数据迁移。
