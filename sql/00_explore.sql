-- Разведка «грязных» данных. Запускать после 02_load_data.sql, смотреть глазами.

-- 0. Типы колонок в самом parquet-файле
DESCRIBE TABLE file('logs.parquet', 'Parquet');

-- 1. Общая картина
SELECT count() AS rows,
       uniqExact(user_id) AS users,
       min(timestamp) AS ts_min,
       max(timestamp) AS ts_max
FROM search_logs;

-- 2. Мусор в запросах: пустые, из одних пробелов, слишком длинные
SELECT countIf(search_query = '')                              AS empty_raw,
       countIf(normalize_query(search_query) = '')             AS empty_after_norm,
       countIf(length(search_query) > 256)                     AS too_long,
       countIf(search_query != normalize_query(search_query))  AS needs_normalization
FROM search_logs;

-- 3. Мусор в пользователях и времени
SELECT countIf(user_id = '' OR user_id = '0')        AS bad_user,
       countIf(timestamp < toDateTime('2000-01-01')) AS ts_too_old,
       countIf(timestamp > now())                    AS ts_in_future
FROM search_logs;

-- 4. Полные дубликаты строк
SELECT count() - uniqExact(user_id, timestamp, search_query) AS exact_duplicates
FROM search_logs;

-- 5. Подозрительно активные пользователи (боты?)
SELECT user_id, count() AS c FROM search_logs GROUP BY user_id ORDER BY c DESC LIMIT 10;

-- 6. Самые частые запросы после нормализации
SELECT normalize_query(search_query) AS q, count() AS c
FROM search_logs GROUP BY q ORDER BY c DESC LIMIT 20;
