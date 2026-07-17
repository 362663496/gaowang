# Implementation Plan: 库存页面导出含图片

1. Add Excel dependencies to `apps/api`:
   - `github.com/xuri/excelize/v2@v2.11.0`
   - direct `golang.org/x/image` imports for WebP decoding and thumbnail scaling.
2. Refactor the existing inventory base query into one shared helper used by list and export.
3. Add the protected `/inventory/export` route and workbook builder:
   - unpaginated current-filter query;
   - confirmed columns and numeric formats;
   - bounded PNG thumbnails for JPG/PNG/WebP;
   - per-image graceful fallback.
4. Add one backend regression covering filter scope, workbook values, embedded image bytes, and missing-image tolerance.
5. Add `apiDownload` using the existing API authentication/error path and cover it in `api.test.ts`.
6. Add the inventory-page export button, loading state, low-stock query propagation, browser download, and non-destructive error feedback.
7. Run formatting and focused tests, then full checks:
   - `cd apps/api && gofmt -w <changed-go-files> && go test ./... && go vet ./...`
   - `cd apps/web && npm run lint && npx tsc --noEmit --incremental false && npm test && npm run build`
8. Inspect the generated workbook in the regression test by reopening it with Excelize and asserting the expected image is embedded in its row.

## Risk and Rollback Points

- If thumbnail decoding fails for one product, keep the row and mark the image cell `无图片`; do not weaken whole-export errors.
- If the shared query changes list behavior, revert the extraction and keep the export query equivalent while preserving test coverage.
- No migration or persistent data change is involved.

