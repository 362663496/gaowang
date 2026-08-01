# 按用户管理权限：技术设计

## 1. 边界与风险

- 保留现有权限目录、依赖闭包、管理员全权限规则和路由权限键。
- 改动覆盖 `models → db migration → services → handlers → web page`，不增加依赖。
- GitNexus 将 `StaffPermission` 与 `EffectivePermissions` 标记为 HIGH：后者直接服务 `/auth/me` 和 `RequireAuth`，因此必须完整回归登录态与业务路由鉴权。
- 页面仍使用 `/permissions`，不把权限编辑合并进用户管理页，也不新增角色或模板概念。

## 2. 数据模型与兼容迁移

新增 `user_permissions`：

```text
user_id uuid       composite primary key, FK users.id, ON DELETE CASCADE
permission varchar composite primary key
created_at timestamp
```

旧 `staff_permissions(permission, created_at)` 仅作为兼容迁移来源。`db.Migrate` 先创建 `user_permissions`，再执行一次可重试事务：

1. 若旧表不存在，直接结束。
2. 读取 `settings` 中的内部迁移标记；已存在则结束。
3. 读取所有旧授权与现有 `staff` 用户，将每个旧授权按用户写入 `user_permissions`，冲突时跳过。
4. 最后写入迁移标记并提交。

旧表及其数据保留，便于回滚到旧应用版本；新代码只读取 `user_permissions`。标记与复制处于同一事务，失败后可安全重试，成功后的服务重启不会覆盖个性化授权。新建员工没有对应行，自然得到零权限。

## 3. 服务与鉴权

- `EffectivePermissions(db, user)`：管理员返回完整目录；其他用户只查询 `user_permissions.user_id = user.ID`，继续忽略未知或管理员专属键。
- 将全局替换函数改为 `ReplaceUserPermissions(tx, userID, keys)`：只读取、删除并重建目标用户的授权，继续复用 `ExpandPermissionClosure`。
- 不缓存权限。每次认证仍沿用当前数据库读取行为，因此新授权在下一次请求生效；当前前端会话继续通过 focus/403 刷新 `/auth/me`。

## 4. HTTP 合同

### `GET /api/v1/permissions`

返回权限目录和全部 `staff` 的当前授权，按姓名、邮箱、ID 稳定排序：

```json
{
  "catalog": [],
  "users": [
    {"id":"UUID","name":"员工","email":"staff@example.com","permissions":["product.read"]}
  ]
}
```

该列表是权限页员工选择器的数据源；管理员不返回。查询使用一次员工查询和一次授权查询，避免逐用户查询。

### `PUT /api/v1/permissions`

请求：

```json
{"user_id":"UUID","permissions":["product.create"]}
```

响应：

```json
{"permissions":["product.create","product.read"]}
```

- 无效 UUID：`400 VALIDATION`。
- 用户不存在：`404 USER_NOT_FOUND`。
- 目标不是 `staff`：`400 VALIDATION`。
- 未知或管理员专属权限：`400 VALIDATION`。
- 数据库失败：`500 INTERNAL`。
- 成功时授权替换与 `permission.updated` 审计同一事务；审计 `resource_id` 为目标用户 ID，metadata 包含 `user_id`、`before`、`after`。

路由保护保持 `GET → permission.read`、`PUT → permission.update`。

## 5. 页面交互

- 加载一次 `/permissions`，首位员工自动选中。
- 页面顶部使用 Ant Design `Select` 展示姓名与邮箱；权限表只展示“管理员”和“当前员工”两列。
- 每个用户的页面内草稿保存在返回的用户数组中，切换员工不会丢失本次页面内尚未保存的勾选。
- 保存只提交当前员工，成功后用服务端展开后的权限覆盖该员工草稿。
- 无员工时显示 `Empty`，并隐藏权限表和保存按钮；加载、错误、保存中与权限只读状态沿用现有反馈模式。
- 不增加批量保存、离页脏状态拦截或权限模板。

## 6. 回滚

- 应用回滚：旧版本继续读取保留的 `staff_permissions`，恢复共享授权语义。
- 数据库无需删除 `user_permissions`；再次升级时迁移标记阻止重复覆盖。
- 新版本部署后新增的员工在旧版本回滚期间会重新采用旧共享权限，这是旧版本原有语义。
