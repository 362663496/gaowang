package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gaowang/apps/api/internal/models"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larksafety "github.com/larksuite/oapi-sdk-go/v3/channel/safety"
	larktypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
)

func Test_Lark_parse_command_supports_fixed_commands(t *testing.T) {
	mention := larktypes.Mention{Key: "@_user_1", IsBot: true}
	tests := []struct {
		name    string
		message larktypes.NormalizedMessage
		action  string
		keyword string
	}{
		{name: "inventory", message: larktypes.NormalizedMessage{RawContentType: "text", Content: "@_user_1 查库存 绿茶", Mentions: []larktypes.Mention{mention}}, action: larkActionInventory, keyword: "绿茶"},
		{name: "product", message: larktypes.NormalizedMessage{RawContentType: "text", Content: "@_user_1 查商品 SKU-1", Mentions: []larktypes.Mention{mention}}, action: larkActionProduct, keyword: "SKU-1"},
		{name: "low stock", message: larktypes.NormalizedMessage{RawContentType: "text", Content: "@_user_1 低库存", Mentions: []larktypes.Mention{mention}}, action: larkActionLowStock},
		{name: "help", message: larktypes.NormalizedMessage{RawContentType: "text", Content: "@_user_1 帮助", Mentions: []larktypes.Mention{mention}}, action: larkActionHelp},
		{name: "missing keyword", message: larktypes.NormalizedMessage{RawContentType: "text", Content: "@_user_1 查库存", Mentions: []larktypes.Mention{mention}}, action: larkActionHelp},
		{name: "unknown", message: larktypes.NormalizedMessage{RawContentType: "text", Content: "@_user_1 盘点", Mentions: []larktypes.Mention{mention}}, action: larkActionHelp},
		{name: "low stock with argument", message: larktypes.NormalizedMessage{RawContentType: "text", Content: "@_user_1 低库存 茶", Mentions: []larktypes.Mention{mention}}, action: larkActionHelp, keyword: "茶"},
		{name: "unsupported message", message: larktypes.NormalizedMessage{RawContentType: "image", Content: "[image]", Mentions: []larktypes.Mention{mention}}, action: larkActionHelp},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := parseLarkCommand(test.message)
			if command.Action != test.action || command.Keyword != test.keyword {
				t.Fatalf("command = %+v, want action=%s keyword=%q", command, test.action, test.keyword)
			}
		})
	}
}

func Test_Lark_policy_allows_only_mentioned_messages_from_fixed_group(t *testing.T) {
	policy := larkPolicy("oc_inventory")
	gate := larksafety.NewPolicyGate(&policy, nil)
	tests := []struct {
		name    string
		message larktypes.NormalizedMessage
		allowed bool
	}{
		{name: "fixed group mention", message: larktypes.NormalizedMessage{ChatID: "oc_inventory", ChatType: "group", MentionedBot: true}, allowed: true},
		{name: "other group", message: larktypes.NormalizedMessage{ChatID: "oc_other", ChatType: "group", MentionedBot: true}},
		{name: "without mention", message: larktypes.NormalizedMessage{ChatID: "oc_inventory", ChatType: "group"}},
		{name: "mention all", message: larktypes.NormalizedMessage{ChatID: "oc_inventory", ChatType: "group", MentionAll: true}},
		{name: "private chat", message: larktypes.NormalizedMessage{ChatType: "p2p", MentionedBot: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if decision := gate.Evaluate(&test.message); decision.Allowed != test.allowed {
				t.Fatalf("decision = %+v, want allowed=%t", decision, test.allowed)
			}
		})
	}
}

