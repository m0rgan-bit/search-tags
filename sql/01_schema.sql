-- Нормализация: нижний регистр (UTF-8, т.е. и кириллица), все виды пробелов -> один пробел, обрезка по краям.
-- Те же правила реализованы в Go (internal/normalize): strings.Fields + strings.ToLower.
CREATE FUNCTION IF NOT EXISTS normalize_query AS (s) ->
    trimBoth(replaceRegexpAll(lowerUTF8(s), '[\\s\\x{00A0}\\x{0085}\\x{1680}\\x{2000}-\\x{200A}\\x{2028}\\x{2029}\\x{202F}\\x{205F}\\x{3000}]+', ' '));

-- Сырые логи.
-- user_id сделан String: тип в parquet заранее неизвестен, а строка принимает и числа, и uuid.
-- Если в файле это целое число, можно заменить на UInt64 (меньше места).
CREATE TABLE IF NOT EXISTS search_logs
(
    timestamp    DateTime,
    user_id      String,
    search_query String
)
ENGINE = MergeTree
ORDER BY (user_id, timestamp);

-- Результат: топ-10 тегов на каждый запрос.
CREATE TABLE IF NOT EXISTS tags
(
    query String,
    tag   String,
    users UInt32,
    rank  UInt8
)
ENGINE = MergeTree
ORDER BY (query, rank);
