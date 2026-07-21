# API 访问控制技术设计

## 1. 设计结论

- 认证采用数据库会话：浏览器只保存随机 `HttpOnly` Cookie，数据库保存 HMAC 哈希，不使用 JWT。
- 授权采用代码内固定权限目录 + `staff_permissions` 授权行；不引入 Casbin、OPA、角色表或用户级覆盖。
- `admin` 永远拥有全部权限；数据库只持久化 `staff` 的显式授权。空表即 `staff` 全部拒绝，新权限也天然默认拒绝。
- 每个业务路由在 `router.go` 显式绑定一个业务权限键，统一中间件完成判断，handler 不包含角色分支。
- 后端权限目录同时提供依赖关系和界面元数据，前端权限矩阵不复制依赖规则。
- 不拆分子任务：会话、路由授权和前端登录态不向后兼容，必须作为一个跨层变更按顺序实现和一起发布。

## 2. 边界与组件

### 后端

- `internal/models`：新增 `Session`、`StaffPermission` 两个持久化模型。
- `internal/config`：读取初始管理员字段与 `SESSION_COOKIE_SECURE`；后者作为生产 HTTPS 的显式安全开关。
- `internal/services`：新增会话令牌、首个管理员引导、权限目录/依赖闭包等可复用安全逻辑。
- `internal/http/middleware.go`：认证 Cookie、加载当前数据库用户与 `staff` 权限、执行权限判断、校验修改请求来源。
- `internal/http/router.go`：保留公开、仅登录、业务授权三类路由；所有业务路由显式挂载权限中间件。
- `internal/http/handlers/auth.go`：登录、当前用户、退出、改密。
- `internal/http/handlers/permissions.go`：读取权限目录和原子替换 `staff` 权限。

### 前端

- `lib/api.ts`：继续作为唯一请求入口，只发送 Cookie；集中处理 `401` 与 `403`。
- `AppShell`：加载 `/auth/me`，持有当前用户与权限，监听窗口聚焦和 `403` 事件并刷新。
- 各页面：根据同一会话权限隐藏菜单、功能块和动作按钮；页面本身不决定后端是否放行。
- 新权限页：使用 Ant Design 表格展示角色 × 权限矩阵，一次保存整套 `staff` 权限。

## 3. 数据模型

### `sessions`

| 字段 | 约束 | 用途 |
| --- | --- | --- |
| `token_hash` | `varchar(64)` 主键 | `HMAC-SHA256(AUTH_SECRET, raw_token)` 的十六进制值 |
| `user_id` | UUID、非空、索引、外键 | 关联 `users.id` |
| `expires_at` | 非空、索引 | 固定登录时间 + 7 天，不滑动续期 |
| `created_at` | 非空 | 会话审计时间 |

原始令牌由 `crypto/rand` 生成 32 字节并以无填充 Base64URL 编码。数据库从不保存原始令牌；删除用户时会话级联删除。

### `staff_permissions`

| 字段 | 约束 | 用途 |
| --- | --- | --- |
| `permission` | `varchar(64)` 主键 | 授予所有 `staff` 的稳定业务权限键 |
| `created_at` | 非空 | 授权写入时间 |

不建立权限表或角色表，也不保存恒定的 `staff` 角色列：权限目录随代码版本发布，数据库只保存这一套共享授权。未知、已删除和 `admin` 专属的数据库行在读取时不产生权限。

## 4. 权限目录与依赖

| 模块 | 权限 | `staff` 可配置 | 依赖 |
| --- | --- | --- | --- |
| 商品 | `product.read` | 是 | — |
| 商品 | `product.create` | 是 | `product.read` |
| 商品 | `product.update` | 是 | `product.read` |
| 商品 | `product.toggle` | 是 | `product.read` |
| 商品 | `product.delete` | 是 | `product.read` |
| 店铺 | `shop.read` | 是 | — |
| 店铺 | `shop.create` | 是 | `shop.read` |
| 库存 | `inventory.read` | 是 | `product.read` |
| 库存 | `inventory.inbound` | 是 | `inventory.read`, `shop.read` |
| 库存 | `inventory.sales_outbound` | 是 | `inventory.read`, `shop.read` |
| 库存 | `inventory.adjust` | 是 | `inventory.read` |
| 流水 | `movement.read` | 是 | `product.read`, `shop.read` |
| 报表 | `report.sales_summary` | 是 | — |
| 报表 | `report.sales_trend` | 是 | — |
| 报表 | `report.product_ranking` | 是 | — |
| 报表 | `report.shop_ranking` | 是 | — |
| 审计 | `audit.read` | 是 | — |
| 备份 | `backup.read` | 是 | — |
| 备份 | `backup.run` | 是 | `backup.read` |
| 设置 | `setting.read` | 是 | — |
| 设置 | `setting.update` | 是 | `setting.read` |
| 用户 | `user.read` | 否 | — |
| 用户 | `user.create` | 否 | `user.read` |
| 权限 | `permission.read` | 否 | — |
| 权限 | `permission.update` | 否 | `permission.read` |

