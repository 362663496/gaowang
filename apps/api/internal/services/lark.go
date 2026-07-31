package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkchannel "github.com/larksuite/oapi-sdk-go/v3/channel"
	larkoutbound "github.com/larksuite/oapi-sdk-go/v3/channel/outbound"
	larktypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	larkQueueSize       = 64
	larkResultLimit     = 5
	larkKeywordMaxRunes = 100

	larkActionInventory         = "lark.query_inventory"
	larkActionLowStock          = "lark.query_low_stock"
	larkActionOutOfStock        = "lark.query_out_of_stock"
	larkActionSummary           = "lark.query_summary"
	larkActionValueRanking      = "lark.query_inventory_value_ranking"
	larkActionMovements         = "lark.query_movements"
	larkActionTodayChanges      = "lark.query_today_changes"
	larkActionTodaySalesRanking = "lark.query_today_sales_ranking"
	larkActionHelp              = "lark.help"
	larkActionUnknown           = "lark.unknown"
)

var larkLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type LarkNotifier struct {
	db        *gorm.DB
	jobs      chan InventoryChange
	messenger *larkMessenger
}

// NewLarkNotifier returns nil when the integration is disabled.
func NewLarkNotifier(cfg config.Config, db *gorm.DB) *LarkNotifier {
	if !cfg.LarkEnabled() {
		return nil
	}
	client := lark.NewClient(cfg.LarkAppID, cfg.LarkAppSecret)
	notifier := &LarkNotifier{
		db:        db,
		jobs:      make(chan InventoryChange, larkQueueSize),
		messenger: newLarkMessenger(client, cfg.UploadDir, cfg.LarkChatID),
	}
	// ponytail: delivery is process-local; use a DB outbox only if guaranteed delivery becomes required.
	go notifier.run()
	return notifier
}

func (n *LarkNotifier) Enqueue(change InventoryChange) {
	if n == nil {
		return
	}
	select {
	case n.jobs <- change:
	default:
		slog.Warn("drop lark inventory notification: queue full",
			slog.String("movement_id", change.Movement.ID.String()),
			slog.String("product_id", change.Product.ID.String()))
	}
}

func (n *LarkNotifier) run() {
	for change := range n.jobs {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		n.notify(ctx, change)
		cancel()
	}
}

func (n *LarkNotifier) notify(ctx context.Context, change InventoryChange) {
	movement := change.Movement
	if err := n.db.Preload("Operator").Preload("Shop").First(&movement, "id = ?", movement.ID).Error; err != nil {
		slog.Warn("load lark notification details", slog.Any("err", err), slog.String("movement_id", movement.ID.String()))
		return
	}
	change.Movement = movement
	imageKey := n.messenger.imageKey(ctx, change.Product.ImagePath)
	card, err := larkInventoryChangeCard(change, imageKey)
	if err != nil {
		slog.Warn("build lark inventory notification", slog.Any("err", err))
		return
	}
	if err := n.messenger.sendCard(ctx, card, ""); err != nil {
		slog.Warn("send lark inventory notification", slog.Any("err", err), slog.String("movement_id", movement.ID.String()))
	}
}

type larkBot struct {
	db           *gorm.DB
	jobs         chan larktypes.NormalizedMessage
	messenger    *larkMessenger
	intentParser *deepSeekIntentParser
}

func RunLarkBot(ctx context.Context, cfg config.Config, db *gorm.DB) error {
	if !cfg.LarkEnabled() {
		return nil
	}

	client := lark.NewClient(cfg.LarkAppID, cfg.LarkAppSecret)
	eventHandler := dispatcher.NewEventDispatcher("", "")
	wsClient := larkws.NewClient(cfg.LarkAppID, cfg.LarkAppSecret, larkws.WithEventHandler(eventHandler))

	channelCfg := larktypes.DefaultChannelConfig()
	channelCfg.Safety.Batch.DelayMs = 0
	channel := larkchannel.NewChannel(
		client,
		wsClient,
		larktypes.WithPolicyConfig(larkPolicy(cfg.LarkChatID)),
		larktypes.WithSafetyConfig(channelCfg.Safety),
	)
	bot := &larkBot{
		db:           db,
		jobs:         make(chan larktypes.NormalizedMessage, larkQueueSize),
		messenger:    newLarkMessenger(client, cfg.UploadDir, cfg.LarkChatID),
		intentParser: newDeepSeekIntentParser(cfg.DeepSeekAPIKey, cfg.DeepSeekModel),
	}
	go bot.run(ctx)
	channel.OnMessage(func(_ context.Context, message *larktypes.NormalizedMessage) error {
		bot.enqueue(message)
		return nil
	})
	channel.OnReady(func() {
		slog.Info("lark bot connected")
	})
	channel.OnError(func(err error) {
		slog.Warn("lark bot connection error", slog.Any("err", err))
	})
	return channel.Start(ctx)
}

