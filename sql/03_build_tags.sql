-- Полный пересчёт тегов. Можно запускать повторно.
TRUNCATE TABLE tags;

INSERT INTO tags (query, tag, users, rank)
WITH
    -- нормализуем, выкидываем пустые запросы и пустых пользователей,
    -- схлопываем полные дубли (тот же пользователь, то же время, тот же текст)
    cleaned AS
    (
        SELECT DISTINCT
            user_id,
            timestamp,
            normalize_query(search_query) AS q
        FROM search_logs
        WHERE user_id != ''
          AND q != ''
    ),
    -- правило 2: у каждого пользователя упорядочиваем запросы по времени и берём следующий
    -- (для leadInFrame нужна явная рамка на всю партицию, иначе вернётся значение по умолчанию)
    sequenced AS
    (
        SELECT
            user_id,
            q,
            timestamp,
            leadInFrame(q)         OVER w AS next_q,
            leadInFrame(timestamp) OVER w AS next_ts
        FROM cleaned
        WINDOW w AS (
            PARTITION BY user_id
            ORDER BY timestamp, q
            ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING
        )
    ),
    -- правила 3 и 4: следующий запрос есть, прошло не больше 30 минут, второй не равен первому
    pairs AS
    (
        SELECT user_id, q AS first_q, next_q AS second_q
        FROM sequenced
        WHERE next_q != ''
          AND dateDiff('second', timestamp, next_ts) <= 1800
          AND q != next_q
    ),
    -- правила 5 и 6: уникальные пользователи (точный подсчёт, чтобы порог 5 не плавал), не меньше 5
    counted AS
    (
        SELECT first_q AS query, second_q AS tag, uniqExact(user_id) AS users
        FROM pairs
        GROUP BY query, tag
        HAVING users >= 5
    ),
    -- правило 7: топ-10 на каждый первый запрос; при равенстве users сортируем по tag, чтобы результат был детерминированным
    ranked AS
    (
        SELECT
            query, tag, users,
            row_number() OVER (PARTITION BY query ORDER BY users DESC, tag ASC) AS rank
        FROM counted
    )
SELECT query, tag, users, rank
FROM ranked
WHERE rank <= 10;