func Test_Lark_query_filters_sorts_and_limits_results(t *testing.T) {
	db := newInventoryTestDB(t)
	for index := 0; index < 7; index++ {
		product := models.Product{
			Name:              "商品" + string(rune('A'+index)),
			Code:              "SKU-" + string(rune('A'+index)),
			LowStockThreshold: 4,
			Enabled:           true,
		}
		if err := db.Create(&product).Error; err != nil {
			t.Fatalf("create product: %v", err)
		}
		if err := db.Create(&models.InventorySnapshot{ProductID: product.ID, Quantity: int64(index)}).Error; err != nil {
			t.Fatalf("create snapshot: %v", err)
		}
	}
	noSnapshot := models.Product{Name: "无快照", Code: "NO-SNAPSHOT", Enabled: true}
	archivedAt := time.Now()
	archived := models.Product{Name: "归档商品", Code: "SKU-ARCHIVED", Enabled: false, ArchivedAt: &archivedAt}
	for _, product := range []*models.Product{&noSnapshot, &archived} {
		if err := db.Create(product).Error; err != nil {
			t.Fatalf("create product: %v", err)
		}
	}

	rows, more, err := queryLarkProducts(db, larkCommand{Action: larkActionInventory, Keyword: "sku-"})
	if err != nil {
		t.Fatalf("query inventory: %v", err)
	}
	if len(rows) != 5 || !more || rows[0].Code != "SKU-A" || rows[4].Code != "SKU-E" {
		t.Fatalf("inventory rows = %+v more=%t", rows, more)
	}

	rows, more, err = queryLarkProducts(db, larkCommand{Action: larkActionInventory, Keyword: "sku-d"})
	if err != nil || more || len(rows) != 1 || rows[0].Quantity != 3 {
		t.Fatalf("unique inventory rows = %+v more=%t err=%v", rows, more, err)
	}

	rows, _, err = queryLarkProducts(db, larkCommand{Action: larkActionProduct, Keyword: "no-snapshot"})
	if err != nil || len(rows) != 1 || rows[0].Quantity != 0 {
		t.Fatalf("product rows = %+v err=%v", rows, err)
	}

	rows, more, err = queryLarkProducts(db, larkCommand{Action: larkActionLowStock})
	if err != nil || more || len(rows) != 5 {
		t.Fatalf("low-stock rows = %+v more=%t err=%v", rows, more, err)
	}

	rows, more, err = queryLarkProducts(db, larkCommand{Action: larkActionInventory, Keyword: "missing"})
	if err != nil || more || len(rows) != 0 {
		t.Fatalf("missing rows = %+v more=%t err=%v", rows, more, err)
	}
}

func Test_Lark_cards_show_current_prices_and_stock_transition(t *testing.T) {
	product := models.Product{
		Name: "绿茶", Code: "TEA-1", DefaultPurchaseCents: 123, DefaultSaleCents: 456,
		LowStockThreshold: 5, Enabled: true,
	}
	productCard, err := larkProductCard(larkProductRow{Product: product, Quantity: 3}, "img_test")
	if err != nil {
		t.Fatalf("build product card: %v", err)
	}
	for _, expected := range []string{"img_test", "绿茶", "TEA-1", "¥1.23", "¥4.56", "¥3.69", "低库存"} {
		if !strings.Contains(productCard, expected) {
			t.Fatalf("product card does not contain %q: %s", expected, productCard)
		}
	}
	messenger := newLarkMessenger(lark.NewClient("cli_test", "secret"), t.TempDir(), "oc_test")
	if imageKey := messenger.imageKey(context.Background(), "/uploads/missing.png"); imageKey != "" {
		t.Fatalf("missing image key = %q, want empty fallback", imageKey)
	}

	change := InventoryChange{
		Product: product,
		Movement: models.StockMovement{
			Type: models.MovementTypeSalesOutbound, QuantityDelta: -1,
			Operator: models.User{Name: "操作员"}, CreatedAt: time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC),
		},
		QuantityBefore: 6,
		QuantityAfter:  5,
	}
	notification, err := larkInventoryChangeCard(change, "")
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}
	if !strings.Contains(notification, `"template":"orange"`) || !strings.Contains(notification, "已进入低库存") || !strings.Contains(notification, "操作员") {
		t.Fatalf("low-stock notification = %s", notification)
	}

	change.QuantityBefore, change.QuantityAfter, change.Movement.QuantityDelta = 5, 6, 1
	notification, err = larkInventoryChangeCard(change, "")
	if err != nil || !strings.Contains(notification, `"template":"green"`) || !strings.Contains(notification, "已恢复正常") {
		t.Fatalf("recovery notification = %s err=%v", notification, err)
	}

	change.QuantityBefore, change.QuantityAfter, change.Movement.QuantityDelta = 5, 4, -1
	notification, err = larkInventoryChangeCard(change, "")
	if err != nil || !strings.Contains(notification, `"template":"blue"`) ||
		strings.Contains(notification, "已进入低库存") || strings.Contains(notification, "已恢复正常") {
		t.Fatalf("continuing low-stock notification = %s err=%v", notification, err)
	}

	change.Product.LowStockThreshold = 0
	change.QuantityBefore, change.QuantityAfter = 1, 0
	notification, err = larkInventoryChangeCard(change, "")
	if err != nil || !strings.Contains(notification, `"template":"blue"`) || strings.Contains(notification, "库存状态") {
		t.Fatalf("zero-threshold notification = %s err=%v", notification, err)
	}
}

