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

	larkActionInventory = "lark.query_inventory"
	larkActionProduct   = "lark.query_product"
	larkActionLowStock  = "lark.query_low_stock"
	larkActionHelp      = "lark.help"
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
	db        *gorm.DB
	jobs      chan larktypes.NormalizedMessage
	messenger *larkMessenger
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
		db:        db,
		jobs:      make(chan larktypes.NormalizedMessage, larkQueueSize),
		messenger: newLarkMessenger(client, cfg.UploadDir, cfg.LarkChatID),
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

func parseLarkCommand(message larktypes.NormalizedMessage) larkCommand {
	if message.RawContentType != "text" {
		return larkCommand{Action: larkActionHelp, Name: "帮助"}
	}
	content := message.Content
	for _, mention := range message.Mentions {
		if mention.IsBot && mention.Key != "" {
			content = strings.ReplaceAll(content, mention.Key, "")
		}
	}
	parts := strings.Fields(strings.TrimSpace(content))
	if len(parts) == 0 {
		return larkCommand{Action: larkActionHelp, Name: "帮助"}
	}

	name := parts[0]
	keyword := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), name))
	runes := []rune(keyword)
	if len(runes) > larkKeywordMaxRunes {
		keyword = string(runes[:larkKeywordMaxRunes])
	}
	switch name {
	case "查库存":
		if keyword != "" {
			return larkCommand{Action: larkActionInventory, Name: name, Keyword: keyword}
		}
	case "查商品":
		if keyword != "" {
			return larkCommand{Action: larkActionProduct, Name: name, Keyword: keyword}
		}
	case "低库存":
		if keyword == "" {
			return larkCommand{Action: larkActionLowStock, Name: name}
		}
	case "帮助":
		if keyword == "" {
			return larkCommand{Action: larkActionHelp, Name: name}
		}
	}
	return larkCommand{Action: larkActionHelp, Name: name, Keyword: keyword}
}

