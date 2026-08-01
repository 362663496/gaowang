# 实施计划：出入库可选备注

## 1. 影响分析

- [x] 对 `InboundInput`、`OutboundInput`、`InventoryService.CreateInbound/CreateSalesOutbound`、两个 handler 和两个前端表单执行 GitNexus upstream impact。
- [x] HIGH/CRITICAL 结果先向用户报告再编辑。

## 2. 实现

- [x] API 请求与服务输入增加可选 `note`，服务层在写事务前修剪并验证 500 字符上限。
- [x] 把规范化备注写入 `StockMovement.Reason`，非空时加入现有成功审计元数据。
- [x] 入库和销售出库弹窗增加可选备注 TextArea 并发送修剪值。
- [x] 保持调整原因必填、流水修订和现有展示逻辑不变。

## 3. 验证

- [x] 后端覆盖省略、空白、合法、500 边界、超限回滚以及调整回归。
- [x] 验证流水 API 与飞书卡片展示新备注。
- [x] `cd apps/api && gofmt -w <changed-go-files> && go test ./... && go vet ./...`。
- [x] `cd apps/web && npm run lint && npx tsc --noEmit --incremental false && npm test && npm run build`。

## 4. 回滚点

- [x] 无 schema 变更；切回旧应用即可，已写入 `reason` 的备注继续保留。
