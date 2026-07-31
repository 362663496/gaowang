# Feishu Inventory Integration

## Scenario: Fixed-group inventory bot

### 1. Scope / Trigger

Use this contract when changing the Feishu self-built app bot, inventory-operation notifications, product/stock query commands, or its production configuration.

### 2. Signatures

- Process entry: `services.RunLarkBot(ctx, cfg, db)`.
- Notification entry: `services.NewLarkNotifier(cfg, db)`; disabled configuration returns `nil`.
- Event: `im.message.receive_v1` over the official SDK long connection.
- Commands: `查库存 <名称或编码>`, `查商品 <名称或编码>`, `低库存`, and `帮助`.
- Required app scopes: `im:message:send_as_bot`, `im:message.group_at_msg:readonly`, and `im:resource`.

### 3. Contracts

- `LARK_APP_ID`, `LARK_APP_SECRET`, and `LARK_CHAT_ID` are server-only environment variables and must be all empty or all present.
- The enabled bot handles only group text messages that mention it and whose `chat_id` equals `LARK_CHAT_ID`; private messages and other groups are ignored.
- Query results exclude archived products, match name or code, show at most five rows, and calculate value as quantity times the product's current purchase price.
- Successful inbound, sales-outbound, and adjustment operations enqueue one best-effort notification after commit. Feishu network work never runs inside the inventory transaction.
- Long connection mode needs outbound internet access but no callback URL, verification token, encrypt key, or Nginx route.

### 4. Validation & Error Matrix

| Condition | Behavior |
| --- | --- |
| All three `LARK_*` values are empty | API starts with the integration disabled |
| Only some `LARK_*` values are present | Configuration load fails without exposing any value |
| Wrong group, private message, no mention, duplicate message, or unsupported type | Ignore or return the short help response as defined by the command policy; do not query or mutate stock |
| Query/audit/image upload/reply fails | Log once at the owning boundary; image may degrade to no image; stock remains unchanged |
| Notification queue is full or Feishu send ultimately fails | Drop/log the notification without changing the committed inventory result or HTTP response |

### 5. Good / Base / Bad Cases

- Good: a user mentions the bot in the configured group with `查库存 SKU`, receives current stock and prices, and the query creates a non-secret audit record.
- Base: all Feishu variables are empty, so local tests and the API operate exactly as before.
- Bad: sending to Feishu synchronously inside `InventoryService` makes an external outage roll back or delay stock accounting.

### 6. Tests Required

- Configuration tests assert disabled, enabled, partial-invalid, and secret-redaction behavior.
- Policy/parser tests cover the fixed group, mention requirement, commands, missing arguments, unknown commands, and duplicate messages.
- Query/card tests cover 0, 1, 2–5, and more-than-5 results; name/code matching; current-price valuation; disabled/archived products; and image fallback.
- Inventory handler/service tests assert one post-commit notification per successful operation and no notification for rejected operations.
- Queue and messenger tests assert non-blocking saturation, idempotency UUIDs, retry bounds, and unchanged inventory outcomes on send failure.

### 7. Wrong vs Correct

Wrong: add a public webhook endpoint, store app credentials in frontend configuration, accept messages from any group, or make inventory success depend on Feishu.

Correct: keep credentials in the server environment, use the SDK long connection, enforce one configured `chat_id`, process only fixed read-only commands, and isolate all messaging behind bounded best-effort queues.
