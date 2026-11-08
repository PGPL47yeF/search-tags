package httpapi

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"net/http"

	"github.com/PGPL47yeF/search-tags/internal/tags"
)

//go:embed web/search.html
var webFS embed.FS

// searchPage разбирается один раз при старте: ошибка в шаблоне ломает сборку тестов, а не прод.
var searchPage = template.Must(template.ParseFS(webFS, "web/search.html"))

type pageView struct {
	// Query — то, что ввёл пользователь, без нормализации: так поле не «прыгает» после поиска.
	Query    string
	Searched bool
	Tags     []tags.Tag
	// Problem — понятное пользователю описание сбоя, если теги загрузить не удалось.
	Problem string
}

// page: GET /?q=... — форма поиска и до 10 тегов-ссылок.
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	view := pageView{Query: r.URL.Query().Get("q")}
	status := http.StatusOK

	_, found, err := s.lookup(r)
	switch {
	case errors.Is(err, errQueryRequired):
		// Пустая форма — это просто первый заход на страницу, а не ошибка.
	case err != nil:
		f := s.classify(r, err)
		s.logFailure(r, f, err)
		if f.clientGone {
			return
		}
		view.Searched = true
		status = f.status
		view.Problem = "Не получилось загрузить теги. Попробуйте ещё раз чуть позже."
	default:
		view.Searched = true
		view.Tags = found
	}

	// Рендерим в буфер: если шаблон упадёт на середине, пользователь получит 500, а не половину страницы.
	var buf bytes.Buffer
	if err := searchPage.Execute(&buf, view); err != nil {
		s.log.Error("render search page", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