func larkPolicy(chatID string) larktypes.PolicyConfig {
	requireMention := true
	respondToMentionAll := false
	return larktypes.PolicyConfig{
		GroupAllowlist:      []string{chatID},
		RequireMention:      &requireMention,
		RespondToMentionAll: &respondToMentionAll,
		DMMode:              "disabled",
	}
}

func (b *larkBot) enqueue(message *larktypes.NormalizedMessage) {
	if message == nil || message.MessageID == "" {
		return
	}
	select {
	case b.jobs <- *message:
	default:
		slog.Warn("drop lark query: queue full", slog.String("message_id", message.MessageID))
	}
}

func (b *larkBot) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-b.jobs:
			jobCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			b.handle(jobCtx, message)
			cancel()
		}
	}
}

type larkCommand struct {
	Action  string
	Name    string
	Keyword string
}

func larkMessageText(message larktypes.NormalizedMessage) string {
	content := message.Content
	for _, mention := range message.Mentions {
		if mention.IsBot && mention.Key != "" {
			content = strings.ReplaceAll(content, mention.Key, "")
		}
	}
	return strings.TrimSpace(content)
}

func (b *larkBot) resolveCommand(ctx context.Context, message larktypes.NormalizedMessage) (larkCommand, error) {
	if message.RawContentType != "text" || larkMessageText(message) == "" {
		return larkCommand{Action: larkActionHelp, Name: "帮助"}, nil
	}
	if b.intentParser == nil {
		return larkCommand{}, errDeepSeekIntent
	}
	return b.intentParser.Parse(ctx, larkMessageText(message))
}

func (b *larkBot) handle(ctx context.Context, message larktypes.NormalizedMessage) {
	command, resolveErr := b.resolveCommand(ctx, message)

	var (
		card string
		err  error
	)
	if resolveErr != nil {
		card, err = larkAIUnavailableCard()
	} else {
		b.recordAudit(message, command)
	}

	switch {
	case resolveErr != nil:
	case command.Action == larkActionHelp:
		card, err = larkHelpCard()
	case command.Action == larkActionInventory || command.Action == larkActionLowStock ||
		command.Action == larkActionOutOfStock || command.Action == larkActionValueRanking ||
		command.Action == larkActionTodaySalesRanking:
		var rows []larkProductRow
		var more bool
		var queryErr error
		if command.Action == larkActionTodaySalesRanking {
			rows, more, queryErr = queryLarkTodaySalesRanking(b.db, time.Now())
		} else {
			rows, more, queryErr = queryLarkProducts(b.db, command)
		}
		if queryErr != nil {
			slog.Warn("query inventory from lark", slog.Any("err", queryErr), slog.String("message_id", message.MessageID))
			card, err = larkSimpleCard("查询失败", "red", "库存查询暂时失败，请稍后重试。")
		} else {
			paths := make([]string, len(rows))
			for index := range rows {
				paths[index] = rows[index].ImagePath
			}
			card, err = larkQueryCard(command, rows, more, b.messenger.resolveImageKeys(ctx, paths))
		}
	case command.Action == larkActionSummary:
		var summary larkInventorySummary
		summary, err = queryLarkInventorySummary(b.db)
		if err == nil {
			card, err = larkInventorySummaryCard(summary)
		}
	case command.Action == larkActionMovements:
		var movements []models.StockMovement
		var more bool
		movements, more, err = queryLarkMovements(b.db, command.Keyword)
		if err == nil {
			paths := make([]string, len(movements))
			for index := range movements {
				paths[index] = movements[index].Product.ImagePath
			}
			card, err = larkMovementsCard(movements, more, b.messenger.resolveImageKeys(ctx, paths))
		}
	case command.Action == larkActionTodayChanges:
		var changes larkTodayChanges
		changes, err = queryLarkTodayChanges(b.db, time.Now())
		if err == nil {
			card, err = larkTodayChangesCard(changes)
		}
	case command.Action == larkActionUnknown:
		card, err = larkUnknownCard()
	default:
		card, err = larkHelpCard()
	}
	if err != nil {
		slog.Warn("handle lark query", slog.Any("err", err), slog.String("message_id", message.MessageID))
		card, err = larkSimpleCard("查询失败", "red", "库存查询暂时失败，请稍后重试。")
		if err != nil {
			return
		}
	}
	if err := b.messenger.sendCard(ctx, card, message.MessageID); err != nil {
		slog.Warn("reply to lark query", slog.Any("err", err), slog.String("message_id", message.MessageID))
	}
}