依赖按传递闭包计算。例如授予 `inventory.inbound` 会同时授予 `inventory.read`、`product.read` 和 `shop.read`。前端取消基础权限时删除所有传递依赖项；后端保存时再次闭包，保证绕过前端也不能形成残缺组合。

## 5. HTTP 契约

### 认证接口

| 方法与路径 | 访问条件 | 行为 |
| --- | --- | --- |
| `POST /api/v1/auth/login` | 公开、同源 | 校验账号后创建会话，设置 Cookie，返回当前用户与权限 |
| `GET /api/v1/auth/me` | 有效会话 | 返回数据库中的当前用户与有效权限 |
| `POST /api/v1/auth/logout` | 有效会话 | 删除当前会话、清 Cookie，返回 `204` |
| `POST /api/v1/auth/password` | 有效会话 | 校验旧密码，更新密码，删除该用户全部会话并清 Cookie，返回 `204` |

登录与当前用户响应：

```json
{
  "user": {"id": "uuid", "name": "张三", "email": "a@example.com", "role": "staff"},
  "permissions": ["product.read"]
}
```

### 权限管理接口

| 方法与路径 | 权限 | 行为 |
| --- | --- | --- |
| `GET /api/v1/permissions` | `permission.read` | 返回固定目录和当前 `staff` 权限 |
| `PUT /api/v1/permissions` | `permission.update` | 原子替换整套 `staff` 权限并返回最新结果 |

读取/保存响应：

```json
{
  "catalog": [
    {
      "key": "product.read",
      "module": "product",
      "module_label": "商品",
      "action_label": "查看",
      "staff_assignable": true,
      "requires": []
    }
  ],
  "staff_permissions": ["product.read"]
}
```

保存请求为 `{"permissions":["product.create"]}`。服务端拒绝未知键和 `admin` 专属键，去重并补齐依赖。替换授权行和写入 `permission.updated` 审计记录处于同一数据库事务；审计元数据只包含排序后的 `before`、`after` 权限数组。

### 业务路由映射

| 路由 | 权限 |
| --- | --- |
| `GET /products` | `product.read` |
| `POST /products` | `product.create` |
| `PATCH /products/:id` | `product.update` |
| `PATCH /products/:id/enabled` | `product.toggle` |
| `DELETE /products/:id` | `product.delete` |
| `GET /shops` / `POST /shops` | `shop.read` / `shop.create` |
| `GET /inventory` / `GET /inventory/export` | `inventory.read` |
| `POST /inventory/inbound` | `inventory.inbound` |
| `POST /inventory/sales-outbound` | `inventory.sales_outbound` |
| `POST /inventory/adjustments` | `inventory.adjust` |
| `GET /stock-movements` | `movement.read` |
| 四个 `/reports/*` 路由 | 各自同名 `report.*` 权限 |
| `GET /audit-logs` | `audit.read` |
| `GET /backups/latest` / `POST /backups/run` | `backup.read` / `backup.run` |
| `GET /settings` / `POST /settings` | `setting.read` / `setting.update` |
| `GET /users` / `POST /users` | `user.read` / `user.create` |

`/health`、`/auth/login` 和公开 `/uploads/*` 不经过业务授权；`/auth/me`、`/auth/logout`、`/auth/password` 只要求有效会话。

## 6. 请求数据流

### 登录

1. 同源中间件验证修改请求的 `Origin`。
2. handler 按用户名或邮箱加载启用用户并校验 bcrypt 密码。
3. 生成随机令牌，写入哈希会话记录，固定过期时间为 7 天。
4. 设置 `gaowang_session` Cookie：`HttpOnly`、`SameSite=Strict`、路径 `/api/v1`；`SESSION_COOKIE_SECURE=true`、TLS 或可信转发协议任一表明 HTTPS 时设置 `Secure`。
5. 返回用户与当前权限；登录成功/失败继续写现有审计日志，不记录密码或令牌。

