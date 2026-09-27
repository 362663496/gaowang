# Personal API tokens：技术设计

## 1. 设计结论

- 网页会话协议不变。新增独立的 `api_tokens` 表和个人 Bearer Token；原始值只在创建/重新生成时返回一次。
- Token 认证复用现有用户权限和库存逻辑。新增的是认证方式、CSRF 例外、路由白名单，以及挂在同一 API 进程上的远程 MCP。
- MCP 使用官方 `github.com/modelcontextprotocol/go-sdk` 的 Streamable HTTP，地址 `/api/v1/mcp`。AI 客户端只配 URL + Bearer，不安装本地二进制。
- `/mcp` 只接受 API Token，拒绝 Cookie，避免网页 CSRF 打到工具调用。
- 改密不撤销 API Token。删除或停用用户必须立即失效。

## 2. 边界与组件

### 后端

- `internal/models`：新增 `APIToken`。
- `internal/db`：把 `APIToken` 加入 `AutoMigrate`。无数据回填。
- `internal/services/api_token.go`：生成、哈希、查找、每人一把的替换/删除。哈希算法与 session 相同：`HMAC-SHA256(AUTH_SECRET, raw)` 十六进制。
- `internal/services` 增加商品/店铺解析辅助（编码精确匹配 → 名称精确匹配），供 MCP 写工具使用。
- `internal/http/middleware.go`：
  - `RequireSameOrigin`：请求带 `Authorization: Bearer` 时跳过 Origin 校验；Cookie 会话路径保持现状。
  - `RequireAuth`：有 Bearer 则只走 Token 查找，忽略 Cookie；无效 Bearer 一律 `401`，即使 Cookie 有效。无 Bearer 则走现有 Cookie 会话。
  - `RejectAPITokenUnlessAllowlisted`：`auth_method=api_token` 且 `METHOD + FullPath` 不在白名单时 `403`。
  - `/mcp` 额外要求 `auth_method=api_token`，Cookie 会话得到 `401` 或 `403`。
- `internal/http/handlers/auth.go`：会话专属的 Token 管理接口。
- `internal/http/handlers/mcp.go`：把 Streamable HTTP handler 接到 Gin；按当前用户权限注册工具；写操作走 `InventoryService` 并 `recordAudit` + `Lark.Enqueue`。
- `internal/http/handlers/audit.go`：`recordAudit` 自动写入 `auth_method`（`session` / `api_token`）；Token 调用再带 `token_prefix`。
- `internal/http/handlers/users.go`：删除用户的同一事务里删除该用户 API Token。

### 前端

- `lib/api.ts`：补 `apiDelete`。
- `features/users/api-token.ts`：Token CRUD 和远程 MCP 配置 JSON（`url` + `headers.Authorization`）。
- `app/(app)/settings/page.tsx`：Token / AI 卡片。不改备份设置权限逻辑。

## 3. 数据模型

`api_tokens`

| 字段 | 约束 | 用途 |
| --- | --- | --- |
| `token_hash` | `varchar(64)` 主键 | HMAC 哈希，永不存原始 Token |
| `user_id` | UUID、唯一、非空、FK `users.id` ON DELETE CASCADE | 每人一把 |
| `token_prefix` | `varchar(12)` 非空 | 设置页脱敏展示，例如 `gw_ab12cd` |
| `created_at` | 非空 | 创建或最近一次重新生成时间 |
| `last_used_at` | 可空 | 认证成功后尽力更新 |

原始 Token：`gw_` + 32 字节 `base64.RawURLEncoding`。查找用完整原始值做 HMAC。

Session Cookie 与 API Token 分表。交叉投放应认证失败。

## 4. HTTP 合同

### 会话管理（仅 Cookie，不在 Token 白名单）

`GET /api/v1/auth/api-token` → `200 {"token": null | {"prefix","created_at","last_used_at"}}`

`PUT /api/v1/auth/api-token` → 创建或替换，`200 {"token":{"prefix","created_at"},"secret":"gw_..."}`。审计：`api_token.created` 或 `api_token.regenerated`。

