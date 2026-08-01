# 技术设计：用户软删除

## 数据模型

在 `models.User` 增加显式字段：

```go
DeletedAt *time.Time `gorm:"index"`
```

不用 `gorm.DeletedAt`。业务列表、登录和权限管理显式过滤 `deleted_at IS NULL`，而库存流水、审计日志的历史 `Preload("Operator")` / `Preload("Actor")` 保持无作用域读取，因此不会丢失姓名。

用户名和邮箱不做匿名化，也不释放唯一占用。现有邮箱唯一索引继续阻止复用；创建用户时的用户名查重继续包含软删记录。

## 权限与路由

- 新权限键：`user.delete`，管理员专用，依赖 `user.read`。
- 新路由：`DELETE /api/v1/users/:id`，使用 `RequirePermission(PermUserDelete)`。
- 成功：`204 No Content`。
- 错误：
  - 无效 UUID：`400 VALIDATION`
  - 不存在或已删除：`404 USER_NOT_FOUND`
  - 删除当前账号：`409 USER_DELETE_SELF`
  - 删除最后一个可用管理员：`409 LAST_ADMIN`
  - 事务失败：`500 USER_DELETE_FAILED`

## 删除事务

`UserHandler.Delete` 复用商品删除的事务与行锁风格：

1. 解析目标 ID；在事务前即可用当前会话 ID 拒绝明显的自删请求，事务内再次以目标记录为准校验。
2. 以 `FOR UPDATE` 锁定 `deleted_at IS NULL` 的目标用户。
3. 若目标是可用管理员，按稳定 ID 顺序锁定全部 `enabled=true AND deleted_at IS NULL` 管理员，确认数量大于 1。
4. 同一事务将目标设为 `enabled=false`、`deleted_at=UTC now`。
5. 删除目标的 `sessions` 和 `user_permissions`。
6. 同一事务写入 `user.delete` 审计，元数据只含目标姓名、邮箱和角色。

任何步骤失败都回滚账号状态、会话、权限和审计。历史外键行不修改。

## 读取边界

- `UserHandler.List`：只列出 `deleted_at IS NULL`，分页总数使用相同过滤。
- `AuthHandler.Login`：要求 `enabled=true AND deleted_at IS NULL`。
- `SessionService.LookupActiveUser`：即使会话撤销遗漏，也拒绝 `DeletedAt != nil`。
- `PermissionHandler.Get/Update`：不列出也不接受已删除目标。
- 一次性旧权限迁移只读取未删除员工。
- 流水、审计和飞书历史操作人聚合不加删除过滤，只返回既有姓名。

## 前端

在用户表增加“操作”列：

- 仅拥有 `user.delete` 时展示删除按钮。
- 当前登录用户不展示删除按钮。
- 使用 `App.useApp().modal.confirm`，明确说明账号立即失效、历史记录保留、身份不可复用。
- 用目标用户 ID 维护提交中状态，禁用其他删除操作并避免重复提交。
- 成功后提示并重新加载当前分页；失败保留列表并显示服务端原始消息。
- 审计动作字典增加 `user.delete`。

## 迁移与回滚

`AutoMigrate` 只新增可空索引列，既有用户默认为未删除。回滚旧版本时多余列被忽略；软删用户仍因 `enabled=false` 被旧版本拒绝登录。恢复需求本次只保留数据库手工恢复能力。

## 测试重点

- 普通用户删除后的列表、登录、会话、权限和审计原子状态。
- 历史 `StockMovement.Operator`、`LastEditedBy` 与 `AuditLog.Actor` 仍可读取。
- 自删、最后管理员、重复删除、缺失 ID 和事务回滚。
- 权限列表/更新排除已删除员工。
- 前端按钮权限、自删隐藏、确认、忙碌态和错误保留通过静态检查及浏览器验收。
