#!/usr/bin/env bash
# Запускает SQL-проверки и падает, если есть хотя бы одно нарушение.
set -euo pipefail
cd "$(dirname "$0")/.."
out=$(docker compose exec -T clickhouse clickhouse-client \
      --user app --password app --database tags_db \
      --format TSV --multiquery < sql/04_checks.sql)
printf 'check\tviolations\n'
echo "$out"
if echo "$out" | awk -F'\t' '$2 != 0 {bad=1} END {exit !bad}'; then
  echo "FAILED"; exit 1
fi
echo "ALL CHECKS PASSED"
