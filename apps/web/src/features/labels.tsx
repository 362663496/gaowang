import { Badge } from "@/components/ui/badge";
import type { BackupJob, MovementType } from "@/features/types";

export function MovementBadge({ type }: { type: MovementType }) {
  const map = {
    inbound: { label: "入库", tone: "success" },
    sales_outbound: { label: "销售出库", tone: "accent" },
    adjustment: { label: "调整", tone: "warning" },
  } as const;
  return <Badge tone={map[type].tone}>{map[type].label}</Badge>;
}

export function StockBadge({ quantity, threshold }: { quantity: number; threshold: number }) {
  if (quantity <= 0) {
    return <Badge tone="error">无库存</Badge>;
  }
  if (threshold > 0 && quantity <= threshold) {
    return <Badge tone="warning">低库存</Badge>;
  }
  return <Badge tone="success">正常</Badge>;
}

export function BackupStatusBadge({ status }: { status: BackupJob["Status"] }) {
  const map = {
    running: { label: "运行中", tone: "warning" },
    success: { label: "成功", tone: "success" },
    failed: { label: "失败", tone: "error" },
  } as const;
  return <Badge tone={map[status].tone}>{map[status].label}</Badge>;
}

const auditActions: Record<string, string> = {
  "auth.login_succeeded": "登录成功",
  "auth.login_failed": "登录失败",
  "auth.password_changed": "修改密码",
  "product.create": "新增商品",
  "product.update": "修改商品",
  "product.enable": "启用商品",
  "product.disable": "禁用商品",
  "product.delete": "删除商品",
  "product.archive": "归档商品",
  "shop.create": "新增店铺",
  "inventory.inbound": "入库",
  "inventory.sales_outbound": "销售出库",
  "inventory.adjustment": "库存调整",
  "user.create": "新增用户",
  "backup.run_succeeded": "备份成功",
  "backup.run_failed": "备份失败",
  "settings.update": "设置修改",
};

const auditResources: Record<string, string> = {
  auth: "认证",
  backup: "备份",
  product: "商品",
  shop: "店铺",
  setting: "设置",
  user: "用户",
};

export function auditActionLabel(action: string): string {
  return auditActions[action] ?? action;
}

export function auditResourceLabel(resource: string): string {
  return auditResources[resource] ?? resource;
}

export function AuditActionBadge({ action }: { action: string }) {
  const tone = action.endsWith("failed") ? "error" : action.startsWith("inventory.") ? "accent" : "neutral";
  return <Badge tone={tone}>{auditActionLabel(action)}</Badge>;
}
