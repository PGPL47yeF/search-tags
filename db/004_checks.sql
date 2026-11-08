-- Проверки результата. В каждой строке failed — число нарушений, должно быть 0.
-- Загрузчик выполняет файл после сборки тегов и падает, если хоть одна проверка не прошла.
CREATE TEMPORARY TABLE check_results
(
    id     UInt8,
    check  String,
    failed UInt64
)
ENGINE = Memory;

INSERT INTO check_results
SELECT 1, 'тег совпадает с запросом', countIf(query = tag)
FROM query_tags
UNION ALL
SELECT 2, 'меньше 5 пользователей', countIf(users < 5)
FROM query_tags
UNION ALL
SELECT 3, 'больше 10 тегов на запрос', count()
FROM (SELECT query FROM query_tags GROUP BY query HAVING count() > 10)
UNION ALL
-- «Платье» и «платье » после нормализации должны давать один и тот же непустой список.
SELECT 4, 'Платье и «платье » дают разный результат',
    (SELECT groupArray((tag, users, rank)) FROM (SELECT * FROM query_tags WHERE query = normalize_search('Платье') ORDER BY rank))
        != (SELECT groupArray((tag, users, rank)) FROM (SELECT * FROM query_tags WHERE query = normalize_search('платье ') ORDER BY rank))
    OR (SELECT count() FROM query_tags WHERE query = normalize_search('Платье')) = 0
UNION ALL
SELECT 5, 'таблица тегов пустая', (SELECT count() FROM query_tags) = 0
UNION ALL
SELECT 6, 'query или tag не нормализованы', countIf(query != normalize_search(query) OR tag != normalize_search(tag) OR query = '' OR tag = '')
FROM query_tags
UNION ALL
-- Колонка query_norm и функция normalize_search описаны отдельно: следим, что они не разошлись.
SELECT 7, 'query_norm расходится с normalize_search', countIf(query_norm != normalize_search(search_query))
FROM search_log
UNION ALL
SELECT 8, 'rank идёт не подряд с 1', count()
FROM (SELECT query, arraySort(groupArray(rank)) AS ranks FROM query_tags GROUP BY query HAVING ranks != range(1, length(ranks) + 1))
UNION ALL
SELECT 9, 'users растёт с увеличением rank', count()
FROM (
    SELECT users, lagInFrame(users) OVER (PARTITION BY query ORDER BY rank ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) AS prev_users, rank
    FROM query_tags
)
WHERE rank > 1 AND users > prev_users
UNION ALL
-- Пары ботов (1 пользователь, сотни повторов) не должны попасть в теги.
SELECT 10, 'в тегах пара бота', count()
FROM query_tags
WHERE (query, tag) IN (('платье', 'странный запрос'), ('кроссовки', 'кроссовки nike акция'));

SELECT check, failed, if(failed = 0, 'ok', 'FAIL') AS status
FROM check_results
ORDER BY id
FORMAT PrettyCompactMonoBlock;

SELECT throwIf(sum(failed) > 0, 'Есть непройденные проверки, см. таблицу выше') AS all_checks_passed
FROM check_results
FORMAT Null;