func (b *larkBot) recordAudit(message larktypes.NormalizedMessage, command larkCommand) {
	metadata, err := json.Marshal(map[string]string{
		"chat_id":        message.ChatID,
		"sender_open_id": message.UserID,
		"command":        command.Name,
		"keyword":        command.Keyword,
	})
	if err != nil {
		return
	}
	audit := models.AuditLog{
		Action:       command.Action,
		ResourceType: "lark_message",
		ResourceID:   message.MessageID,
		Metadata:     datatypes.JSON(metadata),
	}
	if err := b.db.Create(&audit).Error; err != nil {
		slog.Warn("record lark query audit", slog.Any("err", err), slog.String("message_id", message.MessageID))
	}
}

type larkProductRow struct {
	models.Product `gorm:"embedded"`
	Quantity       int64 `gorm:"column:quantity"`
	SalesQuantity  int64 `gorm:"column:sales_quantity"`
}

func queryLarkProducts(db *gorm.DB, command larkCommand) ([]larkProductRow, bool, error) {
	query := db.Table("products").
		Select("products.*, COALESCE(inventory_snapshots.quantity, 0) AS quantity").
		Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id").
		Where("products.archived_at IS NULL")
	switch command.Action {
	case larkActionLowStock:
		query = query.Where("products.low_stock_threshold > 0 AND COALESCE(inventory_snapshots.quantity, 0) <= products.low_stock_threshold")
	case larkActionOutOfStock:
		query = query.Where("COALESCE(inventory_snapshots.quantity, 0) <= 0")
	case larkActionValueRanking:
		query = query.Where("COALESCE(inventory_snapshots.quantity, 0) > 0")
	}
	if command.Keyword != "" {
		like := "%" + strings.ToLower(command.Keyword) + "%"
		query = query.Where("LOWER(products.name) LIKE ? OR LOWER(products.code) LIKE ?", like, like)
	}

	if command.Action == larkActionValueRanking {
		query = query.Order("COALESCE(inventory_snapshots.quantity, 0) * products.default_purchase_cents DESC")
	}
	var rows []larkProductRow
	err := query.Order("products.name ASC").
		Order("products.code ASC").
		Order("products.id ASC").
		Limit(larkResultLimit + 1).
		Scan(&rows).Error
	if err != nil {
		return nil, false, err
	}
	more := len(rows) > larkResultLimit
	if more {
		rows = rows[:larkResultLimit]
	}
	return rows, more, nil
}

type larkInventorySummary struct {
	ProductCount   int64 `gorm:"column:product_count"`
	Quantity       int64 `gorm:"column:quantity"`
	ValueCents     int64 `gorm:"column:value_cents"`
	LowStockCount  int64 `gorm:"column:low_stock_count"`
	ZeroStockCount int64 `gorm:"column:zero_stock_count"`
}

