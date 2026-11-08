-- Профиль сырых логов: что в них грязного и как это влияет на решения.
-- Запуск: docker compose exec clickhouse clickhouse-client --database search --queries-file /db/profile.sql
-- Результат последнего прогона: results/profile.txt

-- 1. Объём и период
SELECT count() AS rows, uniqExact(user_id) AS users, min(timestamp) AS first_event, max(timestamp) AS last_event,
       uniqExact(toDate(timestamp)) AS days
FROM search_log
FORMAT Vertical;

-- 2. Мусор в тексте запросов
SELECT
    countIf(search_query = '')                         AS empty_raw,
    countIf(query_norm = '')                           AS empty_after_norm,
    countIf(search_query != lowerUTF8(search_query))   AS with_upper_case,
    countIf(search_query != trimBoth(search_query))    AS with_edge_spaces,
    countIf(match(search_query, '\\s{2,}'))            AS with_space_runs,
    countIf(match(search_query, '[\\t\\n\\r\\f\\v]'))  AS with_control_whitespace,
    countIf(match(search_query, '\\x{00A0}'))          AS with_nbsp,
    countIf(NOT isValidUTF8(search_query))             AS invalid_utf8,
    countIf(search_query != query_norm)                AS changed_by_norm,
    uniqExact(search_query)                            AS distinct_raw,
    uniqExact(query_norm)                              AS distinct_normalized
FROM search_log
FORMAT Vertical;

-- 3. Сколько написаний у самых частых запросов
SELECT query_norm, uniqExact(search_query) AS spellings, count() AS rows
FROM search_log
GROUP BY query_norm
ORDER BY rows DESC
LIMIT 5
FORMAT PrettyCompactMonoBlock;

-- 4. Дубли и одинаковое время у одного пользователя: от этого зависит, кто чей «сосед»
SELECT
    (SELECT count() FROM search_log) - (SELECT count() FROM (SELECT DISTINCT timestamp, user_id, search_query FROM search_log)) AS exact_duplicates,
    (SELECT count() FROM (SELECT user_id, timestamp FROM search_log GROUP BY user_id, timestamp HAVING count() > 1)) AS same_user_same_time
FORMAT Vertical;

-- 5. Подозрительно активные пользователи: много событий на мало разных запросов
SELECT user_id, count() AS events, uniqExact(query_norm) AS distinct_queries, round(events / distinct_queries) AS events_per_query
FROM search_log
GROUP BY user_id
ORDER BY events_per_query DESC
LIMIT 5
FORMAT PrettyCompactMonoBlock;

-- 6. Паузы между соседними запросами: сколько пар отсекает граница 30 минут и сколько лежит ровно на ней
SELECT
    countIf(gap_ms <= 1800000)              AS within_30_min,
    countIf(gap_ms > 1800000)               AS over_30_min,
    countIf(gap_ms = 1800000)               AS exactly_30_min,
    countIf(gap_ms > 1800000 AND gap_ms < 1801000) AS over_by_less_than_1s
FROM
(
    SELECT dateDiff('millisecond', timestamp, leadInFrame(timestamp) OVER w) AS gap_ms,
           leadInFrame(query_norm, 1, '') OVER w AS next_query
    FROM search_log
    WHERE query_norm != ''
    WINDOW w AS (PARTITION BY user_id ORDER BY timestamp, query_norm ROWS BETWEEN CURRENT ROW AND 1 FOLLOWING)
)
WHERE next_query != ''
FORMAT Vertical;
