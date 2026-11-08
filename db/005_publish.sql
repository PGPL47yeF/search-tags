-- Публикация проверенной сборки: атомарно меняем query_tags и query_tags_next местами.
-- Сервис видит либо старые теги, либо новые, промежуточного состояния нет.
EXCHANGE TABLES query_tags AND query_tags_next;
DROP TABLE query_tags_next;