func Test_Lark_audit_records_query_without_actor(t *testing.T) {
	db := newInventoryTestDB(t)
	bot := larkBot{db: db}
	message := larktypes.NormalizedMessage{MessageID: "om_test", ChatID: "oc_test", UserID: "ou_test"}
	bot.recordAudit(message, larkCommand{Action: larkActionInventory, Name: "查库存", Keyword: "TEA"})

	var audit models.AuditLog
	if err := db.First(&audit).Error; err != nil {
		t.Fatalf("load audit: %v", err)
	}
	if audit.ActorID != nil || audit.Action != larkActionInventory || audit.ResourceType != "lark_message" || audit.ResourceID != "om_test" || audit.IPAddress != "" {
		t.Fatalf("audit = %+v", audit)
	}
	var metadata map[string]string
	if err := json.Unmarshal(audit.Metadata, &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata["chat_id"] != "oc_test" || metadata["sender_open_id"] != "ou_test" || metadata["keyword"] != "TEA" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func Test_Lark_messenger_sends_idempotent_cards_to_chat_and_reply(t *testing.T) {
	mock := &larkMockHTTPClient{}
	client := lark.NewClient("cli_test", "secret", lark.WithHttpClient(mock))
	messenger := newLarkMessenger(client, "", "oc_inventory")

	if err := messenger.sendCard(context.Background(), `{"elements":[]}`, ""); err != nil {
		t.Fatalf("send notification: %v", err)
	}
	if err := messenger.sendCard(context.Background(), `{"elements":[]}`, "om_source"); err != nil {
		t.Fatalf("send reply: %v", err)
	}
	if len(mock.payloads) != 2 {
		t.Fatalf("request count = %d, want 2", len(mock.payloads))
	}
	for index, payload := range mock.payloads {
		if payload["uuid"] == "" || payload["msg_type"] != "interactive" {
			t.Fatalf("payload %d = %+v", index, payload)
		}
	}
	if mock.payloads[0]["receive_id"] != "oc_inventory" {
		t.Fatalf("notification payload = %+v", mock.payloads[0])
	}
}

func Test_Lark_notifier_queue_is_bounded_and_nonblocking(t *testing.T) {
	notifier := &LarkNotifier{jobs: make(chan InventoryChange, 1)}
	done := make(chan struct{})
	go func() {
		notifier.Enqueue(InventoryChange{})
		notifier.Enqueue(InventoryChange{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Enqueue blocked on a full queue")
	}
	if len(notifier.jobs) != 1 {
		t.Fatalf("queued notifications = %d, want 1", len(notifier.jobs))
	}
}

type larkMockHTTPClient struct {
	payloads []map[string]string
}

func (m *larkMockHTTPClient) Do(request *http.Request) (*http.Response, error) {
	payload := map[string]string{}
	if request.Body != nil {
		data, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(data, &payload)
	}
	if payload["msg_type"] != "" {
		m.payloads = append(m.payloads, payload)
	}
	body := `{"code":0,"msg":"success","data":{"message_id":"om_test","chat_id":"oc_inventory"}}`
	if strings.Contains(request.URL.Path, "auth") {
		body = `{"code":0,"msg":"success","tenant_access_token":"token","expire":7200}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}
