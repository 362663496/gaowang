# Feishu Inventory Integration

## Scenario: Fixed-group inventory bot

### 1. Scope / Trigger

Use this contract when changing the Feishu self-built app bot, inventory-operation notifications, product/stock query commands, or its production configuration.

### 2. Signatures

- Process entry: `services.RunLarkBot(ctx, cfg, db)`.
- Notification entry: `services.NewLarkNotifier(cfg, db)`; disabled configuration returns `nil`.
- Event: `im.message.receive_v1` over the official SDK long connection.
- Commands: `查库存 <名称或编码>`, `低库存`, `库存概览`, `查流水 [名称或编码]`, `今日变动`, and `帮助`; legacy `查商品` returns a migration hint.
- Optional AI parser: `POST https://api.deepseek.com/chat/completions` maps unmatched text to the read-only inventory intent whitelist. The request uses `model`, two `messages` (system + mention-stripped user text), `thinking.type=disabled`, `response_format.type=json_object`, and a bounded `max_tokens`.
- Accepted model result: one strict JSON object containing only `intent` and optional `keyword`; supported intents are `inventory`, `low_stock`, `inventory_summary`, `movements`, `today_changes`, `help`, and `unknown`.
- Required app scopes: `im:message:send_as_bot`, `im:message.group_at_msg:readonly`, and `im:resource`.

### 3. Contracts

- `LARK_APP_ID`, `LARK_APP_SECRET`, and `LARK_CHAT_ID` are server-only environment variables and must be all empty or all present.
- `DEEPSEEK_API_KEY` is optional and server-only; `DEEPSEEK_MODEL` defaults to `deepseek-v4-flash`. An empty key disables only natural-language parsing.
- The enabled bot handles only group text messages that mention it and whose `chat_id` equals `LARK_CHAT_ID`; private messages and other groups are ignored.
- Fixed commands are parsed locally before AI. AI receives only mention-stripped user text; local code validates its JSON, runs parameterized GORM queries, calculates values, and builds cards. Model output is never used as SQL, a field name, a sort expression, or a write instruction.
- Product queries exclude archived products, include products without snapshots as quantity zero, match name or code, show at most five rows, and calculate value as quantity times the product's current purchase price.
- Every concrete product row attempts to show its own compact right-side image; one image failure degrades only that image.
- Successful inbound, sales-outbound, and adjustment operations enqueue one best-effort notification after commit. Feishu network work never runs inside the inventory transaction.
- Long connection mode needs outbound internet access but no callback URL, verification token, encrypt key, or Nginx route.

### 4. Validation & Error Matrix

| Condition | Behavior |
| --- | --- |
| All three `LARK_*` values are empty | API starts with the integration disabled |
| Only some `LARK_*` values are present | Configuration load fails without exposing any value |
| DeepSeek key is empty, times out, is rate-limited, or returns invalid JSON | Fixed commands continue; unmatched natural language receives a short fallback card |
| Wrong group, private message, no mention, duplicate message, or unsupported type | Ignore or return the short help response as defined by the command policy; do not query or mutate stock |
| Query/audit/image upload/reply fails | Log once at the owning boundary; image may degrade to no image; stock remains unchanged |
| Notification queue is full or Feishu send ultimately fails | Drop/log the notification without changing the committed inventory result or HTTP response |

### 5. Good / Base / Bad Cases

- Good: a user mentions the bot with `查库存 SKU` or asks `SKU 还有多少？`, receives current stock, current prices, status, and the matching compact product image, and the resolved query creates a non-secret audit record.
- Base: all Feishu variables are empty, so local tests and the API operate exactly as before.
- Bad: sending to Feishu synchronously inside `InventoryService`, sending inventory data to DeepSeek, or executing model-generated SQL makes external behavior affect stock accounting or data access.

### 6. Tests Required

- Configuration tests assert disabled, enabled, partial-invalid, and secret-redaction behavior.
- Policy/parser tests cover the fixed group, mention requirement, fixed-command priority, at least 20 common natural-language messages, missing arguments, unknown commands, invalid AI output, and duplicate messages.
- Query/card tests cover 0, 1, 2–5, and more-than-5 results; missing snapshots; name/code matching; summaries; recent/today movements; current-price valuation; disabled/archived products; compact per-product images; and image fallback.
- Inventory handler/service tests assert one post-commit notification per successful operation and no notification for rejected operations.
- Queue and messenger tests assert non-blocking saturation, idempotency UUIDs, retry bounds, and unchanged inventory outcomes on send failure.

### 7. Wrong vs Correct

Wrong: add a public webhook endpoint, store app credentials in frontend configuration, accept messages from any group, let the model generate SQL/tool calls, or make inventory success depend on Feishu.

Correct: keep credentials in the server environment, use the SDK long connection, enforce one configured `chat_id`, parse fixed commands locally, map other text through a strict read-only intent whitelist, and isolate all messaging behind bounded best-effort queues.