func queryLarkInventorySummary(db *gorm.DB) (larkInventorySummary, error) {
	var summary larkInventorySummary
	err := db.Table("products").
		Select(`COUNT(products.id) AS product_count,
			COALESCE(SUM(COALESCE(inventory_snapshots.quantity, 0)), 0) AS quantity,
			COALESCE(SUM(COALESCE(inventory_snapshots.quantity, 0) * products.default_purchase_cents), 0) AS value_cents,
			COALESCE(SUM(CASE WHEN products.low_stock_threshold > 0 AND COALESCE(inventory_snapshots.quantity, 0) <= products.low_stock_threshold THEN 1 ELSE 0 END), 0) AS low_stock_count,
			COALESCE(SUM(CASE WHEN COALESCE(inventory_snapshots.quantity, 0) = 0 THEN 1 ELSE 0 END), 0) AS zero_stock_count`).
		Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id").
		Where("products.archived_at IS NULL").
		Scan(&summary).Error
	return summary, err
}

func queryLarkMovements(db *gorm.DB, keyword string) ([]models.StockMovement, bool, error) {
	query := db.Model(&models.StockMovement{}).
		Select("stock_movements.*").
		Joins("JOIN products ON products.id = stock_movements.product_id").
		Preload("Product").
		Preload("Shop").
		Preload("Operator")
	if keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		query = query.Where("LOWER(products.name) LIKE ? OR LOWER(products.code) LIKE ?", like, like)
	}

	var movements []models.StockMovement
	err := query.
		Order("stock_movements.created_at DESC").
		Order("stock_movements.id DESC").
		Limit(larkResultLimit + 1).
		Find(&movements).Error
	if err != nil {
		return nil, false, err
	}
	more := len(movements) > larkResultLimit
	if more {
		movements = movements[:larkResultLimit]
	}
	return movements, more, nil
}

type larkTodayChanges struct {
	InboundCount       int64 `gorm:"column:inbound_count"`
	InboundQuantity    int64 `gorm:"column:inbound_quantity"`
	OutboundCount      int64 `gorm:"column:outbound_count"`
	OutboundQuantity   int64 `gorm:"column:outbound_quantity"`
	AdjustmentCount    int64 `gorm:"column:adjustment_count"`
	AdjustmentQuantity int64 `gorm:"column:adjustment_quantity"`
}

func queryLarkTodayChanges(db *gorm.DB, now time.Time) (larkTodayChanges, error) {
	localNow := now.In(larkLocation)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, larkLocation)
	end := start.AddDate(0, 0, 1)
	var changes larkTodayChanges
	err := db.Model(&models.StockMovement{}).
		Select(`COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0) AS inbound_count,
			COALESCE(SUM(CASE WHEN type = ? THEN quantity_delta ELSE 0 END), 0) AS inbound_quantity,
			COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0) AS outbound_count,
			COALESCE(SUM(CASE WHEN type = ? THEN -quantity_delta ELSE 0 END), 0) AS outbound_quantity,
			COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0) AS adjustment_count,
			COALESCE(SUM(CASE WHEN type = ? THEN quantity_delta ELSE 0 END), 0) AS adjustment_quantity`,
			models.MovementTypeInbound, models.MovementTypeInbound,
			models.MovementTypeSalesOutbound, models.MovementTypeSalesOutbound,
			models.MovementTypeAdjustment, models.MovementTypeAdjustment).
		Where("created_at >= ? AND created_at < ?", start.UTC(), end.UTC()).
		Scan(&changes).Error
	return changes, err
}

func queryLarkTodaySalesRanking(db *gorm.DB, now time.Time) ([]larkProductRow, bool, error) {
	localNow := now.In(larkLocation)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, larkLocation)
	end := start.AddDate(0, 0, 1)
	var rows []larkProductRow
	err := db.Table("products").
		Select(`products.*, COALESCE(inventory_snapshots.quantity, 0) AS quantity,
			COALESCE(SUM(-stock_movements.quantity_delta), 0) AS sales_quantity`).
		Joins(`JOIN stock_movements ON stock_movements.product_id = products.id
			AND stock_movements.type = ? AND stock_movements.created_at >= ? AND stock_movements.created_at < ?`,
			models.MovementTypeSalesOutbound, start.UTC(), end.UTC()).
		Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id").
		Where("products.archived_at IS NULL").
		Group("products.id, inventory_snapshots.quantity").
		Order("sales_quantity DESC").
		Order("products.name ASC").
		Order("products.code ASC").
		Order("products.id ASC").
		Limit(larkResultLimit + 1).
		Scan(&rows).Error
	if err != nil {
		return nil, false, err
	}
	more := len(rows) > larkResultLimit
	if more {
		rows = rows[:larkResultLimit]
	}
	return rows, more, nil
}

