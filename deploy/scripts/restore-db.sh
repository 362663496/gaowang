#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: deploy/scripts/restore-db.sh /path/to/backup.sql.gz"
  exit 1
fi

backup_file="$1"
if [ ! -f "$backup_file" ]; then
  echo "backup file not found: $backup_file"
  exit 1
fi

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

gunzip -t "$backup_file"
gunzip -c "$backup_file" | docker compose exec -T postgres psql -U "${POSTGRES_USER:-gaowang}" -d "${POSTGRES_DB:-gaowang}"
