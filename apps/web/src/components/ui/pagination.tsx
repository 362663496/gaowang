import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { PaginationMeta } from "@/features/types";
import { formatQuantity } from "@/lib/format";

export const initialPagination: PaginationMeta = { page: 1, page_size: 20, total: 0, total_pages: 0 };

export function Pagination({ meta, onPageChange }: { meta: PaginationMeta; onPageChange: (page: number) => void }) {
  if (meta.total === 0) return null;
  const totalPages = Math.max(meta.total_pages, 1);
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-[var(--text-secondary)]">
      <span>共 {formatQuantity(meta.total)} 条，第 {meta.page} / {totalPages} 页</span>
      <div className="flex gap-2">
        <Button disabled={meta.page <= 1} size="sm" type="button" variant="secondary" onClick={() => onPageChange(meta.page - 1)}>
          <ChevronLeft className="h-4 w-4" />上一页
        </Button>
        <Button disabled={meta.page >= totalPages} size="sm" type="button" variant="secondary" onClick={() => onPageChange(meta.page + 1)}>
          下一页<ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
