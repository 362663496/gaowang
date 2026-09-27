# Personal API tokens：实现计划

## 顺序

1. **模型与 Token 服务**
   - `models.APIToken`；`db.Migrate` 加入 AutoMigrate。
   - `services.APITokenService`：生成 `gw_` Token、HMAC、按用户替换、查找启用用户、删除。
   - 单测：哈希找回、每用户一把、停用拒绝、与 session 隔离。
2. **中间件**
   - Bearer 跳过 Origin；`RequireAuth` 优先 Bearer。
   - 白名单常量含 REST 七条 + `/api/v1/mcp` 的 GET/POST/DELETE。
   - Cookie 访问 `/mcp` 拒绝。
3. **Token 管理与删除用户**
   - `GET/PUT/DELETE /api/v1/auth/api-token` 仅 Cookie。
   - 删除用户事务同时删 API Token。
   - `recordAudit` 合并 `auth_method` / `token_prefix`。
4. **远程 MCP**
   - 引入 `github.com/modelcontextprotocol/go-sdk`。
   - `handlers/mcp.go` 挂 Streamable HTTP；优先无状态 POST JSON。
   - 商品/店铺解析辅助 + 7 个中文说明工具；按权限过滤 `tools/list`。
   - 写工具走 `InventoryService`，成功后审计和飞书通知。
5. **后端测试**
   - Bearer 无 Origin 可入库；Cookie 跨源仍 403。
   - admin Token 调 `/users`、商品 PATCH、流水 PATCH、`/inventory/export` 为 403。
   - Cookie 调 `/mcp` 拒绝；Bearer 可 `tools/list`。
   - 无 `inventory.sales_outbound` 的用户看不到/不能调出库工具。
   - 名称不唯一的出库返回候选且不记账。
   - 零权限 staff 可管理 Token；`/auth/api-token` 加入账户路由例外。
6. **设置页**
   - `apiDelete`；`features/users/api-token.ts` 生成远程 MCP JSON。
   - 卡片：空态、一次性 secret、复制 URL+Token 配置、确认重新生成/撤销。
7. **文档**
   - README：Bearer 白名单、`/api/v1/mcp`、Cursor / Claude 远程 MCP 示例。
   - Phase 3 更新 `http-contracts.md` 与 directory-structure。

## 校验

```bash
gofmt -w <changed-go-files>
cd apps/api && go test ./... && go vet ./...
cd apps/web && npm run lint && npx tsc --noEmit --incremental false && npm test && npm run build
```

手工：登录 → 设置 → 创建 Token → curl 不带 Origin 调 `GET /products` 成功、调 `GET /users` 得 403 → 用 Token POST `/mcp` 列出工具 → 用中文商品名查库存 → 撤销后 401。

## 风险点

- `middleware.go` 的 `RequireAuth`、`RequireSameOrigin`：改前跑 GitNexus impact。
- `permissions_test.go` 全路由 403 枚举。
- MCP handler 必须从 Gin 上下文带上当前用户。
- 审计不得包含 secret 或 token_hash。
- Token 不能调用 `PUT /auth/api-token`。

## 回滚

- 代码回退即关闭 Bearer 与 `/mcp`；`api_tokens` 表可留。
- 前端卡片随代码消失，不影响改密和备份设置。
