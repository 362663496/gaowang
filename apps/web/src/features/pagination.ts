import type { PaginationMeta } from "@/features/types";
import { formatQuantity } from "@/lib/format";

export const initialPagination: PaginationMeta = { page: 1, page_size: 20, total: 0, total_pages: 0 };

export function tablePagination(meta: PaginationMeta, onChange: (page: number) => void) {
  return {
    current: meta.page,
    pageSize: meta.page_size,
    total: meta.total,
    showSizeChanger: false,
    showTotal: (total: number, range: [number, number]) =>
      `${formatQuantity(range[0])}-${formatQuantity(range[1])}，共 ${formatQuantity(total)} 条`,
    onChange,
  };
}
