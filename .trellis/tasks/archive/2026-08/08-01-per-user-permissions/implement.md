# 按用户管理权限：实施计划

## 1. 模型与迁移

- 新增 `UserPermission` 复合主键模型和用户级外键；保留仅供迁移读取的旧表模型。
- 更新 `db.Migrate`，通过 `settings` 标记在事务中把旧共享授权复制给全部已有员工。
- 增加 SQLite 迁移测试：首次复制、管理员排除、重复运行不覆盖用户级调整、新员工不继承旧权限。

## 2. 服务与鉴权

- 将 `EffectivePermissions` 改为按 `user.ID` 查询。
- 将 `ReplaceStaffPermissions` 收窄为目标用户级原子替换，保留权限闭包和校验。
- 更新服务测试和共享测试助手，覆盖两个员工之间的隔离、零权限、回滚及管理员全权限。

## 3. API

- 更新权限 GET 响应为目录加员工授权列表，使用批量读取并稳定排序。
- 更新 PUT 请求以接收 `user_id`，校验 UUID、用户存在且角色为 `staff`。
- 将目标用户 ID 与前后授权写入同事务审计；响应返回服务端展开后的权限。
- 更新 handler 测试，覆盖读取、单用户更新、隔离、非法目标、权限校验、审计和路由鉴权。

## 4. Web 页面

- 更新本地响应类型和状态为员工授权数组。
- 加入员工 `Select`，按当前员工展示和修改权限矩阵；切换时保留页面内草稿。
- 更新标题、说明、列名、空状态及保存反馈；继续复用现有权限依赖 helpers。
- 不新增前端依赖或通用组件。

## 5. 规范与验证

- 更新后端 HTTP/数据库权限合同和前端权限页面合同。
- 运行：
  - `gofmt -w <changed-go-files>`
  - `cd apps/api && go test ./... && go vet ./...`
  - `cd apps/web && npm run lint && npx tsc --noEmit --incremental false && npm test && npm run build`
  - `npx gitnexus detect-changes --scope all --repo gaowang`
- 手动检查：两个员工分别登录后导航和业务接口权限互不影响；管理员切换员工、保存、刷新后保持；旧共享权限迁移一次且可回滚。

## 6. 修改前风险门槛

- 已完成 GitNexus upstream impact：`StaffPermission` 与 `EffectivePermissions` 为 HIGH，`Migrate`、权限替换函数和页面为 LOW。
- 实现时先改数据与服务并跑 Go 测试，再改 API/UI；任一鉴权回归即回退该阶段，不带失败进入下一层。