### 退出与改密

- 普通退出只按当前 Cookie 哈希删除一条会话并清 Cookie。
- 改密在一个数据库事务内更新密码哈希并删除该用户全部会话；任一步失败都回滚。提交后清当前 Cookie，审计不记录密码。
- 当前密码错误返回 `400 INVALID_CREDENTIALS`，因为此时会话仍然有效；受保护接口的 `401` 专用于会话失效。

### 已登录请求

1. 认证中间件读取 Cookie，使用 `AUTH_SECRET` 计算哈希。
2. 一次数据库查询关联 `sessions` 与当前启用的 `users`，同时验证固定过期时间；客户端传来的 `X-Dev-*` 头完全忽略。
3. 对 `staff` 再加载当前授权行并过滤为已知、可配置权限；`admin` 直接使用完整目录。
4. 中间件把当前用户和权限集合放入 Gin Context。
5. 路由权限中间件执行常量时间集合查找：允许后调用 handler，否则返回 `403`。

不缓存角色或权限，因此账号停用、角色数据库变更和权限保存从下一次请求起生效。认证失败会删除当前无效会话行（如存在）、清 Cookie 并返回 `401`。

### 权限更新

1. 验证请求权限键并计算依赖闭包。
2. 在事务中读取并排序旧授权、删除 `staff` 旧行、批量插入新行。
3. 同一事务写入审计；任一步失败全部回滚。
4. 返回最终有效权限。并发保存采用最后完成者生效，不增加版本号或锁定界面。

## 7. 首个管理员引导

- AutoMigrate 完成后、启动 HTTP 服务前调用一次引导函数。
- 若 `users` 表非空，不读取也不校验初始管理员环境变量，不创建或覆盖任何用户。
- 若表为空，三个 `INITIAL_ADMIN_*` 值必须齐全且邮箱/密码有效；创建唯一启用的 `admin`，失败则进程记录不含密码的错误并退出。
- 部署文档要求首次成功启动后删除 `INITIAL_ADMIN_PASSWORD`。
- 若旧数据库已有用户但没有启用的 `admin`，启动检查给出明确错误并停止，不绕过“仅空库自动引导”的边界。
- 当前部署只有一个 API 实例，因此不引入分布式启动锁；未来横向扩容时再增加数据库 advisory lock。

## 8. 前端行为

### 会话状态

- 删除 `DevSession`、`gaowang.devSession` 和 `X-Dev-*` 头逻辑；浏览器可读存储中不留身份或令牌。
- `AppShell` 初次挂载调用 `/auth/me`，并通过一个 React Context 向菜单和页面提供 `user`、`permissions`、`hasPermission`。
- 窗口重新获得焦点或 API 客户端收到 `403` 时重新加载 `/auth/me`；不使用轮询、WebSocket、SSE、全局状态库或请求缓存库。
- `401` 清理客户端内存状态并跳转 `/login`；`403` 只广播权限刷新事件并抛出原错误，绝不退出登录。
- 改密成功后因全部会话已撤销，前端提示后跳回登录页。

### 页面与动作

- 仪表盘始终可进入，只请求并显示用户有权读取的库存、流水和销售摘要；没有任何可展示权限时显示明确空状态。
- 商品、店铺、库存、流水、审计、备份、用户和权限页分别按对应读取权限控制菜单和直接访问。
- 报表页在拥有任一 `report.*` 权限时可进入，只请求并显示被授权板块；库存金额/低库存板块还要求 `inventory.read`。
- 设置页始终可进入以查看账号、改密和退出；系统设置卡片要求 `setting.read`，保存按钮要求 `setting.update`。
- 商品新增、编辑、启停、删除，店铺新增，三种库存操作，备份执行等按钮分别按动作权限隐藏。
- 审计页只有同时拥有 `user.read` 时才请求用户列表并显示人员下拉筛选；审计记录自身已带演员展示数据。
- 无页面权限时 AppShell 不挂载页面内容，直接渲染 Ant Design `Result` 403 页面，保留登录态。

### 权限矩阵

