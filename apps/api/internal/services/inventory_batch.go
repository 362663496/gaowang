package services

import (
	"errors"
	"fmt"
	"sort"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errOutboundBatchDryRun = errors.New("outbound batch dry run")

type OutboundBatchItem struct {
	ProductID   uuid.UUID
	ProductCode string
	Quantity    int64
}

type OutboundBatchInput struct {
	ShopID     uuid.UUID
	Note       string
	OperatorID uuid.UUID
	Items      []OutboundBatchItem
}

type OutboundBatchLine struct {
	ProductCode     string
	Product         models.Product
	Quantity        int64
	QuantityAfter   int64
	CostAmountCents int64
	Change          InventoryChange
}

type OutboundBatchResult struct {
	Lines          []OutboundBatchLine
	TotalQuantity  int64
	TotalCostCents int64
}

type OutboundBatchItemError struct {
	ProductID   string
	ProductCode string
	Message     string
	Available   *int64
	Requested   int64
	Candidates  []CatalogRef
}

type OutboundBatchRejectedError struct {
	Items []OutboundBatchItemError
}

func (e *OutboundBatchRejectedError) Error() string {
	return "outbound batch rejected"
}

// CreateSalesOutboundBatch validates every item, then writes every sales movement in one transaction.
// dryRun uses the same locks and checks, then rolls the transaction back.
func (s InventoryService) CreateSalesOutboundBatch(input OutboundBatchInput, dryRun bool) (OutboundBatchResult, error) {
	var result OutboundBatchResult
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		applied, rejected, err := applyOutboundBatch(tx, input)
		if err != nil {
			return err
		}
		if rejected != nil {
			return rejected
		}
		result = applied
		if dryRun {
			return errOutboundBatchDryRun
		}
		return nil
	})
	if errors.Is(err, errOutboundBatchDryRun) {
		return result, nil
	}
	return result, err
}

