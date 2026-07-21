# API 访问控制实施计划

## 实施原则

- 本任务保持单一跨层任务，不创建子任务：后端 Cookie 会话与前端开发头登录态不兼容，拆开发布会直接导致无法登录。
- 按“数据库与安全服务 → 后端契约 → 前端会话 → 页面授权 → 文档与全量验证”顺序推进。
- 不引入新 Go/npm 依赖；使用 Go 标准库、现有 Gin/GORM、React 与 Ant Design。
- 任何中间步骤都不得单独部署；API 与 Web 完成后同批发布。

## 1. 数据模型、配置和首个管理员

- [x] 在 `apps/api/internal/models/models.go` 增加 `Session` 与 `StaffPermission`，保持公开 DTO 不直接序列化用户模型。
- [x] 在 `apps/api/internal/db/db.go` 的集中迁移列表加入两个新模型。
- [x] 在 `apps/api/internal/config/config.go` 读取可选的 `INITIAL_ADMIN_NAME`、`INITIAL_ADMIN_EMAIL`、`INITIAL_ADMIN_PASSWORD` 与布尔值 `SESSION_COOKIE_SECURE`；保留 `AUTH_SECRET` 至少 32 字节校验。
- [x] 在 `apps/api/internal/services` 增加最小引导函数：空库时校验并创建 admin，非空库不消费初始密码；非空但无启用 admin 时返回可操作错误。
- [x] 在 `apps/api/cmd/api/main.go` 中于迁移后、启动路由前运行引导/管理员检查。
- [x] 增加配置和引导测试：空库成功、缺字段失败、已有用户不覆盖、无启用 admin 失败、密码不出现在错误文本。

检查点：`go test ./internal/config ./internal/services ./internal/db`。

## 2. 数据库会话与同源防护

- [ ] 在 `apps/api/internal/services` 实现 32 字节随机令牌、HMAC-SHA256 哈希、固定 7 天过期、创建/查询/删除当前会话、删除用户全部会话和登录时清理过期行。
- [ ] 在 `apps/api/internal/http/middleware.go` 替换 `X-Dev-*` 认证：从 Cookie 查询会话与启用用户，将当前用户放入 Gin Context；失败时清 Cookie 并统一返回 `401`。
- [ ] 增加修改类请求 `Origin` 校验，匹配请求或可信转发后的 scheme + host；保持无 CORS。
- [ ] 在 `apps/api/internal/http/handlers/auth.go` 完成登录 Set-Cookie、`GET /auth/me`、当前会话退出，以及改密后事务性删除该用户所有会话。
- [ ] Cookie 统一使用 `gaowang_session`、`Path=/api/v1`、`HttpOnly`、`SameSite=Strict`、7 天 `Expires/Max-Age`；配置开关或 HTTPS 请求设置 `Secure`。
- [ ] 登录成功/失败、改密继续记录审计，但不记录密码、Cookie 或令牌哈希。

后端测试：

- [ ] 正确登录会设置 Cookie，数据库只出现哈希；用户名和邮箱均可登录。
- [ ] 缺失/伪造 Cookie、过期会话、停用用户返回 `401`；伪造 `X-Dev-*` 无效。
- [ ] 普通退出只撤销当前会话；同账号另一会话仍有效。
- [ ] 改密撤销全部会话，旧密码失败、新密码成功。
- [ ] 当前密码错误返回 `400 INVALID_CREDENTIALS` 且不清会话；受保护接口只有会话失效返回 `401`。
- [ ] 非同源修改请求在 handler 前返回 `403`；同源请求通过。
- [ ] 提取同包测试 helper，用直接插入哈希会话 + Cookie 的方式替换现有测试中的开发头，避免每个测试重复 bcrypt 登录。

检查点：`go test ./internal/http/... ./internal/services/...`。

## 3. 权限目录、存储与统一中间件

