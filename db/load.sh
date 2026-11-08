#!/usr/bin/env bash
# Одноразовый загрузчик: схема -> логи -> теги -> проверки.
# Запускается сервисом loader из docker-compose.yml; повторный запуск безопасен.
set -euo pipefail

ch() {
    clickhouse-client \
        --host "${CLICKHOUSE_HOST:-clickhouse}" \
        --user "${CLICKHOUSE_USER:-default}" \
        --password "${CLICKHOUSE_PASSWORD:-}" \
        --database "${CLICKHOUSE_DB:-search}" \
        "$@"
}

# Ждём сервер по сетевому имени. При первом старте образ поднимает временный сервер
# только на localhost, поэтому healthcheck внутри контейнера мог бы пройти раньше времени.
echo "waiting for clickhouse..."
for attempt in $(seq 1 90); do
    if ch --query 'SELECT 1' >/dev/null 2>&1; then
        break
    fi
    if [ "$attempt" -eq 90 ]; then
        echo "clickhouse is not reachable after 90s" >&2
        exit 1
    fi
    sleep 1
done

echo "schema"
ch --multiquery --queries-file /db/001_schema.sql

if ! ch --query "SELECT 1 FROM file('logs.parquet', Parquet) LIMIT 1" >/dev/null 2>&1; then
    echo "data/logs.parquet not found: put the logs file there and run 'docker compose up' again" >&2
    exit 1
fi

echo "load logs"
ch --multiquery --queries-file /db/002_load.sql

echo "build tags"
ch --multiquery --queries-file /db/003_tags.sql

echo "checks"
ch --multiquery --queries-file /db/004_checks.sql

ch --query "SELECT
    (SELECT count() FROM search_log) AS log_rows,
    (SELECT uniqExact(query) FROM query_tags) AS queries_with_tags,
    (SELECT count() FROM query_tags) AS tag_rows
FORMAT Vertical"
echo "done"
