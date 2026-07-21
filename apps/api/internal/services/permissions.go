package services

import (
	"fmt"
	"sort"

	"gaowang/apps/api/internal/models"
	"gorm.io/gorm"
)

// Permission keys — keep stable; they are stored in staff_permissions and exposed to the web UI.
const (
	PermProductRead            = "product.read"
	PermProductCreate          = "product.create"
	PermProductUpdate          = "product.update"
	PermProductToggle          = "product.toggle"
	PermProductDelete          = "product.delete"
	PermShopRead               = "shop.read"
	PermShopCreate             = "shop.create"
	PermInventoryRead          = "inventory.read"
	PermInventoryInbound       = "inventory.inbound"
	PermInventorySalesOutbound = "inventory.sales_outbound"
	PermInventoryAdjust        = "inventory.adjust"
	PermMovementRead           = "movement.read"
	PermMovementUpdate         = "movement.update"
	PermReportSalesSummary     = "report.sales_summary"
	PermReportSalesTrend       = "report.sales_trend"
	PermReportProductRanking   = "report.product_ranking"
	PermReportShopRanking      = "report.shop_ranking"
	PermAuditRead              = "audit.read"
	PermBackupRead             = "backup.read"
	PermBackupRun              = "backup.run"
	PermSettingRead            = "setting.read"
	PermSettingUpdate          = "setting.update"
	PermUserRead               = "user.read"
	PermUserCreate             = "user.create"
	PermPermissionRead         = "permission.read"
	PermPermissionUpdate       = "permission.update"
)

type PermissionDef struct {
	Key             string   `json:"key"`
	Module          string   `json:"module"`
	ModuleLabel     string   `json:"module_label"`
	ActionLabel     string   `json:"action_label"`
	StaffAssignable bool     `json:"staff_assignable"`
	Requires        []string `json:"requires"`
}

var permissionCatalog = []PermissionDef{
	{Key: PermProductRead, Module: "product", ModuleLabel: "商品", ActionLabel: "查看", StaffAssignable: true},
	{Key: PermProductCreate, Module: "product", ModuleLabel: "商品", ActionLabel: "新增", StaffAssignable: true, Requires: []string{PermProductRead}},
	{Key: PermProductUpdate, Module: "product", ModuleLabel: "商品", ActionLabel: "编辑", StaffAssignable: true, Requires: []string{PermProductRead}},
	{Key: PermProductToggle, Module: "product", ModuleLabel: "商品", ActionLabel: "启停", StaffAssignable: true, Requires: []string{PermProductRead}},
	{Key: PermProductDelete, Module: "product", ModuleLabel: "商品", ActionLabel: "删除", StaffAssignable: true, Requires: []string{PermProductRead}},
	{Key: PermShopRead, Module: "shop", ModuleLabel: "店铺", ActionLabel: "查看", StaffAssignable: true},
	{Key: PermShopCreate, Module: "shop", ModuleLabel: "店铺", ActionLabel: "新增", StaffAssignable: true, Requires: []string{PermShopRead}},
	{Key: PermInventoryRead, Module: "inventory", ModuleLabel: "库存", ActionLabel: "查看", StaffAssignable: true, Requires: []string{PermProductRead}},
	{Key: PermInventoryInbound, Module: "inventory", ModuleLabel: "库存", ActionLabel: "入库", StaffAssignable: true, Requires: []string{PermInventoryRead, PermShopRead}},
	{Key: PermInventorySalesOutbound, Module: "inventory", ModuleLabel: "库存", ActionLabel: "销售出库", StaffAssignable: true, Requires: []string{PermInventoryRead, PermShopRead}},
	{Key: PermInventoryAdjust, Module: "inventory", ModuleLabel: "库存", ActionLabel: "调整", StaffAssignable: true, Requires: []string{PermInventoryRead}},
	{Key: PermMovementRead, Module: "movement", ModuleLabel: "流水", ActionLabel: "查看", StaffAssignable: true, Requires: []string{PermProductRead, PermShopRead}},
	{Key: PermMovementUpdate, Module: "movement", ModuleLabel: "流水", ActionLabel: "编辑", StaffAssignable: true, Requires: []string{PermMovementRead}},
	{Key: PermReportSalesSummary, Module: "report", ModuleLabel: "报表", ActionLabel: "销售汇总", StaffAssignable: true},
	{Key: PermReportSalesTrend, Module: "report", ModuleLabel: "报表", ActionLabel: "销售趋势", StaffAssignable: true},
	{Key: PermReportProductRanking, Module: "report", ModuleLabel: "报表", ActionLabel: "商品排行", StaffAssignable: true},
	{Key: PermReportShopRanking, Module: "report", ModuleLabel: "报表", ActionLabel: "店铺排行", StaffAssignable: true},
	{Key: PermAuditRead, Module: "audit", ModuleLabel: "审计", ActionLabel: "查看", StaffAssignable: true},
	{Key: PermBackupRead, Module: "backup", ModuleLabel: "备份", ActionLabel: "查看", StaffAssignable: true},
	{Key: PermBackupRun, Module: "backup", ModuleLabel: "备份", ActionLabel: "执行", StaffAssignable: true, Requires: []string{PermBackupRead}},
	{Key: PermSettingRead, Module: "setting", ModuleLabel: "设置", ActionLabel: "查看", StaffAssignable: true},
	{Key: PermSettingUpdate, Module: "setting", ModuleLabel: "设置", ActionLabel: "修改", StaffAssignable: true, Requires: []string{PermSettingRead}},
	{Key: PermUserRead, Module: "user", ModuleLabel: "用户", ActionLabel: "查看", StaffAssignable: false},
	{Key: PermUserCreate, Module: "user", ModuleLabel: "用户", ActionLabel: "新建", StaffAssignable: false, Requires: []string{PermUserRead}},
	{Key: PermPermissionRead, Module: "permission", ModuleLabel: "权限", ActionLabel: "查看", StaffAssignable: false},
	{Key: PermPermissionUpdate, Module: "permission", ModuleLabel: "权限", ActionLabel: "修改", StaffAssignable: false, Requires: []string{PermPermissionRead}},
}

