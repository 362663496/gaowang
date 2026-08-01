# 技术设计：出入库可选备注

## 数据流

```text
入库/销售出库 TextArea
  → JSON note
  → handler request
  → InboundInput/OutboundInput.Note
  → 服务层规范化与长度校验
  → StockMovement.Reason
```

复用现有 `Reason` 列，不新增数据库字段。省略字段时 Go 零值为空字符串，保持旧客户端兼容。

## 后端契约

- 两个请求结构增加 `note`，可省略，边界最大 500 字符。
- 两个服务输入增加 `Note`；服务层统一 `TrimSpace`，再校验 rune 数不超过 500。
- 校验在任何库存写入前完成；成功后把规范化文本赋给 `movement.Reason`。
- handler 使用服务返回的规范化 `change.Movement.Reason`，仅在非空时写入审计 metadata。
- 调整的 `reason` 字段、必填规则和现有流水修订 `note` 契约不变。

## 前端契约

- `InboundValues`、`OutboundValues` 增加可选 `note`。
- 两个弹窗在数量之后增加 Ant Design `Input.TextArea`，标签为“备注（可选）”，`maxLength=500`、`showCount`、三行。
- 提交时发送修剪后的字符串；关闭弹窗沿用现有 `form.resetFields()` 清理备注。

## 展示与回滚

流水页面和飞书现有渲染已经读取 `Reason`，无需新展示组件。回滚无 schema 处理；旧版本会保留数据库备注，只是不在创建表单中录入。