`DELETE /api/v1/auth/api-token` → 无 Token 也 `204`。有 Token 时审计 `api_token.revoked`。

零权限 staff 可调用这三条。全路由 403 枚举测试须把它们算作账户路由。

### Token REST

```
Authorization: Bearer gw_...
```

不要求 `Origin`。白名单：

```
GET    /api/v1/products
GET    /api/v1/shops
GET    /api/v1/inventory
GET    /api/v1/stock-movements
POST   /api/v1/inventory/inbound
POST   /api/v1/inventory/sales-outbound
POST   /api/v1/inventory/adjustments
GET    /api/v1/mcp
POST   /api/v1/mcp
DELETE /api/v1/mcp
```

### 远程 MCP

`POST/GET/DELETE /api/v1/mcp` 为 Streamable HTTP。优先无会话、单次 POST 返回 JSON，避免 Nginx 会话亲和与 SSE 缓冲问题。若 SDK 需要 GET SSE，再视情况给 `/api/` 加 `proxy_buffering off`；首期尽量不改 Nginx。

客户端配置：

```json
{
  "mcpServers": {
    "gaowang": {
      "url": "https://<host>/api/v1/mcp",
      "headers": { "Authorization": "Bearer gw_..." }
    }
  }
}
```

## 5. 认证顺序

```
RequireSameOrigin
  GET/HEAD/OPTIONS → 放行
  Authorization 以 "Bearer " 开头 → 放行
  否则维持现有 Origin 校验

RequireAuth
  存在 Bearer → 只查 api_tokens；失败 401
  否则查 Cookie 会话

RejectAPITokenUnlessAllowlisted
  Cookie 访问 /mcp → 拒绝
  api_token 且路径不在白名单 → 403
```

## 6. MCP 工具

工具在进程内执行，不回环 HTTP。`tools/list` 按 `EffectivePermissions` 过滤。

| 工具 | 权限 | 行为 |
| --- | --- | --- |
| `list_products` | `product.read` | 按 `q` 查未归档商品 |
| `list_shops` | `shop.read` | 列出或按名称过滤店铺 |
| `get_inventory` | `inventory.read` | `q`、`low_stock` |
| `list_stock_movements` | `movement.read` | 现有 `type/product/shop/from/to` |
| `create_inbound` | `inventory.inbound` | 解析商品，店铺可选 |
| `create_sales_outbound` | `inventory.sales_outbound` | 解析商品 + 必填店铺 |
| `create_adjustment` | `inventory.adjust` | 解析商品；`reason` 必填 |

写工具入参同时接受 `product_id` / `product_code` / `product_query` 和 `shop_id` / `shop_query`。解析失败返回候选 `id/code/name`，不记账。成功路径与网页 handler 相同：`InventoryService` → 审计 → 飞书通知。

工具 `Description` 用中文写清何时使用、必填项、以及「绿茶还有多少 / 给总店出库 2 件」这类说法如何对应。

## 7. 设置页

卡片「API Token / AI」：

- 无 Token：创建。
- 有 Token：前缀、创建时间、最后使用；复制 MCP 配置；重新生成；撤销。
- 创建/重新生成成功：一次性展示 `secret` 和远程 MCP JSON（`url` 为当前 `origin + "/api/v1/mcp"`）。刷新后 secret 消失。
- 危险操作用 `modal.confirm`。

## 8. 兼容、发布与回滚

- 加表和加路由。未创建 Token 时行为与现在一致。
- 回滚应用后表可留。不改 Nginx，除非 SSE 被证实需要。
- README 增加 Bearer 与远程 MCP 配置示例。Phase 3 更新 `http-contracts.md`（网页不再是唯一认证客户端）。

## 9. 风险

- `RequireAuth` / `RequireSameOrigin` 是全局入口。覆盖 Cookie 同源、Cookie 跨源、Bearer 无 Origin、Bearer 非白名单、Cookie 打 `/mcp`。
- 白名单用 Gin `FullPath`。
- 审计不得写入原始 Token 或哈希。
- MCP SDK 挂到 Gin 时要带上当前用户，避免工具跑成空身份。
