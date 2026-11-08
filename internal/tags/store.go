// Package tags читает готовые поисковые теги из ClickHouse.
package tags

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Tag — подсказка под строкой поиска и число пользователей, которые после запроса искали её.
type Tag struct {
	Tag   string `json:"tag"`
	Users uint32 `json:"users"`
}

// lookupSQL — единственный запрос сервиса. Значение подставляется только через параметр:
// драйвер экранирует его сам, текст запроса от пользовательского ввода не зависит.
const lookupSQL = `SELECT tag, users FROM query_tags WHERE query = ? ORDER BY rank`

// ConnConfig описывает подключение к ClickHouse.
type ConnConfig struct {
	Addr     string
	Database string
	User     string
	Password string
	// Timeout ограничивает установку соединения и чтение ответа. Контекст запроса
	// на установку соединения не влияет, поэтому таймаут нужен и здесь.
	Timeout time.Duration
}

// Connect открывает пул соединений. Сеть при этом не трогается: проверить доступность можно через Ping.
func Connect(cfg ConnConfig) *sql.DB {
	db := clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
		DialTimeout: cfg.Timeout,
		ReadTimeout: cfg.Timeout,
	})
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	db.SetConnMaxLifetime(time.Hour)
	return db
}

// Store отдаёт теги для уже нормализованного запроса.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Lookup возвращает теги в порядке rank. Если тегов нет, возвращает пустой (не nil) срез.
func (s *Store) Lookup(ctx context.Context, query string) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, lookupSQL, query)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	found := make([]Tag, 0, 10)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.Tag, &t.Users); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		found = append(found, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}
	return found, nil
}

// Ping проверяет, что ClickHouse доступен.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
