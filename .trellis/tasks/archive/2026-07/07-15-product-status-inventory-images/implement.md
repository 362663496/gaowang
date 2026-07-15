# 商品状态管理与库存图片：实施计划

## 1. Backend

- [x] 在 `apps/api/internal/http/handlers/products.go` 增加状态请求校验、状态更新和受引用保护的删除处理；复用同包 `parseUUID`、`bindJSON`、`writeError`、`recordAudit`。
- [x] 在 `apps/api/internal/http/router.go` 的现有 authenticated product 路由旁注册 `PATCH /products/:id/enabled` 与 `DELETE /products/:id`。
- [x] 在审计页面已知动作列表加入 `product.enable`、`product.disable`、`product.delete`。
- [x] 添加一个小型商品生命周期回归测试，覆盖显式 `false` 状态更新、未使用商品删除、有关联商品返回 `409`，并确认持久化、图片清理与审计结果。

## 2. Frontend

- [x] 在商品表增加彩色状态 `Badge`、反向状态按钮和带原生确认的删除按钮；操作成功后局部更新 `products` 状态，失败时显示后端消息。
- [x] 在当前库存商品单元格加入商品图片缩略图与无图占位图标，不增加请求或共享组件。

## 3. Local validation

- [x] 格式化改动的 Go 文件：`gofmt -w <changed-go-files>`。
- [x] 后端：`cd apps/api && go test ./... && go vet ./...`。
- [x] 前端：`cd apps/web && npm run lint && npx tsc --noEmit --incremental false && npm test && npm run build`。
- [x] 检查 `git diff --check`、完整 diff、API/前端契约及无关文件改动。

## 4. Release and deployment

- [x] 记录部署前 `/opt/gaowang/current` 目标和两个服务状态。
- [x] 本地交叉编译 Linux amd64 API 二进制；把二进制和排除 `.next`/`node_modules` 的 Web 源码上传到新的 `/opt/gaowang/releases/<timestamp>`。
- [x] 在服务器使用现有 Node/npm 构建 Web standalone 产物，并组装 `api/`、`web/` 版本目录；构建完成前不切换 `current`。
- [x] 原子更新 `current` 软链，依次重启 `gaowang-api.service`、`gaowang-web.service`。
- [x] 验证 `systemctl is-active`、`http://127.0.0.1:9509/api/v1/health`、`http://127.0.0.1:9508/login`、商品/库存静态资源路径及最近服务日志。
- [x] 若任一验证失败，恢复部署前软链目标、重启服务并再次检查健康状态；始终保留共享数据库、上传和备份目录。

## Deployment result

- Active release: `/opt/gaowang/releases/20260715205042`
- Previous rollback release: `/opt/gaowang/releases/20260705224738`
- Online checks: service health, Nginx API/Web/upload routes, API lifecycle smoke, audit persistence, and browser UI/image rendering all passed.

## Risk and rollback points

- 删除接口是唯一数据破坏点：后端引用检查和数据库 `RESTRICT` 约束共同保护历史数据，前端确认只作为防误触补充。
- 版本切换前保留旧 release；回滚只切软链，不重建 PostgreSQL、不触碰共享目录。
- 远端 Web 构建避免把 macOS 原生 Node 模块带到 Linux 运行环境。
