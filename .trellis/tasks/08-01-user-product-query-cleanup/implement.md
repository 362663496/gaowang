# 实施计划：用户、商品与飞书查询整合

## 0. 开始门禁

- [x] 用户审阅并批准父任务及四个子任务的 `prd.md`、`design.md`、`implement.md`。
- [x] 批准后激活任务；实施前加载 `trellis-before-dev` 和对应层规范。
- [x] 记录当前 `git status`，保留 `AGENTS.md`、`.claude/`、`CLAUDE.md` 等用户已有改动。
- [x] 每次修改函数、方法或类前执行 GitNexus upstream impact；若为 HIGH/CRITICAL，先向用户报告再编辑。

## 1. 子任务实施顺序

- [x] 实施并独立验证 `08-01-soft-delete-users`。
- [x] 实施并独立验证 `08-01-remove-product-sale-price`。
- [x] 在销售价子任务契约稳定后实施并验证 `08-01-optional-inventory-notes`。
- [x] 仅在销售价和备注契约稳定后实施 `08-01-fix-lark-query-recognition`。
- [x] 每个子任务按其 `implement.md` 留下可运行回归检查，不把失败推迟到最终集成。

## 2. 跨任务集成验证

- [x] 软删一个有历史流水/审计的普通用户，确认账号、会话和权限失效，历史姓名仍可见。
- [x] 确认商品创建/编辑不提交销售价，销售出库、流水修订、报表、仪表盘和飞书均无销售额/毛利。
- [x] 验证入库/销售出库备注的留空、持久化、超限回滚、流水与飞书展示。
- [x] 验证飞书操作人查询可统计已删除操作人的历史流水，但不展示邮箱、权限或账号状态。
- [x] 验证“今天按商品排序并列出各店铺占比”等自然语言生成二维计划和匹配卡片。
- [x] 验证 DeepSeek 500/429/网络异常一次重试，JSON/计划错误不重试，总时延上限 10 秒。

## 3. 质量门禁

- [x] `cd apps/api && gofmt -w <changed-go-files>`。
- [x] `cd apps/api && go test ./... && go vet ./...`。
- [x] `cd apps/web && npm run lint`。
- [x] `cd apps/web && npx tsc --noEmit --incremental false`。
- [x] `cd apps/web && npm test && npm run build`。
- [x] 扫描 `apps/api`、`apps/web`，确认不存在运行时 `DefaultSaleCents`、`default_sale_cents`、销售收入或毛利字段依赖。
- [x] 运行 `trellis-check`，修复发现的问题并重跑受影响检查。
- [x] 按实际新契约更新 `.trellis/spec/`，不改写历史设计文档。

## 4. 提交与影响复核

- [x] 仅暂存本任务文件，保留无关工作树改动。
- [x] 运行 `npx gitnexus detect-changes --scope staged --repo gaowang`，确认只影响预期符号和流程。
- [x] 审阅 staged diff，提交并推送精确发布提交。

## 5. 阿里云发布

- [x] 按 Aliyun release 规范本地构建 Linux API 和 Web standalone 包并完成本地 smoke。
- [x] 切换前创建并校验 PostgreSQL 备份，核对归档校验和及提交元数据。
- [x] 原子切换 `/opt/gaowang/current`，依次重启 API/Web。
- [x] 验证 API/Nginx 健康、登录、静态资源、上传、`users.deleted_at`、服务重启次数和近期日志。
- [ ] 用目标飞书群执行代表性自然语言查询，确认回复、重试诊断和卡片布局。
- [x] 任一生产检查失败则切回上一 symlink 并重启两项服务。
