-- Файл data/logs.parquet смонтирован в user_files контейнера (см. docker-compose.yml).
-- Явные приведения типов: если в parquet тип другой, значения приводятся к типам таблицы.
INSERT INTO search_logs (timestamp, user_id, search_query)
SELECT
    toDateTime(timestamp),
    toString(user_id),
    ifNull(search_query, '')
FROM file('logs.parquet', 'Parquet');