func (b *larkBot) handle(ctx context.Context, message larktypes.NormalizedMessage) {
	command := parseLarkCommand(message)
	b.recordAudit(message, command)

	var (
		card string
		err  error
	)
	if command.Action == larkActionHelp {
		card, err = larkHelpCard()
	} else {
		rows, more, queryErr := queryLarkProducts(b.db, command)
		if queryErr != nil {
			slog.Warn("query inventory from lark", slog.Any("err", queryErr), slog.String("message_id", message.MessageID))
			card, err = larkSimpleCard("查询失败", "red", "库存查询暂时失败，请稍后重试。")
		} else {
			imageKey := ""
			if len(rows) == 1 {
				imageKey = b.messenger.imageKey(ctx, rows[0].ImagePath)
			}
			card, err = larkQueryCard(command, rows, more, imageKey)
		}
	}
	if err != nil {
		slog.Warn("build lark query reply", slog.Any("err", err), slog.String("message_id", message.MessageID))
		return
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
}

func queryLarkProducts(db *gorm.DB, command larkCommand) ([]larkProductRow, bool, error) {
	query := db.Table("products").
		Select("products.*, COALESCE(inventory_snapshots.quantity, 0) AS quantity").
		Where("products.archived_at IS NULL")
	if command.Action == larkActionProduct {
		query = query.Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id")
	} else {
		query = query.Joins("JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id")
	}
	if command.Action == larkActionLowStock {
		query = query.Where("products.low_stock_threshold > 0 AND inventory_snapshots.quantity <= products.low_stock_threshold")
	}
	if command.Keyword != "" {
		like := "%" + strings.ToLower(command.Keyword) + "%"
		query = query.Where("LOWER(products.name) LIKE ? OR LOWER(products.code) LIKE ?", like, like)
	}

	var rows []larkProductRow
	err := query.
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

func larkQueryCard(command larkCommand, rows []larkProductRow, more bool, imageKey string) (string, error) {
	if len(rows) == 0 {
		return larkSimpleCard("查询结果", "grey", "没有找到匹配的商品。")
	}
	if len(rows) == 1 {
		return larkProductCard(rows[0], imageKey)
	}

	var content strings.Builder
	for index, row := range rows {
		if index > 0 {
			content.WriteString("\n---\n")
		}
		fmt.Fprintf(&content, "**%s**  `%s`\n库存：%d　状态：%s",
			escapeLarkMarkdown(row.Name), escapeLarkMarkdown(row.Code), row.Quantity, larkProductStatus(row))
	}
	if more {
		content.WriteString("\n\n结果超过 5 条，请缩小关键词。")
	}
	title := "库存查询"
	if command.Action == larkActionProduct {
		title = "商品查询"
	} else if command.Action == larkActionLowStock {
		title = "低库存商品"
	}
	return larkSimpleCard(title, "blue", content.String())
}

func larkProductCard(row larkProductRow, imageKey string) (string, error) {
	value, err := CurrentInventoryValue(models.InventorySnapshot{Product: row.Product, Quantity: row.Quantity})
	if err != nil {
		return "", err
	}
	content := fmt.Sprintf(
		"**商品名称：** %s\n**编码：** `%s`\n**当前数量：** %d\n**当前采购价：** %s\n**商品售价：** %s\n**库存金额：** %s\n**状态：** %s",
		escapeLarkMarkdown(row.Name),
		escapeLarkMarkdown(row.Code),
		row.Quantity,
		larkMoney(row.DefaultPurchaseCents),
		larkMoney(row.DefaultSaleCents),
		larkMoney(value),
		larkProductStatus(row),
	)
	return larkCard("商品详情", "blue", content, imageKey)
}

func larkInventoryChangeCard(change InventoryChange, imageKey string) (string, error) {
	operation := "库存调整"
	switch change.Movement.Type {
	case models.MovementTypeInbound:
		operation = "入库"
	case models.MovementTypeSalesOutbound:
		operation = "销售出库"
	}
	template := "blue"
	statusLine := ""
	beforeLow := larkLowStock(change.Product, change.QuantityBefore)
	afterLow := larkLowStock(change.Product, change.QuantityAfter)
	if !beforeLow && afterLow {
		template = "orange"
		statusLine = "\n**库存状态：** 已进入低库存"
	} else if beforeLow && !afterLow {
		template = "green"
		statusLine = "\n**库存状态：** 已恢复正常"
	}

	content := fmt.Sprintf(
		"**操作：** %s\n**商品：** %s\n**编码：** `%s`\n**数量变化：** %s\n**库存结果：** %d → %d\n**操作人：** %s\n**时间：** %s%s",
		operation,
		escapeLarkMarkdown(change.Product.Name),
		escapeLarkMarkdown(change.Product.Code),
		larkSignedQuantity(change.Movement.QuantityDelta),
		change.QuantityBefore,
		change.QuantityAfter,
		escapeLarkMarkdown(change.Movement.Operator.Name),
		change.Movement.CreatedAt.In(larkLocation).Format("2006-01-02 15:04:05"),
		statusLine,
	)
	if change.Movement.Shop != nil {
		content += "\n**店铺：** " + escapeLarkMarkdown(change.Movement.Shop.Name)
	}
	if change.Movement.Reason != "" {
		content += "\n**原因：** " + escapeLarkMarkdown(change.Movement.Reason)
	}
	return larkCard("库存操作 · "+operation, template, content, imageKey)
}

func larkHelpCard() (string, error) {
	return larkSimpleCard("库存机器人帮助", "blue",
		"请在固定库存群内 @机器人 后发送：\n"+
			"- `查库存 <名称或编码>`\n"+
			"- `查商品 <名称或编码>`\n"+
			"- `低库存`\n"+
			"- `帮助`")
}

func larkSimpleCard(title string, template string, content string) (string, error) {
	return larkCard(title, template, content, "")
}

func larkCard(title string, template string, content string, imageKey string) (string, error) {
	elements := make([]larkcard.MessageCardElement, 0, 2)
	if imageKey != "" {
		elements = append(elements, larkcard.NewMessageCardImage().
			ImgKey(imageKey).
			Alt(larkcard.NewMessageCardPlainText().Content("商品图片").Build()).
			Mode(larkcard.MessageCardImageModelFitHorizontal).
			Build())
	}
	elements = append(elements, larkcard.NewMessageCardMarkdown().Content(content).Build())
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
		status = append(status, "已禁用")
	}
	if larkLowStock(row.Product, row.Quantity) {
		status = append(status, "低库存")
	}
	if len(status) == 0 {
		return "正常"
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
