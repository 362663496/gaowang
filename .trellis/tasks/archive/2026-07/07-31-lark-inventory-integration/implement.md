# 飞书库存集成实施计划

## 0. Start gate

- 用户审阅并明确批准本 PRD、设计和实施计划。
- 批准后运行 `python3 ./.trellis/scripts/task.py start .trellis/tasks/07-31-lark-inventory-integration`。
- 进入 Phase 2 后先加载 `trellis-before-dev`；本任务为 inline 模式，不维护 `implement.jsonl`/`check.jsonl`。
- 开始修改每个现有符号前执行 GitNexus upstream impact，至少覆盖：
  - `Config`
  - `Load`
  - `main`
  - `mountProtected`
  - `InventoryHandler.CreateInbound`
  - `InventoryHandler.CreateSalesOutbound`
  - `InventoryHandler.CreateAdjustment`
  - `InventoryService.CreateInbound`
  - `InventoryService.CreateSalesOutbound`
  - `InventoryService.CreateAdjustment`

## 1. Add SDK and configuration

Files:

- `apps/api/go.mod`
- `apps/api/go.sum`
- `apps/api/internal/config/config.go`
- `apps/api/internal/config/config_test.go`
- `.env.example`

Work:

- Add the official Lark OpenAPI Go SDK.
- Add `LARK_APP_ID`, `LARK_APP_SECRET`, and `LARK_CHAT_ID`.
- Implement all-empty disabled / all-present enabled / partial invalid validation.
- Add tests proving errors never include the secret value.

Check:

```bash
cd apps/api
go test ./internal/config
```

## 2. Return committed inventory change data

Files:

- `apps/api/internal/services/inventory.go`
- `apps/api/internal/services/inventory_test.go`

Work:

- Add `InventoryChange`.
- Make inbound, sales outbound, and adjustment return product, movement, before quantity, and after quantity only after successful commit.
- Keep price and inventory arithmetic unchanged.
- Add transition tests for normal→low, low→low, low→normal, threshold zero, and rejected operations.

Check:

```bash
cd apps/api
go test ./internal/services -run 'Test_InventoryService'
```

## 3. Implement the bot service

Files:

- `apps/api/internal/services/lark.go`
- `apps/api/internal/services/lark_cards.go`
- `apps/api/internal/services/lark_test.go`

Work:

- Start the SDK long connection and register `im.message.receive_v1`.
- Filter by fixed group, group chat, @mention and text message.
- Add bounded message dedupe and query queue.
- Parse the four fixed commands.
- Query active products/inventory with a six-row cap.
- Build full/compact/help/not-found cards.
- Upload and cache images with no-image fallback.
- Reply with UUID idempotency.
- Persist best-effort query audit records.
- Implement `LarkNotifier`, notification cards, bounded queue and finite retry.

Checks:

```bash
cd apps/api
go test ./internal/services -run 'Test_Lark'
```

## 4. Wire notifications after successful stock writes

Files:

- `apps/api/internal/http/handlers/inventory.go`
- `apps/api/internal/http/handlers/inventory_test.go` or the nearest existing inventory route test
- `apps/api/internal/http/router.go` (`mountProtected` only)

Work:

- Add the concrete notifier to `InventoryHandler`.
- Enqueue exactly one notification after each successful operation and existing audit call.
- Do not enqueue for validation, conflict, archived product or database failures.
- Assert notifier failure/queue saturation does not alter HTTP status or committed state.
- Do not change `NewRouter` signature or body.

Check:

```bash
cd apps/api
go test ./internal/http/handlers -run 'Test_.*Inventory'
```

## 5. Start the long-connection listener

Files:

- `apps/api/cmd/api/main.go`

Work:

- Start `RunLarkBot` after database/bootstrap success.
- Pass a cancellable context.
- Log listener failure through `slog` without printing configuration values or terminating the API.
- Leave startup unchanged when Lark is disabled.

Check:

```bash
cd apps/api
go test ./cmd/api ./internal/services
```

## 6. Document setup and deployment

Files:

- `README.md`
- `.env.example`

Work:

- Document app creation, bot capability, minimal scopes, message event subscription, long connection, group installation and `chat_id`.
- Document the three environment variables and all-empty rollback.
- Do not add Nginx routes or frontend configuration.

## 7. Quality gate

Run:

```bash
cd apps/api
gofmt -w <changed-go-files>
go test ./...
go vet ./...
cd ../..
npx gitnexus detect-changes -r gaowang -s all
git diff --check
git status --short
```

Verify:

- GitNexus reports only the expected config, inventory, handler, startup and Lark flows.
- No application secret/token appears in diff, test output or logs.
- Existing inventory, movement, product, permission, audit and router tests pass.
- No user-owned changes (`AGENTS.md`, `.claude/`, `CLAUDE.md`) are modified or committed.

## 8. Manual QA and release

Before production:

- Create a test app/group or use the intended fixed group.
- Verify `帮助`, `查库存`, `查商品`, `低库存`, unknown command and repeated event.
- Verify 0/1/2–5/>5 query results and image fallback.
- Verify inbound, sales outbound, adjustment, low-stock entry and recovery cards.
- Simulate unavailable Feishu API and confirm stock HTTP responses and database state remain successful.

Release:

- Commit and push the exact tested state.
- Follow `.trellis/spec/deployment/aliyun-release.md`.
- Add the three `LARK_*` values to the protected server environment without printing them.
- Build locally, upload one immutable release archive, atomically switch, restart API/Web and run health checks.
- Inspect API logs for long-connection readiness and confirm `NRestarts=0`.

Rollback:

- Remove or blank all three `LARK_*` variables and restart API to disable the integration.
- If the release itself is unhealthy, restore the previous release symlink and restart both services.
