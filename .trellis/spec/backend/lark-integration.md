# Feishu Inventory Integration

## Scenario: Fixed-group inventory bot

### 1. Scope / Trigger

Use this contract when changing the Feishu self-built app bot, inventory-operation notifications, natural-language inventory queries, product cards, or production configuration.

### 2. Signatures

- Process entry: `services.RunLarkBot(ctx, cfg, db)`.
- Notification entry: `services.NewLarkNotifier(cfg, db)`; disabled configuration returns `nil`.
- Event: `im.message.receive_v1` over the official SDK long connection.
- AI parser: `POST https://api.deepseek.com/chat/completions` receives every non-empty mention-stripped text message. The request uses `model`, system/user messages, `thinking.type=disabled`, `response_format.type=json_object`, and bounded `max_tokens`.
- Accepted model result: one strict JSON object containing `intent`, optional `keyword`, or a validated read-only `analytics` plan. The model supplies neither SQL nor result values/card JSON.
- Direct read-only intents: `inventory`, `low_stock`, `out_of_stock`, `inventory_summary`, `movements`, `today_changes`, `analytics`, `help`, and `unknown`.
- `analytics` plan: `domain`, `metric`, `operation`, up to two `group_by` dimensions, `movement_type`, extended date/range fields, trend/comparison fields, `sort`, `limit`, `presentation`, and bounded product/shop/operator filters.
- Required app scopes: `im:message:send_as_bot`, `im:message.group_at_msg:readonly`, and `im:resource`.

### 3. Contracts

- `LARK_APP_ID`, `LARK_APP_SECRET`, and `LARK_CHAT_ID` are server-only environment variables and must be all empty or all present.
- `DEEPSEEK_API_KEY` is optional and server-only; `DEEPSEEK_MODEL` defaults to `deepseek-v4-flash`. When the key is empty, notifications still work but non-empty query messages return an AI-unavailable card.
- The enabled bot handles only group text messages that mention it and whose `chat_id` equals `LARK_CHAT_ID`; private messages and other groups are ignored.
- There is no fixed phrase parser or local command priority. Empty mention text can show local help; every non-empty message is classified through DeepSeek, including phrases such as `帮助` and `查库存 编码`. A failed `plan_validate` may consume the single correction request described below.
- Local code validates the strict JSON result, runs parameterized GORM queries, calculates values, and builds cards. Model output is never used as SQL, field names, sort expressions, or write instructions. Natural-language phrasing is unrestricted, but the readable dataset is limited to products, current inventory, shops, stock movements, and historical operator names; account fields, permissions, audits, backups, and settings are outside the dataset.
- `inventory` requires a non-empty keyword. `keyword` is accepted only for `inventory`, `low_stock`, `out_of_stock`, and `movements`.
- Analytics domains are product, current inventory, shop, movement, and historical operator. Metrics remain `inventory_quantity`, `inventory_value`, `movement_quantity`, `movement_count`, and `movement_value`; operations are total, details, ranking, trend, share, and comparison.
- Grouping accepts zero, one, or two unique dimensions from product/shop/operator. Rankings allow two dimensions; shares one; trends/comparisons at most one. Three dimensions are rejected rather than silently dropped.
- Movement types are `all`, `inbound`, `sales_outbound`, and `adjustment`. Time ranges include today/yesterday, current/previous week, current/previous month, current year, 1–365 recent days, inclusive explicit dates, and all history; current-inventory metrics use `current` only.
- Current-inventory analytics allow only no grouping or product grouping. Movement analytics may group or filter by product, shop, and operator. Missing movement time defaults to all history; missing grouped sort/limit defaults to descending/top five.
- `last_n_days` accepts 1–365 inclusive and includes today. Cards show at most 10 deterministic rows; aggregate totals and share denominators use all matching rows.
- Trend buckets are day/month. Comparison uses the immediately previous equivalent period or the same interval in the previous year. All explicit dates and natural periods use inclusive Shanghai calendar semantics and UTC query boundaries.
- Product queries exclude archived products, include missing snapshots as quantity zero, and sort deterministically. Historical product movement statistics retain archived products; shop/operator grouping counts only movements with the corresponding association.
- Movement quantity is positive for inbound, absolute for sales outbound, signed for adjustment, and signed net change for `all`. Every monetary result is labeled as current-purchase-price inventory value or estimated purchase/outbound cost, never revenue, gross profit, a movement-stored price, or sale price.
- Product/inventory details expose business fields and current stock; shop details expose name/status/note; movement details expose product, type, quantity, shop, operator, time, and note; historical operator results expose names only. Explicit soft-deleted users remain visible through retained movement history without email/role/permission data.
- Presentation uses the known server templates (`auto`, summary, detail, ranking, matrix, trend, share, comparison). When metric/grouping already determines total versus ranking, conflicting operation labels are canonicalized; a conflicting presentation becomes `auto`. Canonicalization never drops filters, grouping dimensions, or time constraints.
- DeepSeek parsing has a 10-second total deadline and at most two requests. Transport/timeouts, 429, and 5xx retry once; a `plan_validate` failure may use the second request to correct enum or label conflicts. Other HTTP failures, malformed responses, and malformed plan JSON do not retry. Every corrected plan passes the same dataset and parameter validation. Structured logs contain only message ID, stage, status, attempts, and elapsed time.
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
| DeepSeek key is empty, transport times out, or 429/5xx remains after one retry | Return an upstream-service-unavailable card; do not execute a query or stock operation |
| First model plan has an enum or label conflict | Request one correction within the same two-request/10-second budget; execute only if the corrected plan passes all checks |
| Model response is malformed, the corrected plan remains unsupported, or it requests more than two dimensions | Return a distinct not-understood/unsupported card; do not execute a query or stock operation |
| Unknown intent, missing required keyword, forbidden keyword, or unknown JSON field | Reject the model result; do not execute arbitrary data access |
| Missing/unknown domain, required metric, group, date, comparison, unresolved operation, incompatible data combination, limit outside 1–10, or an overlong filter | Reject the complete analytics plan; do not fall back to a different dataset or drop requested conditions |
| Valid grouped query has no rows | Return an empty-statistics card for the requested dimension; do not substitute another dimension |
| Wrong group, private message, no mention, or duplicate message | Ignore it; do not query or mutate stock |
| Query/audit/image upload/reply fails | Log once at the owning boundary; an image may degrade to no image; stock remains unchanged |
| Notification queue is full or Feishu send ultimately fails | Drop/log the notification without changing the committed inventory result or HTTP response |