type larkMessenger struct {
	client    *lark.Client
	uploader  larkoutbound.Uploader
	uploadDir string
	chatID    string
	mu        sync.RWMutex
	imageKeys map[string]string
}

func newLarkMessenger(client *lark.Client, uploadDir string, chatID string) *larkMessenger {
	return &larkMessenger{
		client:    client,
		uploader:  larkoutbound.NewUploader(client),
		uploadDir: uploadDir,
		chatID:    chatID,
		imageKeys: make(map[string]string),
	}
}

func (m *larkMessenger) imageKey(ctx context.Context, imagePath string) string {
	if imagePath == "" || m.uploadDir == "" {
		return ""
	}
	m.mu.RLock()
	key := m.imageKeys[imagePath]
	m.mu.RUnlock()
	if key != "" {
		return key
	}

	localPath := filepath.Join(m.uploadDir, filepath.Base(imagePath))
	key, err := m.uploader.UploadImagePath(ctx, larkim.CreateImageImageTypeMessage, localPath)
	if err != nil {
		slog.Warn("upload product image to lark", slog.Any("err", err), slog.String("image_path", imagePath))
		return ""
	}
	m.mu.Lock()
	if len(m.imageKeys) >= 256 {
		clear(m.imageKeys)
	}
	m.imageKeys[imagePath] = key
	m.mu.Unlock()
	return key
}

func (m *larkMessenger) resolveImageKeys(ctx context.Context, imagePaths []string) []string {
	keys := make([]string, len(imagePaths))
	for index, imagePath := range imagePaths {
		keys[index] = m.imageKey(ctx, imagePath)
	}
	return keys
}

func (m *larkMessenger) sendCard(ctx context.Context, card string, replyMessageID string) error {
	requestUUID := uuid.NewString()
	_, err := larkoutbound.Retry(ctx, func(_ int) (interface{}, error) {
		if replyMessageID != "" {
			req := larkim.NewReplyMessageReqBuilder().
				MessageId(replyMessageID).
				Body(larkim.NewReplyMessageReqBodyBuilder().
					MsgType(larkim.MsgTypeInteractive).
					Content(card).
					Uuid(requestUUID).
					Build()).
				Build()
			resp, err := m.client.Im.V1.Message.Reply(ctx, req)
			if err != nil {
				return nil, err
			}
			if !resp.Success() {
				return nil, &larkcore.CodeError{Code: resp.Code, Msg: resp.Msg}
			}
			return resp, nil
		}

		req := larkim.NewCreateMessageReqBuilder().
			ReceiveIdType("chat_id").
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(m.chatID).
				MsgType(larkim.MsgTypeInteractive).
				Content(card).
				Uuid(requestUUID).
				Build()).
			Build()
		resp, err := m.client.Im.V1.Message.Create(ctx, req)
		if err != nil {
			return nil, err
		}
		if !resp.Success() {
			return nil, &larkcore.CodeError{Code: resp.Code, Msg: resp.Msg}
		}
		return resp, nil
	}, nil)
	return err
}

