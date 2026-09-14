# Personal API tokens for AI and external systems

## Goal

让已登录用户在个人设置里创建一把个人 API Token，把同一把 Token 交给远程 MCP，使任意能访问该网站的 AI 用中文自然语言代替用户执行首期库存操作。

## Background

网页认证仍是 Cookie 会话加修改请求同源校验，第三方 Token 曾被 `07-18-api-access-control` 和 `.trellis/spec/backend/http-contracts.md` 明确排除。现有库存接口和按用户权限已经能用。AI 需要可跨机器使用的凭证，以及自带中文说明的工具，这样不用读 API 文档也能查库存、入库、出库。

## Requirements

### R1. 每人一把 Token

- 每个登录用户最多一把 API Token。创建、查看、重新生成、撤销只要求有效网页会话，不要求 `setting.read` / `setting.update`。
- 不命名、不自动过期。重新生成立即作废旧值；撤销后该用户没有可用 Token。
- 原始 Token 只在创建或重新生成成功时展示一次。数据库只保存哈希、所属用户、创建时间和最后使用时间。
- 刷新设置页后只显示脱敏前缀和创建时间。禁用或删除账号后 Token 立即失效。

### R2. Token 只开放库存业务路由

Token 只能调用这些现有 `/api/v1` 接口，不另建库存实现：

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/products` | `product.read` |
| GET | `/shops` | `shop.read` |
| GET | `/inventory` | `inventory.read` |
| GET | `/stock-movements` | `movement.read` |
| POST | `/inventory/inbound` | `inventory.inbound` |
| POST | `/inventory/sales-outbound` | `inventory.sales_outbound` |
| POST | `/inventory/adjustments` | `inventory.adjust` |
| GET/POST/DELETE | `/mcp` | 见 R3，按工具检查权限 |

- Token 通过 `Authorization: Bearer` 认证；这类请求不走浏览器同源 Origin 校验。Cookie 会话的修改请求仍必须同源。
- Token 身份等于创建它的用户，继续用 `EffectivePermissions`。缺权限返回 `403`。不做 Token 级 scope。
- 即使持有者是 admin，Token 也不能调用白名单外的路由，包括用户/权限/备份/设置、商品写操作、流水编辑、库存导出和 Token 管理接口。
- 入库、出库、调整调用成功即记账，与网页相同，不增加确认或 dry-run。操作者是 Token 所属用户；审计须能区分 Token 与网页会话。飞书成功通知沿用现有 handler。

### R3. 远程 MCP，中文工具自描述

- 库存 API 提供 Streamable HTTP MCP，地址为站点 `/api/v1/mcp`。AI 客户端只配置 URL 和 Bearer Token，不必在每台电脑安装 MCP 程序。
- `/mcp` 只接受 API Token，不接受网页 Cookie，避免浏览器 CSRF 打到 MCP。
- MCP 在进程内调用现有查询和 `InventoryService`，不实现第二套库存规则，也不做本机 stdio 包装器。
- 每个工具带中文用途和参数说明。`tools/list` 只返回当前用户有权使用的工具。工具固定为：
  - `list_products`：按名称或编码搜商品
  - `list_shops`：列店铺或按名称找店铺
  - `get_inventory`：查库存，可按商品关键词或只看低库存
  - `list_stock_movements`：查流水，可按类型/商品/店铺/日期
  - `create_inbound`：入库（商品、数量，店铺可选，备注可选）
  - `create_sales_outbound`：销售出库（商品、店铺、数量，备注可选）
  - `create_adjustment`：库存调整（商品、增减数量、原因）
- 写工具按商品编码或名称、店铺名称查找。显式 ID → 编码精确匹配（不区分大小写）→ 名称精确匹配。0 或多于 1 条则拒绝写入并返回候选。
- 用户用自然语言即可，例如「绿茶还有多少」「给总店出库 2 件绿茶」。
- 设置页在展示新 Token 时提供可复制的远程 MCP 配置（URL + `Authorization: Bearer`）。

## Acceptance Criteria

- [ ] AC1：设置页创建 Token 后，可凭该 Token（不带 Cookie、不带网页 Origin）调用已授权的 R2 REST 接口。
- [ ] AC2：错误、已撤销或已重新生成的旧 Token 得到 `401`；有效 Token 但缺少对应业务权限得到 `403`。
- [ ] AC3：浏览器 Cookie 会话的修改请求仍必须同源；带伪造 Origin 的 Cookie 请求不能绕过 CSRF。
- [ ] AC4：完整 Token 只在创建或重新生成时可见一次；刷新后只显示脱敏前缀和创建时间。
- [ ] AC5：同一用户不能同时有两把有效 Token；撤销、重新生成、停用或删除用户后，旧 Token 立即失效。
- [ ] AC6：Token 或 MCP 发起的入库/出库/调整写入审计，actor 为所属用户，且能看出是 Token 调用。
- [ ] AC7：设置页覆盖无 Token、创建/重新生成成功展示、复制 Token、复制远程 MCP 配置、重新生成确认、撤销确认、加载和保存错误。
- [ ] AC8：用 URL + Token 连接 `/api/v1/mcp` 后，AI 能看到带中文说明的工具，并完成查找商品/店铺、查库存、查流水、入库、销售出库、库存调整。无对应权限的工具不出现。名称不唯一时拒绝写入并返回候选。
- [ ] AC9：admin 的 Token 调用 `/users`、`/permissions`、`/settings`、商品写接口或流水编辑时被拒绝。仅凭 Cookie 不能调用 `/mcp`。
- [ ] AC10：Go 测试与静态检查、Web lint / 类型检查 / 测试 / 构建全部通过。

## Out of Scope

- OAuth、JWT 登录、第三方应用注册、MCP OAuth 2.1 / Dynamic Client Registration。
- 本机 stdio MCP 二进制。
- 改变现有网页 Cookie 会话协议。
- 飞书机器人改造成 Token 客户端。
- 每用户多把 Token、自定义名称、自动过期、Token 级 scope。
- 商品新建/编辑/删除、流水编辑、库存导出、用户/权限/备份/系统设置对 Token 开放。
- 写操作二次确认或 dry-run。
