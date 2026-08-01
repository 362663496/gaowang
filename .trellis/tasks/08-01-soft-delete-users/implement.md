# 实施计划：用户软删除

## 1. 影响分析

- [x] 对 `models.User`、`UserHandler.List`、`UserHandler.Create`、新增 `UserHandler.Delete`、`AuthHandler.Login`、`SessionService.LookupActiveUser`、`PermissionHandler.Get/Update`、`mountProtected`、`UsersPage` 分别执行 GitNexus upstream impact。
- [x] 若任一结果为 HIGH/CRITICAL，在编辑前报告影响流程和风险。

## 2. 后端实现

- [x] 给 `User` 增加显式 `DeletedAt` 索引字段，由现有 `db.Migrate` 覆盖生产和测试迁移。
- [x] 增加管理员专用 `PermUserDelete`、目录项和受保护 DELETE 路由。
- [x] 实现带锁事务：阻止自删/最后管理员、软删并禁用、撤销全部会话和权限、事务内写 `user.delete` 审计。
- [x] 在用户列表、登录、权限管理和旧权限迁移的活动用户查询中增加显式删除过滤。
- [x] 在会话查找中同时校验 `Enabled` 和 `DeletedAt`。
- [x] 保持历史流水和审计预加载无删除过滤。

## 3. 前端实现

- [x] 用户页读取当前会话和 `user.delete` 权限。
- [x] 增加 Ant Design 危险确认、逐用户 loading、防重复提交、错误保留和成功刷新。
- [x] 当前用户不显示删除按钮；最后管理员冲突由服务端提示。
- [x] 审计动作标签增加“删除用户”。

## 4. 自动验证

- [x] 新增 `users_test.go` 覆盖成功软删、会话/权限撤销、历史关联、审计和默认列表过滤。
- [x] 覆盖自删、最后管理员、重复/缺失目标和原子回滚。
- [x] 扩充 auth/session/permissions 测试，证明已删除账号在所有活动入口被拒绝。
- [x] `cd apps/api && gofmt -w <changed-go-files> && go test ./... && go vet ./...`。
- [x] `cd apps/web && npm run lint && npx tsc --noEmit --incremental false && npm test && npm run build`。

## 5. 手工验收与回滚点

统一留到四个子任务整合完成后，在最终部署前后跑一轮浏览器与线上验收。

- [x] 浏览器删除普通员工，确认确认框、loading、成功提示、列表刷新和错误提示。
- [x] 用被删用户原会话访问 `/auth/me` 得到 401，重新登录失败。
- [x] 查看旧流水与审计，确认姓名仍显示。
- [x] 回滚代码时无需回滚 schema；确认旧版本仍因 `enabled=false` 拒绝软删账号。