func larkQueryCard(command larkCommand, rows []larkProductRow, more bool, imageKeys []string) (string, error) {
	title := "库存查询"
	template := "blue"
	empty := "没有找到匹配的商品。"
	switch command.Action {
	case larkActionLowStock:
		title, template, empty = "低库存商品", "orange", "当前没有低库存商品。"
	case larkActionOutOfStock:
		title, template, empty = "缺货清单", "red", "当前没有缺货商品。"
	case larkActionValueRanking:
		title, template, empty = "库存金额排行", "blue", "当前没有可排行的库存商品。"
	case larkActionTodaySalesRanking:
		title, template, empty = "今日销售排行", "green", "今天还没有销售出库记录。"
	}
	if len(rows) == 0 {
		return larkSimpleCard(title, "grey", empty)
	}
	if len(rows) == 1 {
		if command.Action == larkActionInventory {
			title = "商品详情"
		}
		return larkProductCard(title, template, rows[0], larkImageKeyAt(imageKeys, 0))
	}

	elements := make([]larkcard.MessageCardElement, 0, len(rows)*2)
	for index, row := range rows {
		if index > 0 {
			elements = append(elements, larkcard.NewMessageCardHr().Build())
		}
		element, err := larkProductElement(row, larkImageKeyAt(imageKeys, index))
		if err != nil {
			return "", err
		}
		elements = append(elements, element)
	}
	if more {
		moreText := "结果超过 5 条，请缩小关键词。"
		if command.Action == larkActionValueRanking || command.Action == larkActionTodaySalesRanking {
			moreText = "仅展示前 5 条。"
		}
		elements = append(elements, larkcard.NewMessageCardMarkdown().Content(moreText).Build())
	}
	return larkCard(title, template, elements)
}

func larkProductCard(title string, template string, row larkProductRow, imageKey string) (string, error) {
	element, err := larkProductElement(row, imageKey)
	if err != nil {
		return "", err
	}
	if row.Quantity <= 0 {
		template = "red"
	} else if template == "blue" && larkLowStock(row.Product, row.Quantity) {
		template = "orange"
	}
	return larkCard(title, template, []larkcard.MessageCardElement{element})
}

func larkProductElement(row larkProductRow, imageKey string) (larkcard.MessageCardElement, error) {
	value, err := CurrentInventoryValue(models.InventorySnapshot{Product: row.Product, Quantity: row.Quantity})
	if err != nil {
		return nil, err
	}
	fields := make([]*larkcard.MessageCardField, 0, 4)
	if row.SalesQuantity > 0 {
		fields = append(fields, larkField(fmt.Sprintf("**今日售出**\n🔥 **%d 件**", row.SalesQuantity)))
	}
	fields = append(fields,
		larkField(fmt.Sprintf("**状态**\n**%s**", larkProductStatus(row))),
		larkField(fmt.Sprintf("**采购价**\n%s", larkMoney(row.DefaultPurchaseCents))),
		larkField(fmt.Sprintf("**库存金额**\n**%s**", larkMoney(value))),
	)
	quantityIcon := "📦"
	if row.Quantity <= 0 {
		quantityIcon = "⛔"
	} else if larkLowStock(row.Product, row.Quantity) {
		quantityIcon = "⚠️"
	}
	return larkDetailElement(
		fmt.Sprintf("**%s**\n编码：`%s`\n\n%s **库存数量：%d 件**", escapeLarkMarkdown(row.Name), escapeLarkMarkdown(row.Code), quantityIcon, row.Quantity),
		fields,
		imageKey,
		row.Name,
	), nil
}

func larkInventorySummaryCard(summary larkInventorySummary) (string, error) {
	template := "blue"
	if summary.LowStockCount > 0 {
		template = "orange"
	}
	element := larkDetailElement("**当前库存概览**", []*larkcard.MessageCardField{
		larkField(fmt.Sprintf("**商品数**\n**%d**", summary.ProductCount)),
		larkField(fmt.Sprintf("**库存总数量**\n**%d**", summary.Quantity)),
		larkField(fmt.Sprintf("**库存金额**\n**%s**", larkMoney(summary.ValueCents))),
		larkField(fmt.Sprintf("**低库存**\n⚠️ **%d**", summary.LowStockCount)),
		larkField(fmt.Sprintf("**无库存**\n**%d**", summary.ZeroStockCount)),
	}, "", "")
	return larkCard("库存概览", template, []larkcard.MessageCardElement{element})
}

func larkMovementsCard(movements []models.StockMovement, more bool, imageKeys []string) (string, error) {
	if len(movements) == 0 {
		return larkSimpleCard("最近流水", "grey", "没有找到匹配的流水记录。")
	}
	elements := make([]larkcard.MessageCardElement, 0, len(movements)*2)
	for index, movement := range movements {
		if index > 0 {
			elements = append(elements, larkcard.NewMessageCardHr().Build())
		}
		elements = append(elements, larkMovementElement(movement, larkImageKeyAt(imageKeys, index)))
	}
	if more {
		elements = append(elements, larkcard.NewMessageCardMarkdown().Content("结果超过 5 条，请缩小关键词。").Build())
	}
	return larkCard("最近流水", "blue", elements)
}

