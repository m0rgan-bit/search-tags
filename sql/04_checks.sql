-- Каждая проверка возвращает число нарушений. Ожидаем 0 везде.
SELECT 'query_equals_tag' AS check_name, count() AS violations
FROM tags WHERE query = tag

UNION ALL SELECT 'users_below_5', count()
FROM tags WHERE users < 5

UNION ALL SELECT 'more_than_10_tags_per_query', count()
FROM (SELECT query FROM tags GROUP BY query HAVING count() > 10)

UNION ALL SELECT 'duplicate_query_tag_pairs', count()
FROM (SELECT query, tag FROM tags GROUP BY query, tag HAVING count() > 1)

UNION ALL SELECT 'rank_outside_1_10', count()
FROM tags WHERE rank < 1 OR rank > 10

UNION ALL SELECT 'rank_not_contiguous_from_1', count()
FROM (SELECT query FROM tags GROUP BY query HAVING min(rank) != 1 OR max(rank) != count())

UNION ALL SELECT 'users_not_descending_by_rank', count()
FROM
(
    SELECT rank, users,
           lagInFrame(users) OVER (PARTITION BY query ORDER BY rank
                                   ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING) AS prev_users
    FROM tags
)
WHERE rank > 1 AND users > prev_users

UNION ALL SELECT 'not_normalized_or_empty', count()
FROM tags
WHERE query = '' OR tag = ''
   OR query != normalize_query(query) OR tag != normalize_query(tag)

-- «Платье » и «платье» дают один и тот же результат
-- (если такого запроса нет в данных, подставьте любой существующий)
UNION ALL SELECT 'variants_return_different_tags',
    toUInt64(
        (SELECT groupArray(tag) FROM (SELECT tag FROM tags WHERE query = normalize_query('Платье ') ORDER BY rank))
        !=
        (SELECT groupArray(tag) FROM (SELECT tag FROM tags WHERE query = normalize_query('платье')  ORDER BY rank))
    );