var permissionByKey map[string]PermissionDef

func init() {
	permissionByKey = make(map[string]PermissionDef, len(permissionCatalog))
	for i := range permissionCatalog {
		// Always use a non-nil empty slice so JSON encodes "requires":[] not null.
		permissionCatalog[i].Requires = copyRequires(permissionCatalog[i].Requires)
		permissionByKey[permissionCatalog[i].Key] = permissionCatalog[i]
	}
}

func PermissionCatalog() []PermissionDef {
	out := make([]PermissionDef, len(permissionCatalog))
	for i, def := range permissionCatalog {
		out[i] = def
		out[i].Requires = copyRequires(def.Requires)
	}
	return out
}

func copyRequires(requires []string) []string {
	// Prefer empty slice over nil so clients never see requires: null.
	out := make([]string, 0, len(requires))
	out = append(out, requires...)
	return out
}

func AllPermissionKeys() []string {
	keys := make([]string, 0, len(permissionCatalog))
	for _, def := range permissionCatalog {
		keys = append(keys, def.Key)
	}
	return keys
}

func ExpandPermissionClosure(keys []string) ([]string, error) {
	selected := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		def, ok := permissionByKey[key]
		if !ok {
			return nil, fmt.Errorf("unknown permission: %s", key)
		}
		if !def.StaffAssignable {
			return nil, fmt.Errorf("permission is admin-only: %s", key)
		}
		if err := collectClosure(key, selected); err != nil {
			return nil, err
		}
	}
	return sortedKeys(selected), nil
}

func collectClosure(key string, selected map[string]struct{}) error {
	if _, ok := selected[key]; ok {
		return nil
	}
	def, ok := permissionByKey[key]
	if !ok {
		return fmt.Errorf("unknown permission: %s", key)
	}
	selected[key] = struct{}{}
	for _, req := range def.Requires {
		if err := collectClosure(req, selected); err != nil {
			return err
		}
	}
	return nil
}

// EffectivePermissions returns the permission set for an authenticated user.
// Admin always gets the full catalog; staff gets only known assignable grants.
func EffectivePermissions(db *gorm.DB, user models.User) ([]string, error) {
	if user.Role == models.RoleAdmin {
		return AllPermissionKeys(), nil
	}
	var rows []models.StaffPermission
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load staff permissions: %w", err)
	}
	selected := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		def, ok := permissionByKey[row.Permission]
		if !ok || !def.StaffAssignable {
			continue
		}
		selected[row.Permission] = struct{}{}
	}
	return sortedKeys(selected), nil
}

func PermissionSet(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}

func HasPermission(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// ReplaceStaffPermissions atomically replaces all staff grants inside the provided transaction.
func ReplaceStaffPermissions(tx *gorm.DB, permissions []string) (before []string, after []string, err error) {
	var existing []models.StaffPermission
	if err := tx.Find(&existing).Error; err != nil {
		return nil, nil, fmt.Errorf("load existing staff permissions: %w", err)
	}
	beforeSet := make(map[string]struct{}, len(existing))
	for _, row := range existing {
		def, ok := permissionByKey[row.Permission]
		if !ok || !def.StaffAssignable {
			continue
		}
		beforeSet[row.Permission] = struct{}{}
	}
	before = sortedKeys(beforeSet)

	after, err = ExpandPermissionClosure(permissions)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Where("1 = 1").Delete(&models.StaffPermission{}).Error; err != nil {
		return nil, nil, fmt.Errorf("clear staff permissions: %w", err)
	}
	for _, key := range after {
		if err := tx.Create(&models.StaffPermission{Permission: key}).Error; err != nil {
			return nil, nil, fmt.Errorf("insert staff permission %s: %w", key, err)
		}
	}
	return before, after, nil
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