func larkMovementElement(movement models.StockMovement, imageKey string) larkcard.MessageCardElement {
	operation := "库存调整"
	symbol := "↕️"
	switch movement.Type {
	case models.MovementTypeInbound:
		operation = "入库"
		symbol = "⬆️"
	case models.MovementTypeSalesOutbound:
		operation = "销售出库"
		symbol = "⬇️"
	}
	shop := "-"
	if movement.Shop != nil {
		shop = escapeLarkMarkdown(movement.Shop.Name)
	}
	fields := []*larkcard.MessageCardField{
		larkField("**类型**\n" + operation),
		larkField(fmt.Sprintf("**数量变化**\n%s **%s**", symbol, larkSignedQuantity(movement.QuantityDelta))),
		larkField("**操作人**\n" + escapeLarkMarkdown(movement.Operator.Name)),
		larkField("**店铺**\n" + shop),
		larkField("**时间**\n" + movement.CreatedAt.In(larkLocation).Format("2006-01-02 15:04:05")),
	}
	if movement.Reason != "" {
		fields = append(fields, larkField("**原因**\n"+escapeLarkMarkdown(movement.Reason)))
	}
	return larkDetailElement(
		fmt.Sprintf("**%s**\n编码：`%s`", escapeLarkMarkdown(movement.Product.Name), escapeLarkMarkdown(movement.Product.Code)),
		fields,
		imageKey,
		movement.Product.Name,
	)
}

func larkTodayChangesCard(changes larkTodayChanges) (string, error) {
	element := larkDetailElement("**上海时间 · 今日库存变动**", []*larkcard.MessageCardField{
		larkField(fmt.Sprintf("**⬆️ 入库**\n%d 笔 / **%d 件**", changes.InboundCount, changes.InboundQuantity)),
		larkField(fmt.Sprintf("**⬇️ 销售出库**\n%d 笔 / **%d 件**", changes.OutboundCount, changes.OutboundQuantity)),
		larkField(fmt.Sprintf("**↕️ 库存调整**\n%d 笔 / **%s 件**", changes.AdjustmentCount, larkSignedQuantity(changes.AdjustmentQuantity))),
	}, "", "")
	return larkCard("今日变动", "blue", []larkcard.MessageCardElement{element})
}

func larkInventoryChangeCard(change InventoryChange, imageKey string) (string, error) {
	operation := "库存调整"
	template := "blue"
	symbol := "↕️"
	switch change.Movement.Type {
	case models.MovementTypeInbound:
		operation = "入库"
		template = "green"
		symbol = "⬆️"
	case models.MovementTypeSalesOutbound:
		operation = "销售出库"
		template = "red"
		symbol = "⬇️"
	}
	status := ""
	beforeLow := larkLowStock(change.Product, change.QuantityBefore)
	afterLow := larkLowStock(change.Product, change.QuantityAfter)
	if !beforeLow && afterLow {
		template = "orange"
		status = "⚠️ **已进入低库存**"
	} else if beforeLow && !afterLow {
		template = "green"
		status = "✅ **已恢复正常**"
	}

	fields := []*larkcard.MessageCardField{
		larkField(fmt.Sprintf("**数量变化**\n%s **%s**", symbol, larkSignedQuantity(change.Movement.QuantityDelta))),
		larkField(fmt.Sprintf("**库存结果**\n**%d → %d**", change.QuantityBefore, change.QuantityAfter)),
	}
	if status != "" {
		fields = append(fields, larkField("**库存状态**\n"+status))
	}
	fields = append(fields, larkField("**操作人**\n"+escapeLarkMarkdown(change.Movement.Operator.Name)))
	if change.Movement.Shop != nil {
		fields = append(fields, larkField("**店铺**\n"+escapeLarkMarkdown(change.Movement.Shop.Name)))
	}
	fields = append(fields, larkField("**时间**\n"+change.Movement.CreatedAt.In(larkLocation).Format("2006-01-02 15:04:05")))
	if change.Movement.Reason != "" {
		fields = append(fields, larkField("**原因**\n"+escapeLarkMarkdown(change.Movement.Reason)))
	}
	element := larkDetailElement(
		fmt.Sprintf("**%s**\n编码：`%s`", escapeLarkMarkdown(change.Product.Name), escapeLarkMarkdown(change.Product.Code)),
		fields,
		imageKey,
		change.Product.Name,
	)
	return larkCard("库存操作 · "+operation, template, []larkcard.MessageCardElement{element})
}