### 5. Good / Base / Bad Cases

- Good: `帮我找一下编码 BR-1214G 的货` returns current quantity, purchase price, status, and image. `哪个店铺出货最多` becomes a shop ranking. `今天按商品排序并列出店铺各占多少` becomes an intact product × shop matrix. Trend/share/comparison plans remain server-computed.
- Base: all Feishu variables are empty, so local tests and the API operate exactly as before.
- Bad: translating a shop question to a product ranking, matching Chinese substrings before AI, returning sale price, sending inventory data to DeepSeek, executing model-generated SQL, or making inventory success depend on Feishu.

### 6. Tests Required

- Configuration tests assert disabled, enabled, partial-invalid, and secret-redaction behavior.
- Policy/AI tests cover the fixed group, mention requirement, at least 40 natural-language messages, two-dimensional and extended plans, the invalid-plan matrix, missing AI key, duplicate messages, and proof that non-empty fixed-looking phrases still call DeepSeek rather than a local phrase parser.
- Retry tests cover transport timeout, 429, 5xx, one plan-validation correction, a second invalid plan, non-retryable 4xx/malformed response/plan JSON, caller deadline, attempts/elapsed metadata, failure-stage classification, and secret/input/response redaction.
- A regression test must assert `哪个店铺出货最多` resolves to `movement_quantity` + `shop` + `sales_outbound` + `all` + `desc` + `1`, and that the query/card returns a shop rather than a product.
- Query/card tests cover 0, 1, up-to-10 and truncated results; missing snapshots; product/shop/operator filters; extended time ranges; current-price valuation; one/two-dimensional ranking, details, trend, share, comparison, stable ordering, archived product/deleted-operator history, images, and image fallback.
- Product-card tests assert purchase price is present, sale price is absent, inventory quantity is prominent, and zero/low/normal stock styles remain distinct.
- Inventory handler/service tests assert one post-commit notification per successful operation and no notification for rejected operations.
- Queue and messenger tests assert non-blocking saturation, idempotency UUIDs, retry bounds, and unchanged inventory outcomes on send failure.

### 7. Wrong vs Correct

Wrong: add one intent per new ranking, branch on `strings.Contains(text, "库存")`, coerce an unsupported dimension to the nearest existing ranking, expose sale price, accept messages from any group, let the model generate SQL/tool calls, or send Feishu messages inside an inventory transaction.

Correct: enforce one configured `chat_id`, map every non-empty message to a composable read-only product-data plan, canonicalize harmless semantic-label conflicts without dropping requested conditions, validate the corrected plan locally, select fixed SQL/GORM branches, preserve requested dimensions, and isolate messaging behind bounded best-effort queues.
