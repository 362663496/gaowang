# Gaowang Inventory Admin

Self-hosted lightweight inventory admin system.

## Local Setup

1. Copy `.env.example` to `.env`.
2. Run `make compose-up`.
3. Open `http://localhost:3000` for the web app, or `http://localhost` through Nginx.

## Core Commands

- `make api-test`: run Go tests.
- `make api-run`: run Go API locally.
- `make web-install`: install web dependencies after the web app is scaffolded.
- `make web-dev`: run Next.js locally after the web app is scaffolded.
- `make compose-up`: run the stack.
- `make compose-down`: stop the stack.

## Deployment

1. Copy `.env.example` to `.env` and set production values, especially `AUTH_SECRET`, `POSTGRES_PASSWORD`, SMTP settings, and `HTTP_PORT`.
2. Point DNS to the server.
3. Run `docker compose up --build -d`.
4. Put HTTPS in front of Nginx with your certificate manager or cloud load balancer.

The Compose stack runs PostgreSQL, the Go API, the Next.js web app, and Nginx. Nginx routes `/api` to the API, `/uploads` to the shared uploads volume, and all other paths to the web app.

## Restore

Run from the project root:

```bash
deploy/scripts/restore-db.sh /path/to/gaowang-YYYYMMDD-HHMMSS.sql.gz
```

The script reads `.env` when present and restores into the running `postgres` Compose service.
