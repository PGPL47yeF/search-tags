package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
)

type errorBody struct {
	Error string `json:"error"`
}

// apiTags: GET /api/tags?q=... -> [{"tag": "...", "users": N}, ...] в порядке rank.
func (s *Server) apiTags(w http.ResponseWriter, r *http.Request) {
	_, found, err := s.lookup(r)
	switch {
	case errors.Is(err, errQueryRequired):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
	case err != nil:
		f := s.classify(r, err)
		s.logFailure(r, f, err)
		if !f.clientGone {
			// Текст ошибки драйвера наружу не отдаём: в нём адреса и детали инфраструктуры.
			writeJSON(w, f.status, errorBody{Error: f.message})
		}
	default:
		writeJSON(w, http.StatusOK, found)
	}
}

// writeJSON сначала кодирует ответ целиком, чтобы при ошибке кодирования не отдать
// клиенту 200 с обрезанным телом.
func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
