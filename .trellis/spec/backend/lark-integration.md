# Feishu Inventory Integration

## Scenario: Fixed-group inventory bot

### 1. Scope / Trigger

Use this contract when changing the Feishu self-built app bot, inventory-operation notifications, natural-language inventory queries, product cards, or production configuration.

### 2. Signatures

- Process entry: `services.RunLarkBot(ctx, cfg, db)`.
- Notification entry: `services.NewLarkNotifier(cfg, db)`; disabled configuration returns `nil`.
- Event: `im.message.receive_v1` over the official SDK long connection.
- AI parser: `POST https://api.deepseek.com/chat/completions` receives every non-empty mention-stripped text message. The request uses `model`, system/user messages, `thinking.type=disabled`, `response_format.type=json_object`, and bounded `max_tokens`.
- Accepted model result: one strict JSON object containing `intent`, optional `keyword`, or a validated `analytics` plan.
- Direct read-only intents: `inventory`, `low_stock`, `out_of_stock`, `inventory_summary`, `movements`, `today_changes`, `analytics`, `help`, and `unknown`.
- `analytics` plan: `metric`, `group_by`, `movement_type`, `time_range`, `days`, `sort`, `limit`, `product_keyword`, `shop_keyword`, and `operator_keyword`.
- Required app scopes: `im:message:send_as_bot`, `im:message.group_at_msg:readonly`, and `im:resource`.

### 3. Contracts

- `LARK_APP_ID`, `LARK_APP_SECRET`, and `LARK_CHAT_ID` are server-only environment variables and must be all empty or all present.
- `DEEPSEEK_API_KEY` is optional and server-only; `DEEPSEEK_MODEL` defaults to `deepseek-v4-flash`. When the key is empty, notifications still work but non-empty query messages return an AI-unavailable card.
- The enabled bot handles only group text messages that mention it and whose `chat_id` equals `LARK_CHAT_ID`; private messages and other groups are ignored.
- There is no fixed phrase parser or local command priority. Empty mention text can show local help; every non-empty message is classified exactly once by DeepSeek, including phrases such as `帮助` and `查库存 编码`.
- Local code validates the strict JSON result, runs parameterized GORM queries, calculates values, and builds cards. Model output is never used as SQL, field names, sort expressions, or write instructions.
- `inventory` requires a non-empty keyword. `keyword` is accepted only for `inventory`, `low_stock`, `out_of_stock`, and `movements`.
- Analytics metrics are `inventory_quantity`, `inventory_value`, `movement_quantity`, `movement_count`, and `movement_value`; grouping is `none`, `product`, `shop`, or `operator`.
- Movement types are `all`, `inbound`, `sales_outbound`, and `adjustment`. Time ranges are `today`, `yesterday`, `last_n_days`, `current_month`, and `all`; current-inventory metrics use `current` only.
- Current-inventory analytics allow only no grouping or product grouping. Movement analytics may group or filter by product, shop, and operator. Missing movement time defaults to all history; missing grouped sort/limit defaults to descending/top five.
- `last_n_days` accepts 1–365 inclusive and includes today. Grouped analytics return 1–5 rows with stable name/ID tie-breakers; ungrouped analytics return one total.
- Product queries exclude archived products, include missing snapshots as quantity zero, and sort deterministically. Historical product movement statistics retain archived products; shop/operator grouping counts only movements with the corresponding association.
- Movement quantity is positive for inbound, absolute for sales outbound, signed for adjustment, and signed net change for `all`. Every monetary result uses the current product purchase price, never a movement-stored price or sale price.
- Product cards show current purchase price only, never sale price. Inventory quantity is the primary bold, icon-prefixed value; zero, low, and normal stock have distinct status/color treatments.
- Every result that represents a concrete product attempts to show that product's compact right-side image. One image failure degrades only that image.
- Successful inbound, sales-outbound, and adjustment operations enqueue one best-effort notification after commit. Feishu network work never runs inside the inventory transaction.
- Long connection mode needs outbound internet access but no callback URL, verification token, encrypt key, or Nginx route.

### 4. Validation & Error Matrix

| Condition | Behavior |
| --- | --- |
| All three `LARK_*` values are empty | API starts with the integration disabled |
| Only some `LARK_*` values are present | Configuration load fails without exposing any value |
| Empty mention text or unsupported message type | Return local help; do not call DeepSeek or query/mutate stock |
| DeepSeek key is empty, times out, is rate-limited, or returns invalid JSON | Return a short AI-unavailable/error card; do not execute a query or stock operation |
| Unknown intent, missing required keyword, forbidden keyword, or unknown JSON field | Reject the model result; do not execute arbitrary data access |
| Missing/unknown metric or group, incompatible inventory grouping/time/type, invalid movement type/time, days outside 1–365, sort outside `asc`/`desc`, limit outside 1–5, or an overlong filter | Reject the complete analytics plan; do not fall back to a similar ranking |
| Valid grouped query has no rows | Return an empty-statistics card for the requested dimension; do not substitute another dimension |
| Wrong group, private message, no mention, or duplicate message | Ignore it; do not query or mutate stock |
| Query/audit/image upload/reply fails | Log once at the owning boundary; an image may degrade to no image; stock remains unchanged |
| Notification queue is full or Feishu send ultimately fails | Drop/log the notification without changing the committed inventory result or HTTP response |

### 5. Good / Base / Bad Cases

- Good: `帮我找一下编码 BR-1214G 的货` returns current quantity, purchase price, status, and image. `哪个店铺出货最多` becomes sales-outbound quantity grouped by shop across all history, descending, limit one. `本月谁操作次数最多` becomes a current-month operator count ranking.
- Base: all Feishu variables are empty, so local tests and the API operate exactly as before.
- Bad: translating a shop question to a product ranking, matching Chinese substrings before AI, returning sale price, sending inventory data to DeepSeek, executing model-generated SQL, or making inventory success depend on Feishu.

### 6. Tests Required

- Configuration tests assert disabled, enabled, partial-invalid, and secret-redaction behavior.
- Policy/AI tests cover the fixed group, mention requirement, at least 40 natural-language messages across every direct intent and plan dimension, the full invalid-plan matrix, missing AI key, duplicate messages, and proof that non-empty fixed-looking phrases still call DeepSeek exactly once.
- A regression test must assert `哪个店铺出货最多` resolves to `movement_quantity` + `shop` + `sales_outbound` + `all` + `desc` + `1`, and that the query/card returns a shop rather than a product.
- Query/card tests cover 0, 1, 2–5, and more-than-5 results; missing snapshots; product/shop/operator filters; every supported time range; current-price valuation; stable ranking; archived-product history; one image per concrete product; and image fallback.
- Product-card tests assert purchase price is present, sale price is absent, inventory quantity is prominent, and zero/low/normal stock styles remain distinct.
- Inventory handler/service tests assert one post-commit notification per successful operation and no notification for rejected operations.
- Queue and messenger tests assert non-blocking saturation, idempotency UUIDs, retry bounds, and unchanged inventory outcomes on send failure.

### 7. Wrong vs Correct

Wrong: add one intent per new ranking, branch on `strings.Contains(text, "库存")`, coerce an unsupported dimension to the nearest existing ranking, expose sale price, accept messages from any group, let the model generate SQL/tool calls, or send Feishu messages inside an inventory transaction.

Correct: enforce one configured `chat_id`, map every non-empty message once to a small direct-intent set or a composable read-only plan, validate every field and combination locally, select fixed SQL/GORM branches, preserve the requested dimension, present purchase-price-only product cards with prominent quantity and images, and isolate messaging behind bounded best-effort queues.