func applyOutboundBatch(tx *gorm.DB, input OutboundBatchInput) (OutboundBatchResult, *OutboundBatchRejectedError, error) {
	note, err := normalizeOptionalNote(input.Note)
	if err != nil {
		return OutboundBatchResult{}, nil, err
	}
	if len(input.Items) == 0 {
		return OutboundBatchResult{}, nil, fmt.Errorf("items are required")
	}
	if input.ShopID == uuid.Nil {
		return OutboundBatchResult{}, nil, fmt.Errorf("shop is required")
	}

	itemErrors := batchItemErrors(input.Items)
	if len(itemErrors) > 0 {
		return OutboundBatchResult{}, &OutboundBatchRejectedError{Items: itemErrors}, nil
	}

	ids := make([]uuid.UUID, len(input.Items))
	for i, item := range input.Items {
		ids[i] = item.ProductID
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })

	products := make(map[uuid.UUID]models.Product, len(ids))
	snapshots := make(map[uuid.UUID]models.InventorySnapshot, len(ids))
	lockErrors := make(map[uuid.UUID]OutboundBatchItemError, len(ids))
	for _, id := range ids {
		product, err := lockActiveProduct(tx, id)
		if errors.Is(err, ErrProductArchived) || errors.Is(err, gorm.ErrRecordNotFound) {
			message := "商品不存在"
			if errors.Is(err, ErrProductArchived) {
				message = "商品已归档"
			}
			lockErrors[id] = OutboundBatchItemError{ProductID: id.String(), Message: message}
			continue
		}
		if err != nil {
			return OutboundBatchResult{}, nil, fmt.Errorf("lock product: %w", err)
		}
		snapshot, err := lockSnapshot(tx, id)
		if err != nil {
			return OutboundBatchResult{}, nil, fmt.Errorf("lock inventory snapshot: %w", err)
		}
		products[id] = product
		snapshots[id] = snapshot
	}

	stockErrors := make(map[uuid.UUID]OutboundBatchItemError)
	for _, item := range input.Items {
		if _, ok := lockErrors[item.ProductID]; ok {
			continue
		}
		available := snapshots[item.ProductID].Quantity
		if available < item.Quantity {
			availableCopy := available
			stockErrors[item.ProductID] = OutboundBatchItemError{
				ProductID:   item.ProductID.String(),
				ProductCode: item.ProductCode,
				Message:     "库存不足",
				Available:   &availableCopy,
				Requested:   item.Quantity,
			}
		}
	}
	if len(lockErrors) > 0 || len(stockErrors) > 0 {
		rejected := make([]OutboundBatchItemError, 0, len(input.Items))
		for _, item := range input.Items {
			if locked, ok := lockErrors[item.ProductID]; ok {
				locked.ProductCode = item.ProductCode
				locked.Requested = item.Quantity
				rejected = append(rejected, locked)
				continue
			}
			if stock, ok := stockErrors[item.ProductID]; ok {
				rejected = append(rejected, stock)
			}
		}
		return OutboundBatchResult{}, &OutboundBatchRejectedError{Items: rejected}, nil
	}

	lines := make([]OutboundBatchLine, 0, len(input.Items))
	var totalQuantity int64
	var totalCost int64
	for _, item := range input.Items {
		snapshot := snapshots[item.ProductID]
		product := products[item.ProductID]
		quantityBefore := snapshot.Quantity
		movement, err := applySalesOutbound(&snapshot, item.Quantity, product.DefaultPurchaseCents)
		if err != nil {
			return OutboundBatchResult{}, nil, err
		}
		snapshots[item.ProductID] = snapshot
		movement.ProductID = item.ProductID
		movement.ShopID = &input.ShopID
		movement.Reason = note
		movement.OperatorID = input.OperatorID
		if err := tx.Create(&movement).Error; err != nil {
			return OutboundBatchResult{}, nil, fmt.Errorf("create sales movement: %w", err)
		}
		totalQuantity, err = checkedAdd(totalQuantity, item.Quantity)
		if err != nil {
			return OutboundBatchResult{}, nil, err
		}
		totalCost, err = checkedAdd(totalCost, movement.CostAmountCents)
		if err != nil {
			return OutboundBatchResult{}, nil, err
		}
		lines = append(lines, OutboundBatchLine{
			ProductCode:     product.Code,
			Product:         product,
			Quantity:        item.Quantity,
			QuantityAfter:   snapshot.Quantity,
			CostAmountCents: movement.CostAmountCents,
			Change: InventoryChange{
				Product:        product,
				Movement:       movement,
				QuantityBefore: quantityBefore,
				QuantityAfter:  snapshot.Quantity,
			},
		})
	}
	for _, id := range ids {
		snapshot := snapshots[id]
		if err := tx.Save(&snapshot).Error; err != nil {
			return OutboundBatchResult{}, nil, fmt.Errorf("save inventory snapshot: %w", err)
		}
	}
	return OutboundBatchResult{Lines: lines, TotalQuantity: totalQuantity, TotalCostCents: totalCost}, nil, nil
}

func batchItemErrors(items []OutboundBatchItem) []OutboundBatchItemError {
	var failures []OutboundBatchItemError
	seen := make(map[uuid.UUID]struct{}, len(items))
	for _, item := range items {
		if item.Quantity <= 0 {
			failures = append(failures, OutboundBatchItemError{
				ProductID:   item.ProductID.String(),
				ProductCode: item.ProductCode,
				Message:     "数量必须大于 0",
				Requested:   item.Quantity,
			})
			continue
		}
		if item.ProductID == uuid.Nil {
			failures = append(failures, OutboundBatchItemError{
				ProductCode: item.ProductCode,
				Message:     "商品不存在",
				Requested:   item.Quantity,
			})
			continue
		}
		if _, ok := seen[item.ProductID]; ok {
			failures = append(failures, OutboundBatchItemError{
				ProductID:   item.ProductID.String(),
				ProductCode: item.ProductCode,
				Message:     "商品重复",
				Requested:   item.Quantity,
			})
			continue
		}
		seen[item.ProductID] = struct{}{}
	}
	return failures
}
