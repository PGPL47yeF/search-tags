-- Загрузка data/logs.parquet (каталог смонтирован в user_files сервера).
-- Пишем во временную таблицу и подменяем целиком: повторный запуск не задваивает строки,
-- а сервис до конца загрузки видит старые данные.
DROP TABLE IF EXISTS search_log_next;
CREATE TABLE search_log_next AS search_log;

INSERT INTO search_log_next (timestamp, user_id, search_query)
SELECT timestamp, user_id, search_query
FROM file('logs.parquet', Parquet)
-- В схеме parquet все колонки Nullable. В этих логах NULL нет, но строка без времени
-- или пользователя не может участвовать в паре, поэтому такие строки отбрасываем.
WHERE timestamp IS NOT NULL AND user_id IS NOT NULL AND search_query IS NOT NULL;

EXCHANGE TABLES search_log AND search_log_next;
DROP TABLE search_log_next;
