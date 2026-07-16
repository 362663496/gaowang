import type { Role } from "@/lib/api";

export type PaginationMeta = {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
};

export type Paginated<T> = {
  items: T[];
  pagination: PaginationMeta;
};

export type Product = {
  ID: string;
  Name: string;
  Code: string;
  ImagePath: string;
  DefaultPurchaseCents: number;
  DefaultSaleCents: number;
  LowStockThreshold: number;
  Note: string;
  Enabled: boolean;
  ArchivedAt: string | null;
  CreatedAt: string;
  UpdatedAt: string;
};

export type Shop = {
  ID: string;
  Name: string;
  Note: string;
  Enabled: boolean;
  CreatedAt: string;
  UpdatedAt: string;
};

export type InventorySnapshot = {
  ProductID: string;
  Product: Product;
  Quantity: number;
  MovingAverageCostCents: number;
  InventoryValueCents: number;
  UpdatedAt: string;
};

export type MovementType = "inbound" | "sales_outbound" | "adjustment";

export type StockMovement = {
  ID: string;
  Type: MovementType;
  ProductID: string;
  Product: Product;
  ShopID: string | null;
  Shop: Shop | null;
  QuantityDelta: number;
  PurchaseUnitCents: number | null;
  SaleUnitCents: number | null;
  CostUnitCents: number;
  PurchaseAmountCents: number;
  RevenueCents: number;
  CostAmountCents: number;
  GrossProfitCents: number;
  Reason: string;
  CreatedAt: string;
};

export type SalesSummary = {
  revenue_cents: number;
  cost_cents: number;
  gross_profit_cents: number;
};

export type SalesTrendRow = {
  day: string;
  revenue_cents: number;
  cost_cents: number;
  gross_profit_cents: number;
  quantity_sold: number;
};

export type ProductRankingRow = {
  product_id: string;
  product_name: string;
  product_code: string;
  archived: boolean;
  revenue_cents: number;
  cost_cents: number;
  gross_profit_cents: number;
  quantity_sold: number;
  movement_count: number;
};

export type ShopRankingRow = {
  shop_id: string;
  shop_name: string;
  revenue_cents: number;
  cost_cents: number;
  gross_profit_cents: number;
  quantity_sold: number;
  movement_count: number;
};

export type AuditLog = {
  id: string;
  actor_id: string | null;
  actor: User | null;
  action: string;
  resource_type: string;
  resource_id: string;
  metadata: Record<string, string>;
  ip_address: string;
  created_at: string;
};

export type BackupJob = {
  ID: string;
  StartedAt: string;
  FinishedAt: string | null;
  Status: "running" | "success" | "failed";
  FilePath: string;
  FileSize: number;
  EmailStatus: string;
  Recipient: string;
  ErrorMessage: string;
  CreatedAt: string;
  UpdatedAt: string;
};

export type AppSettings = {
  backup_email_recipient: string;
};

export type User = {
  id: string;
  name: string;
  email: string;
  role: Role;
};
