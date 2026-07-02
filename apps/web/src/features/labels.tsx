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
