-- Нормализация запроса: нижний регистр, любые пробельные последовательности -> один пробел,
-- пробелы по краям убраны. lowerUTF8, а не lower: lower не трогает кириллицу.
-- Go-версия в internal/normalize повторяет это выражение, совпадение проверяет интеграционный тест.
CREATE OR REPLACE FUNCTION normalize_search AS (s) -> trimBoth(replaceRegexpAll(lowerUTF8(s), '\\s+', ' '));

-- Сырые логи. Исходный текст запроса хранится как есть, нормализованный считается один раз при вставке.
CREATE TABLE IF NOT EXISTS search_log
(
    timestamp    DateTime64(3, 'UTC') CODEC(Delta, ZSTD(1)),
    user_id      UInt64,
    search_query LowCardinality(String),
    -- То же выражение, что в normalize_search: функцию в MATERIALIZED не используем,
    -- чтобы таблица не зависела от объекта, который можно удалить или пересоздать.
    query_norm   LowCardinality(String) MATERIALIZED trimBoth(replaceRegexpAll(lowerUTF8(search_query), '\\s+', ' '))
)
ENGINE = MergeTree
PARTITION BY toDate(timestamp)
ORDER BY (user_id, timestamp);

-- Готовые теги: до 10 строк на запрос, сервис читает их по точному совпадению query.
CREATE TABLE IF NOT EXISTS query_tags
(
    query String,
    tag   String,
    users UInt32,
    rank  UInt8
)
ENGINE = MergeTree
ORDER BY (query, rank);
