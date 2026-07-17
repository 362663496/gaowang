# Excel Export Research

## Repository Evidence

- `apps/web/src/app/(app)/inventory/page.tsx` already loads all active inventory through `/inventory?all=true` and exposes a low-stock filter.
- `apps/api/internal/http/handlers/inventory.go` owns the authoritative inventory query and preloads `Product`, including `ImagePath`.
- Uploaded product images live under `config.Config.UploadDir`; `/uploads/*` is only the HTTP presentation path.
- Upload validation accepts JPG, PNG, and WebP up to 5 MB.
- No spreadsheet export dependency or existing binary-download helper is present.

## Library Choice

Use `github.com/xuri/excelize/v2` v2.11.0 in the Go API.

- It writes standard `.xlsx` files and supports embedded pictures.
- v2.11.0 requires Go 1.25, matching `apps/api/go.mod`.
- Its `AddPictureFromBytes` API supports PNG/JPEG but not WebP.
- The module already depends on `golang.org/x/image`; import `webp` and `draw` directly to decode every accepted upload type and normalize thumbnails to PNG before embedding.

## Chosen Shape

Generate the workbook in the API rather than the browser:

- The API can read image files directly from the configured upload directory.
- The authoritative unpaginated query and low-stock predicate stay in one backend location.
- The web bundle avoids a large spreadsheet-generation dependency.
- Missing or corrupt individual images can degrade to an empty/labelled image cell without failing the workbook.

Normalize embedded images to bounded PNG thumbnails. This handles WebP compatibility and prevents original 5 MB uploads from being copied unchanged into every workbook.

## Alternatives Rejected

- CSV: cannot embed images.
- HTML renamed to `.xls`: produces format warnings and unreliable image support.
- Browser-side Excel generation: duplicates inventory filtering, adds a large client dependency, and requires browser image conversion.
- Hand-written OpenXML ZIP: more code and compatibility risk than the focused Excel library.

