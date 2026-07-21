# Aliyun Release

## 1. Scope / Trigger

Use this contract for every production release to `ssh aliyun`. API and Web are one release and must be built locally; the server only verifies, extracts, switches, and restarts.

## 2. Signatures

- Local API artifact: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/api`.
- Local Web artifact: build `apps/web` in a local `linux/amd64` glibc Node container, then package `.next/standalone`, `.next/static`, and `public`.
- Release layout: `api/gaowang-api`, `web/server.js`, `web/.next/static`, `web/public`, `meta/deploy-info.txt`.
- Remote target: `/opt/gaowang/releases/<release_id>`; active link: `/opt/gaowang/current`.
- Services: `gaowang-api.service` and `gaowang-web.service`.

## 3. Contracts

- Commit and push the exact release commit before packaging it.
- Run tests and production build locally. Do not run `npm ci`, `npm run build`, `go build`, or Docker builds on the server.
- Package a Linux x86-64 release locally, smoke-test `web/server.js` locally, upload one archive, and verify its SHA-256 remotely.
- Before schema changes, create and verify a PostgreSQL backup. Read the running API process environment without printing credentials; `shared/app.env` may not contain the effective port.
- Extract to a staging path, validate files and commit metadata, then rename to the final release directory.
- Atomically replace `/opt/gaowang/current`, restart API then Web, and retain the previous release for rollback.
- Never copy, replace, or delete `/opt/gaowang/shared/app.env`, shared uploads, shared backups, or the PostgreSQL data volume.

## 4. Validation & Error Matrix

| Condition | Action |
| --- | --- |
| Local test/build or Web smoke fails | Stop before upload |
| Database backup or `gzip -t` fails | Stop before switch |
| Archive checksum/file layout/commit differs | Reject the staged release |
| API restart or health check fails | Restore the previous symlink and restart both services |
| Web/Nginx login check fails | Restore the previous symlink and restart both services |
| Post-release schema, static asset, upload, or log check fails | Roll back and investigate |

## 5. Good / Base / Bad Cases

- Good: local linux/amd64 API and glibc Web build → local smoke → archive/checksum → upload → atomic switch → health/schema checks.
- Base: no schema change still keeps the same backup and rollback flow.
- Bad: upload source and build on Aliyun, or switch `current` before the artifact is complete.

## 6. Tests Required

- Local: Go tests/vet; Web lint, strict TypeScript, Vitest, production build, and standalone `/login` returns `200` in linux/amd64.
- Remote before switch: current services healthy, backup non-empty and passes `gzip -t`, uploaded checksum matches.
- Remote after switch: both services active, API/Nginx health `200`, login/static/upload `200`, expected schema exists, `NRestarts=0`, and recent logs contain no runtime error.

## 7. Wrong vs Correct

```bash
# Wrong: consumes production CPU and makes the release depend on remote registries.
ssh aliyun 'cd /opt/gaowang/release-src/apps/web && npm ci && npm run build'

# Correct: server receives an already-tested immutable release archive.
scp gaowang-release-RELEASE_ID.tar.gz aliyun:/opt/gaowang/releases/.incoming-RELEASE_ID.tar.gz
ssh aliyun 'sha256sum -c RELEASE_ID.sha256 && tar -xzf .incoming-RELEASE_ID.tar.gz -C RELEASE_ID.staging'
```
