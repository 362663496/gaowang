# Gaowang Inventory Admin

Self-hosted lightweight inventory admin system.

## Local Setup

1. Copy `.env.example` to `.env`.
2. Set a long `AUTH_SECRET` (at least 32 bytes).
3. For a brand-new database, set `INITIAL_ADMIN_NAME`, `INITIAL_ADMIN_EMAIL`, and `INITIAL_ADMIN_PASSWORD` (password ≥ 8 chars). Leave them empty if users already exist.
4. Keep `SESSION_COOKIE_SECURE=false` for plain HTTP local access.
5. Run `make compose-up`.
6. Open `http://localhost:3000` for the web app, or `http://localhost` through Nginx.
7. After the first successful login, remove `INITIAL_ADMIN_PASSWORD` from `.env` and recreate/restart the API container.

## Auth And Permissions

- Login uses an HTTP-only cookie session (`gaowang_session`) stored as a hashed token in PostgreSQL. Sessions last 7 days.
- Roles are fixed: `admin` and `staff`. Admin always has every permission.
- Staff start with zero business permissions after upgrade or first deploy. An admin must open **权限管理** and grant access.
- Product delete is an independent permission from create/edit/toggle.
- API and Web must be deployed together; old development `X-Dev-*` headers are ignored.

## Core Commands

- `make api-test`: run Go tests.
- `make api-run`: run Go API locally.
- `make web-install`: install web dependencies after the web app is scaffolded.
- `make web-dev`: run Next.js locally after the web app is scaffolded.
- `make compose-up`: run the stack.
- `make compose-down`: stop the stack.

## Deployment

1. Copy `.env.example` to `.env` and set production values, especially `AUTH_SECRET`, `POSTGRES_PASSWORD`, SMTP settings, and `HTTP_PORT`.
2. Confirm the target database already has at least one enabled admin, or provide `INITIAL_ADMIN_*` for an empty database only.
3. Back up the database before upgrade.
4. Set `SESSION_COOKIE_SECURE=true` and serve the site over HTTPS. Nginx must forward `X-Forwarded-Proto` (see `deploy/nginx/app.conf`).
5. Deploy API and Web from the same release; staff users will have zero permissions until an admin configures them.
6. Point DNS to the server and run `docker compose up --build -d`.
7. Remove `INITIAL_ADMIN_PASSWORD` after the first successful bootstrap.

The Compose stack runs PostgreSQL, the Go API, the Next.js web app, and Nginx. Nginx routes `/api` to the API, `/uploads` to the shared uploads volume, and all other paths to the web app. Product image uploads under `/uploads/*` remain publicly readable.

## Feishu/Lark Inventory Bot

The optional self-built app bot sends successful inbound, sales outbound, and inventory adjustment notifications to one fixed group. In that group, mention the bot with `查库存 <名称或编码>`, `查商品 <名称或编码>`, `低库存`, or `帮助`.

1. Create an enterprise self-built app, enable its bot, and add these permissions: `im:message:send_as_bot`, `im:message.group_at_msg:readonly`, and `im:resource`.
2. Subscribe to `im.message.receive_v1`, select long-connection event delivery, publish the app, and add the bot to the inventory group.
3. Set `LARK_APP_ID`, `LARK_APP_SECRET`, and that group's `LARK_CHAT_ID` in `.env`, then restart the API. All three must be set together.

No public callback route or Nginx change is needed. Leave all three variables empty to disable the integration or roll it back.

## Restore

Run from the project root:

```bash
deploy/scripts/restore-db.sh /path/to/gaowang-YYYYMMDD-HHMMSS.sql.gz
```

The script reads `.env` when present and restores into the running `postgres` Compose service.
