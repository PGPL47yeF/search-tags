-- Сборка тегов в query_tags_next. Живая query_tags не трогается: подмена делается
-- в 005_publish.sql и только после того, как 004_checks.sql проверил новую таблицу.
DROP TABLE IF EXISTS query_tags_next;
CREATE TABLE query_tags_next AS query_tags;

INSERT INTO query_tags_next (query, tag, users, rank)
WITH
    -- Для каждого запроса пользователя берём следующий за ним запрос того же пользователя.
    -- Пустые после нормализации запросы выкидываем заранее: это не поиск, и пару они не разрывают.
    -- query_norm во втором ключе сортировки делает порядок детерминированным при одинаковом времени
    -- (в этих логах таких случаев нет, проверено в db/profile.sql).
    steps AS
    (
        SELECT
            user_id,
            query_norm AS query,
            timestamp,
            leadInFrame(query_norm, 1, '') OVER next_step AS next_query,
            leadInFrame(timestamp) OVER next_step AS next_timestamp
        FROM search_log
        WHERE query_norm != ''
        WINDOW next_step AS (PARTITION BY user_id ORDER BY timestamp, query_norm ROWS BETWEEN CURRENT ROW AND 1 FOLLOWING)
    ),
    -- Пара засчитывается, если следующий запрос есть, отличается от текущего и сделан не позже чем через 30 минут.
    -- Пустой next_query означает «следующего нет»: пустые запросы отфильтрованы выше.
    -- Сравниваем время с точностью до миллисекунд: пауза ровно 30:00.000 подходит, 30:00.001 уже нет.
    pairs AS
    (
        SELECT query, next_query AS tag, uniqExact(user_id) AS users
        FROM steps
        WHERE next_query != ''
          AND next_query != query
          AND next_timestamp <= timestamp + INTERVAL 30 MINUTE
        GROUP BY query, tag
        -- Считаем людей, а не события: бот, повторивший пару 500 раз, даёт одного пользователя.
        HAVING users >= 5
    ),
    top10 AS
    (
        -- tag во второй позиции сортировки задаёт порядок при равном числе пользователей.
        SELECT query, tag, users
        FROM pairs
        ORDER BY query, users DESC, tag
        LIMIT 10 BY query
    )
SELECT
    query,
    tag,
    toUInt32(users),
    -- rank считается уже по 10 оставшимся строкам, поэтому в UInt8 помещается всегда.
    toUInt8(row_number() OVER (PARTITION BY query ORDER BY users DESC, tag))
FROM top10;