- [ ] 在 `apps/api/internal/services/permissions.go` 定义唯一权限目录：键、模块/动作标签、是否允许 staff、依赖；实现去重、未知键拒绝、传递闭包与反向依赖计算。
- [ ] 实现从 `staff_permissions` 读取 staff 有效授权；只返回目录中已知且可授予的键，admin 直接返回全部目录。
- [ ] 在认证中间件为 staff 每次请求加载当前权限；不增加缓存。
- [ ] 增加 `RequirePermission(permission)`：admin 直接通过，staff 做集合查询，缺少权限返回标准 `403 FORBIDDEN`。
- [ ] 新建 `apps/api/internal/http/handlers/permissions.go`，实现 `GET /permissions` 与 `PUT /permissions`。
- [ ] `PUT` 在一个事务内替换 staff 授权并写 `permission.updated` 审计；审计失败则回滚，前后数组排序后写入 metadata。
- [ ] 更新 `apps/api/internal/http/router.go`：增加 `/auth/me`、`/auth/logout` 和权限接口；为每个业务路由显式挂载设计文档中的权限键，移除以客户端角色为依据的旧角色门禁。

后端测试：

- [ ] 权限闭包覆盖多级依赖、级联反向依赖、重复键、未知键、admin 专属键。
- [ ] admin 永远允许；零权限 staff 拒绝；授权 staff 允许；删除权限与新增/编辑/启停互不替代。
- [ ] 权限保存为空、保存组合、回滚和审计 before/after 均正确；新权限没有数据库授权时默认拒绝。
- [ ] 枚举 `router.Routes()`：携带有效同源 `Origin` 的零权限 staff 请求每个非公开、非账号路由都必须得到 `403`，从而发现漏挂中间件的新路由。
- [ ] 保留 `/uploads/*` 与 `/health` 的公开测试。

检查点：在 `apps/api` 运行 `gofmt`、`go test ./...`、`go vet ./...`。

## 4. 前端请求与会话上下文

- [ ] 在 `apps/web/src/lib/api.ts` 删除 localStorage、`DevSession`、自定义开发头；所有请求继续使用 `credentials: "include"`。
- [ ] 统一错误语义：`401` 广播会话失效并跳登录，`403` 只广播授权刷新且保留当前页面/登录态。
- [ ] 更新 `apps/web/src/lib/api.test.ts`：验证不发送开发头、仍携带 Cookie、`401` 跳转、`403` 不跳转且保留错误。
- [ ] 在 `AppShell` 内建立一个必要的 Session Context，首次加载 `/auth/me`，向页面暴露当前用户、权限集合、`hasPermission` 和刷新函数。
- [ ] 监听窗口 `focus`、会话失效事件和授权刷新事件，并正确清理监听器；不加入全局 store、React Query 或 SWR。
- [ ] 登录页只调用 `/auth/login` 后跳转；退出按钮调用 `/auth/logout`；界面显示真实用户名/角色，不再显示截断的客户端用户 ID。
- [ ] 修改密码成功后提示会话已失效并跳转登录。

检查点：在 `apps/web` 运行 `npm test -- src/lib/api.test.ts` 与 `npx tsc --noEmit --incremental false`。

## 5. 页面访问与细粒度动作

- [ ] 在 AppShell 的现有导航定义中增加页面所需权限和“权限管理”入口；无权限菜单隐藏，直接 URL 渲染 Ant Design 403 `Result`，不挂载页面请求。
- [ ] 仪表盘按 `inventory.read`、`movement.read`、`report.sales_summary` 分别请求和渲染板块；全部缺失时显示零权限空状态。
- [ ] 商品页分别按 `product.create/update/toggle/delete` 隐藏新增、修改、启停、删除入口。
- [ ] 店铺页按 `shop.create` 隐藏新增入口。
- [ ] 库存页按三种库存动作权限渲染按钮；依赖闭包保证所需商品/店铺读取接口可用。
- [ ] 报表页只调用并展示有权的四个报表接口；库存板块额外要求 `inventory.read`，不让一个无权请求拖垮整页。
- [ ] 审计页仅在 `user.read` 时请求用户列表/显示人员筛选；staff 仍可从审计响应看到演员展示信息。
- [ ] 备份页按 `backup.run` 隐藏执行按钮；设置页始终保留账号与改密，按 `setting.read/update` 控制系统设置卡片和保存按钮。
- [ ] 用户页保持现有查看/新建功能；不添加停用、改角色或删除。

前端测试：

- [ ] 用纯函数测试路由可见性和 `hasPermission`，不新增组件测试框架。
- [ ] 确认各页面没有遗留 `readDevSession`、`writeDevSession`、`apiDeleteSession`、`X-Dev-*` 或 `gaowang.devSession`。

