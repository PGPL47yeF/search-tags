// Package httpapi — HTTP-слой: JSON API, страница поиска и проверка здоровья.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/PGPL47yeF/search-tags/internal/normalize"
	"github.com/PGPL47yeF/search-tags/internal/tags"
)

// Backend — то, что нужно серверу от хранилища. В тестах подменяется фейком.
type Backend interface {
	Lookup(ctx context.Context, query string) ([]tags.Tag, error)
	Ping(ctx context.Context) error
}

type Server struct {
	backend Backend
	timeout time.Duration
	log     *slog.Logger
}

// New собирает обработчик со всеми маршрутами. timeout ограничивает одно обращение к ClickHouse.
func New(backend Backend, timeout time.Duration, log *slog.Logger) http.Handler {
	s := &Server{backend: backend, timeout: timeout, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tags", s.apiTags)
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /{$}", s.page)
	return s.accessLog(mux)
}

// errQueryRequired — ответ на пустой q, в том числе на q из одних пробелов.
var errQueryRequired = errors.New("query parameter q is required")

// lookup нормализует запрос и ходит в хранилище с таймаутом.
// Возвращает нормализованный запрос, чтобы вызывающий мог его показать или залогировать.
func (s *Server) lookup(r *http.Request) (string, []tags.Tag, error) {
	query := normalize.Query(r.URL.Query().Get("q"))
	if query == "" {
		return "", nil, errQueryRequired
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	found, err := s.backend.Lookup(ctx, query)
	if err != nil {
		// Драйвер не всегда оборачивает причину отмены, поэтому добавляем её из контекста сами.
		if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(err, ctxErr) {
			err = errors.Join(err, ctxErr)
		}
		return query, nil, err
	}
	if found == nil {
		found = []tags.Tag{}
	}
	return query, found, nil
}

// failure описывает, как ответить на ошибку хранилища.
type failure struct {
	status  int
	message string
	// clientGone: клиент ушёл сам, отвечать некому и ошибкой сервера это не считается.
	clientGone bool
}

func (s *Server) classify(r *http.Request, err error) failure {
	switch {
	case r.Context().Err() != nil:
		return failure{clientGone: true}
	case errors.Is(err, context.DeadlineExceeded):
		return failure{status: http.StatusGatewayTimeout, message: "tags storage timed out"}
	default:
		return failure{status: http.StatusInternalServerError, message: "tags storage unavailable"}
	}
}

func (s *Server) logFailure(r *http.Request, f failure, err error) {
	if f.clientGone {
		s.log.Info("client went away before tags were loaded", "path", r.URL.Path)
		return
	}
	s.log.Error("load tags", "path", r.URL.Path, "status", f.status, "error", err)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	if err := s.backend.Ping(ctx); err != nil {
		s.log.Warn("health check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