func larkHelpCard() (string, error) {
	return larkSimpleCard("库存机器人帮助", "blue",
		"直接说你想查什么，例如：\n"+
			"- `BR-1214G 还有多少？`\n"+
			"- `哪些商品快没了？` / `把缺货商品列出来`\n"+
			"- `库存总金额是多少？` / `库存金额最高的是哪些？`\n"+
			"- `绿茶最近谁操作过？`\n"+
			"- `今天出了多少货？` / `今天什么卖得最好？`")
}

func larkAIUnavailableCard() (string, error) {
	return larkSimpleCard("智能识别暂时不可用", "orange",
		"本次没有执行查询或库存操作，请稍后再试。")
}

func larkUnknownCard() (string, error) {
	return larkSimpleCard("只支持库存查询", "grey", "我可以查商品库存、低库存、缺货清单、库存概览、库存金额排行、最近流水、今日变动和今日销售排行。问“你会什么”可查看示例。")
}

func larkField(content string) *larkcard.MessageCardField {
	return larkcard.NewMessageCardField().
		IsShort(true).
		Text(larkcard.NewMessageCardLarkMd().Content(content).Build()).
		Build()
}

func larkDetailElement(content string, fields []*larkcard.MessageCardField, imageKey string, imageAlt string) larkcard.MessageCardElement {
	div := larkcard.NewMessageCardDiv().
		Text(larkcard.NewMessageCardLarkMd().Content(content).Build()).
		Fields(fields)
	if imageKey != "" {
		div.Extra(larkcard.NewMessageCardEmbedImage().
			ImgKey(imageKey).
			Alt(larkcard.NewMessageCardPlainText().Content(imageAlt).Build()).
			Preview(true).
			Build())
	}
	return div.Build()
}

func larkImageKeyAt(imageKeys []string, index int) string {
	if index < len(imageKeys) {
		return imageKeys[index]
	}
	return ""
}

func larkSimpleCard(title string, template string, content string) (string, error) {
	return larkCard(title, template, []larkcard.MessageCardElement{
		larkcard.NewMessageCardMarkdown().Content(content).Build(),
	})
}

func larkCard(title string, template string, elements []larkcard.MessageCardElement) (string, error) {
	return larkcard.NewMessageCard().
		Config(larkcard.NewMessageCardConfig().WideScreenMode(true).Build()).
		Header(larkcard.NewMessageCardHeader().
			Template(template).
			Title(larkcard.NewMessageCardPlainText().Content(title).Build()).
			Build()).
		Elements(elements).
		Build().
		JSON()
}

func larkProductStatus(row larkProductRow) string {
	status := make([]string, 0, 2)
	if !row.Enabled {
		status = append(status, "⏸️ 已禁用")
	}
	if row.Quantity <= 0 {
		status = append(status, "⛔ 缺货")
	} else if larkLowStock(row.Product, row.Quantity) {
		status = append(status, "⚠️ 低库存")
	}
	if len(status) == 0 {
		return "✅ 正常"
	}
	return strings.Join(status, " · ")
}

func larkLowStock(product models.Product, quantity int64) bool {
	return product.LowStockThreshold > 0 && quantity <= product.LowStockThreshold
}

func larkSignedQuantity(quantity int64) string {
	if quantity > 0 {
		return fmt.Sprintf("+%d", quantity)
	}
	return fmt.Sprintf("%d", quantity)
}

func larkMoney(cents int64) string {
	return fmt.Sprintf("¥%d.%02d", cents/100, cents%100)
}

func escapeLarkMarkdown(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	return strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"[", "\\[",
		"]", "\\]",
		"`", "\\`",
		"<", "&lt;",
		">", "&gt;",
	).Replace(value)
}