## 6. 权限矩阵页面

- [ ] 新建 `apps/web/src/app/(app)/permissions/page.tsx`，通过 `GET /permissions` 加载目录和 staff 选择。
- [ ] 使用现有 Ant Design `Table`、`Checkbox`、`Alert`、`Button`：admin 全选锁定，staff 可配置，admin 专属项禁用并标注。
- [ ] 在一个小型纯函数模块中实现勾选依赖闭包和取消反向级联，直接消费服务端 `requires`；为其添加 Vitest。
- [ ] 保存时一次 `PUT /permissions`，按钮有 loading，错误保留在页面，成功后用服务端响应覆盖选择并提示。
- [ ] 表格保持横向滚动、复选框有可读标签，依赖不只靠颜色表达。

检查点：运行权限 helper 测试、lint 和 TypeScript。

## 7. 部署文档与升级说明

- [ ] 更新 `.env.example`，加入空的 `INITIAL_ADMIN_*` 与本地 `SESSION_COOKIE_SECURE=false` 示例，避免提交默认管理员密码。
- [ ] 更新 `README.md`：首次启动步骤、成功后删除初始密码、7 天会话、API/Web 同批发布、升级后 staff 默认零权限。
- [ ] 明确生产必须通过 HTTPS 访问并设置 `SESSION_COOKIE_SECURE=true`；更新 `deploy/nginx/app.conf` 以保留上游 `X-Forwarded-Proto`，保留公开上传说明。
- [ ] 部署前检查已有数据库至少一个启用 admin；先备份数据库。

## 8. 最终验证门

后端（`apps/api`）：

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

前端（`apps/web`）：

```bash
npm run lint
npx tsc --noEmit --incremental false
npm test
npm run build
npm audit --omit=dev
```

根目录：

```bash
docker compose config
rg -n "X-Dev-|gaowang.devSession|readDevSession|writeDevSession" apps
```

人工验收：

- [ ] 新空库通过环境变量创建首个 admin，随后删除初始密码仍可重启。
- [ ] admin 配置 staff 为零权限、只读、允许新增/编辑/启停但禁止删除三组场景。
- [ ] staff 直接调用删除接口得到 `403`，同时其他获准动作成功。
- [ ] 权限保存后 staff 下一请求生效；已打开页面在聚焦或遇到 `403` 后刷新菜单。
- [ ] 零权限 staff 可登录、改密、退出，看到明确空状态且无重定向循环。
- [ ] 直接访问无权 URL 显示 403；API `403` 不退出，过期会话 `401` 才退出。
- [ ] 桌面与 500px 导航、权限矩阵、隐藏按钮、加载/错误/空状态均可用。

## 风险与回滚点

| 风险文件/阶段 | 风险 | 回滚/控制 |
| --- | --- | --- |
| `middleware.go`, `auth.go`, `api.ts`, `app-shell.tsx` | 新旧登录协议不兼容 | 后端会话测试通过后再改前端；最终同批发布 |
| `router.go` | 漏挂或错挂权限会越权/误拒绝 | 路由枚举测试 + 关键动作独立权限测试 |
| `staff_permissions` 整表替换 | 保存中途失败造成权限丢失 | 显式事务，审计同事务，失败不提交 |
| staff 默认零权限 | 升级后业务中断 | 发布说明提前告知；admin 登录后先配置 |
| Cookie `Secure`/Origin | 代理头配置错误导致无法登录或修改 | 在实际 HTTPS 入口做登录、改密、权限保存冒烟测试 |
| 回滚旧版本 | 重新启用可伪造开发头 | 只作为短时应急；新增表不删除，修复后立即回到新版本 |

数据库回滚不执行 DROP：新增表保留即可。若发布失败，先回滚 API 与 Web 镜像到同一旧版本；不要单独回滚一层，也不要删除会话/权限表或现有业务数据。

## 开始实施前复核

- [ ] 用户已评审并批准 `prd.md`、`design.md`、`implement.md`。
- [ ] 当前任务仍处于 planning；得到批准后才运行 `task.py start`。
- [ ] Phase 2 开始时运行 `trellis-before-dev` 重新加载适用规范。
