# 移除商品销售价

## Goal

移除商品层面的默认销售价，并确保销售出库、报表与飞书统计不再依赖无业务意义的估算价格。

## Background

- `Product.DefaultSaleCents` 存在于模型、multipart 创建/更新接口、列表汇总和商品页（`apps/api/internal/models/models.go:97`、`apps/api/internal/http/handlers/products.go:34`、`apps/web/src/app/(app)/products/page.tsx:148`）。
- 销售出库目前用商品默认销售价计算并持久化 `SaleUnitCents`、`RevenueCents` 和 `GrossProfitCents`（`apps/api/internal/services/inventory.go:144`）。
- 销售报表又直接用当前商品默认销售价回算历史收入和毛利（`apps/api/internal/http/handlers/reports.go:17`），因此仅删表单字段会让业务结果失真。

## Requirements

- P1：商品创建、编辑、列表、类型和汇总不再包含默认销售价。
- P2：所有产品层 API 客户端不再提交或依赖 `default_sale_cents`。
- P3：不新增销售出库实际售价输入；停止收入和毛利统计，只保留销售数量、采购成本和库存价值等有可靠来源的指标。
- P4：既有流水中的收入/毛利数据库值为兼容与回滚保留，但运行时 API、报表、前端和飞书不再展示或依赖这些值。
- P5：生产迁移保留回滚路径，不破坏既有库存流水记录。
- P6：流水记录表移除收入和毛利列，将日期列放在采购/成本金额列之后。

## Acceptance Criteria

- [ ] 商品页不显示或提交销售价，商品接口响应不再包含该字段。
- [ ] 创建和更新商品无需 `default_sale_cents` 仍可成功。
- [ ] 全仓库运行时代码不再读取 `Product.DefaultSaleCents`。
- [ ] 新销售出库不再推导虚假收入或毛利；运行时仅展示数量、成本和库存价值指标。
- [ ] 历史流水保留可回滚数据，但用户可见接口不再混用历史收入/毛利。
- [ ] 数据库迁移前后均有可验证的部署和回滚步骤。
- [ ] 流水记录列顺序为业务身份与数量、采购/成本金额、日期、其余审计信息；收入和毛利不可见。

## Dependencies

- 无前置子任务。
- `08-01-fix-lark-query-recognition` 必须以本任务完成后的指标契约为准，不能重新引入销售额或毛利。

## Out of Scope

- 不新增实际售价、价格策略、折扣或订单系统。