- 新增管理员专属“权限管理”导航和页面。
- Ant Design `Table` 按模块/动作分组；`admin` 列全选禁用，`staff` 列可编辑，管理员专属行在 `staff` 列禁用。
- 勾选动作时递归补齐依赖；取消基础权限时递归移除所有依赖项，并以文字提示自动关联关系。
- 页面只在点击保存时发送一次完整权限数组；保存成功后以服务端响应覆盖本地选择。

## 9. 错误与安全约束

| 场景 | 状态/代码 | 客户端行为 |
| --- | --- | --- |
| 缺少、无效、过期会话或停用账号 | `401 UNAUTHORIZED` | 跳转登录 |
| 登录凭据错误 | `401 INVALID_CREDENTIALS` | 留在登录页 |
| 当前密码错误 | `400 INVALID_CREDENTIALS` | 保留会话和表单 |
| 缺少业务权限 | `403 FORBIDDEN` | 保留会话、刷新权限、显示 403/原错误 |
| 修改请求来源不匹配 | `403 FORBIDDEN` | 不执行 handler |
| 权限键未知或管理员专属 | `400 VALIDATION` | 权限页显示原错误 |
| 会话或权限持久化失败 | `500 INTERNAL` | 不泄露内部错误 |

- 修改类方法 `POST`、`PUT`、`PATCH`、`DELETE` 必须携带 `Origin`，其 scheme + host 必须匹配请求 `Host`，或匹配现有 Next.js 反向代理提供的 `X-Forwarded-Host` 与 `X-Forwarded-Proto`；不开放 CORS。Nginx 必须保留上游 HTTPS 协议头。
- `AUTH_SECRET` 至少 32 字节并用于会话哈希；轮换它会使全部现有会话失效。
- Cookie 不设置 `Domain`，不允许 JavaScript 读取；生产部署必须配置 `SESSION_COOKIE_SECURE=true`，避免依赖代理自动推断。
- handler 的公开用户 DTO 永不包含 `PasswordHash`；日志和审计永不包含 Cookie、原始/哈希令牌或密码。
- 未来新增受保护业务路由若未挂权限中间件，路由覆盖测试必须失败。

## 10. 兼容、发布与回滚

- AutoMigrate 只新增 `sessions`、`staff_permissions` 表；现有业务数据不改写。
- 升级后所有 `staff` 初始为零权限，管理员必须在权限页显式授权；这是已确认的安全默认值。
- 旧浏览器中的开发 `localStorage` 数据被忽略，所有用户需要重新登录。
- 新旧认证协议不兼容，API 与 Web 必须同批发布；发布前先备份数据库并确认至少一个启用管理员。
- 回滚旧代码时新增表可保留，不影响旧模型；但旧开发头认证会重新生效，属于安全回退，只可作为短时应急。
- `/uploads/*` 继续公开，不受本次发布影响。

## 11. 取舍

- 不使用 JWT：服务端会话能立即撤销账号、密码和权限变化，代价是每个请求查询数据库。
- 不使用 Casbin/OPA：只有两个固定角色和一个可配置角色，固定目录与授权行更短、更容易审计。
- 不做通用角色授权表：首版只有一套共享的 staff 权限；真正增加自定义角色时再迁移为按角色授权。
- 不做权限缓存：当前规模下即时生效比减少一次小查询更重要；只有监控证明数据库查询成为瓶颈时才增加带失效机制的短缓存。
- 不做并发编辑版本控制：权限管理员少且整表保存，最后完成者生效足够；出现真实覆盖事故后再加版本字段。
- 不做定时会话清理任务：登录时顺手删除过期记录，避免新增调度器；只有会话表增长成为问题时再增加后台清理。

## 12. 验证重点

- 伪造 `X-Dev-*` 头无法登录或提权；Cookie 原文不出现在数据库。
- 过期、退出、改密和停用后的会话立即拒绝。
- `admin` 永远通过；`staff` 仅能访问授权键，且删除权限与其他写权限完全独立。
- 权限闭包、未知键、管理员专属键和原子审计均有后端测试。
- 枚举 Gin 路由，以“有效但零权限 staff”请求每个非公开/非账号路由，任何非 `403` 结果都使测试失败。
- 前端测试覆盖 Cookie 请求、`401` 跳转、`403` 不退出、权限选择级联和路由可见性。
- 运行 Go、TypeScript、Vitest、Next build，并人工检查零权限 staff、细分按钮、直接 URL 403 和权限刷新。
