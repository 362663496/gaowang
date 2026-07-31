# 飞书机器人查询体验升级：实施计划

## 1. 配置与 DeepSeek 解析

- 在 `internal/config` 增加可选 `DEEPSEEK_API_KEY` 和默认 `DEEPSEEK_MODEL=deepseek-v4-flash`，补齐默认值、启停和密钥不泄漏测试。
- 新增标准库 DeepSeek 意图客户端：固定 endpoint、短超时、JSON Output、非思考模式、严格白名单校验。
- 用 `httptest.Server` 覆盖正常映射、非法 JSON、未知意图、空内容、非 2xx 和敏感错误信息。

## 2. 意图与查询

- 删除本地固定命令解析，所有非空文本统一调用 DeepSeek，并在最终意图确定后写审计。
- 扩展严格白名单，增加缺货清单、库存金额排行和今日销售排行。
- 将商品查询统一为 LEFT JOIN；实现库存概览、最近流水和上海当日变动的只读查询。
- 为命令、自然语言映射、查询排序/限制/空结果和聚合边界增加表驱动测试。

## 3. 卡片与图片

- 让单商品与多商品共享完整商品信息 Markdown。
- 将商品图片从全宽图片元素改为 `div.extra` 右侧紧凑缩略图，并保留点击预览。
- 构建最多 5 组“商品字段 + 右侧缩略图”的单卡片结果，组间增加分隔线。
- 流水结果按每条流水附带对应商品图片；缺图和单图上传失败保持文字回复。
- 库存操作卡片使用双列短字段和语义色标题，突出数量变化、库存结果和状态，弱化操作人、店铺与时间。
- 商品主信息区单独突出当前库存数量；删除售价字段，只保留采购价和按采购价计算的库存金额。
- 更新帮助、AI 降级、概览、今日变动、新排行/清单和流水卡片测试；断言商品图位于 `extra`，避免回归为全宽图片。

## 4. 文档与验证

- 更新 `.env.example`、`README.md` 和飞书后端规范，记录 DeepSeek Key、默认模型、命令和关闭 AI 的方式。
- 运行：
  - `gofmt -w <changed-go-files>`
  - `cd apps/api && go test ./...`
  - `cd apps/api && go vet ./...`
  - `npx gitnexus detect-changes --repo gaowang --scope all`
- 部署前只将 DeepSeek Key 写入服务器共享环境文件；重启 API 后验证健康、长连接、自然语言、图片、新排行/清单、错误降级和审计。

## 5. 风险与回滚点

- 修改现有解析、查询和卡片符号前逐一运行 GitNexus upstream impact；HIGH/CRITICAL 先停下告知用户。
- 不修改数据库结构，回滚无需迁移。
- AI 可通过清空 `DEEPSEEK_API_KEY` 独立关闭；图片/命令整体回滚使用上一 API release。
