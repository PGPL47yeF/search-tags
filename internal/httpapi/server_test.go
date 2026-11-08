package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PGPL47yeF/search-tags/internal/tags"
)

// fakeBackend отвечает заранее заданными тегами и запоминает, с каким запросом его вызвали.
type fakeBackend struct {
	mu      sync.Mutex
	data    map[string][]tags.Tag
	err     error
	block   bool // ждать отмены контекста, как зависшая база
	pingErr error
	calls   []string
}

func (f *fakeBackend) Lookup(ctx context.Context, query string) ([]tags.Tag, error) {
	f.mu.Lock()
	f.calls = append(f.calls, query)
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, errors.New("driver: read: i/o timeout") // драйвер может не обернуть ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.data[query], nil
}

func (f *fakeBackend) Ping(context.Context) error { return f.pingErr }

func (f *fakeBackend) lastCall() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
}

var dressTags = []tags.Tag{
	{Tag: "красное платье", Users: 9264},
	{Tag: "вечернее платье", Users: 6609},
	{Tag: "летнее платье", Users: 5047},
}

func newTestServer(b Backend) (http.Handler, *bytes.Buffer) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	return New(b, 200*time.Millisecond, log), &logs
}

func get(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func apiURL(q string) string { return "/api/tags?q=" + url.QueryEscape(q) }

func TestAPIEmptyQueryIsBadRequest(t *testing.T) {
	backend := &fakeBackend{}
	h, _ := newTestServer(backend)

	for _, target := range []string{"/api/tags", "/api/tags?q=", apiURL("   "), apiURL("\t\n")} {
		t.Run(target, func(t *testing.T) {
			rec := get(h, target)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			var body errorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error == "" {
				t.Fatalf("want JSON error body, got %q", rec.Body.String())
			}
		})
	}
	if len(backend.calls) != 0 {
		t.Errorf("storage must not be queried for empty q, got calls %q", backend.calls)
	}
}

func TestAPIReturnsTagsInRankOrder(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{data: map[string][]tags.Tag{"платье": dressTags}})

	rec := get(h, apiURL("платье"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	var got []tags.Tag
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not a JSON list of tags: %v; %s", err, rec.Body.String())
	}
	if len(got) != len(dressTags) {
		t.Fatalf("got %d tags, want %d", len(got), len(dressTags))
	}
	for i := range got {
		if got[i] != dressTags[i] {
			t.Errorf("tag[%d] = %+v, want %+v", i, got[i], dressTags[i])
		}
	}
	// Формат полей — как в ТЗ: tag и users.
	if !strings.Contains(rec.Body.String(), `{"tag":"красное платье","users":9264}`) {
		t.Errorf("unexpected JSON shape: %s", rec.Body.String())
	}
}

func TestAPINormalizesQueryBeforeLookup(t *testing.T) {
	backend := &fakeBackend{data: map[string][]tags.Tag{"платье": dressTags}}
	h, _ := newTestServer(backend)

	first := get(h, apiURL("Платье"))
	if backend.lastCall() != "платье" {
		t.Fatalf("storage got %q, want normalized %q", backend.lastCall(), "платье")
	}
	second := get(h, apiURL("платье "))
	if first.Code != http.StatusOK || first.Body.String() != second.Body.String() {
		t.Fatalf("«Платье» and «платье » differ:\n%s\n%s", first.Body.String(), second.Body.String())
	}
}

func TestAPINoTagsIsEmptyList(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{data: map[string][]tags.Tag{}})

	rec := get(h, apiURL("редкий запрос"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Fatalf("body = %q, want []", body)
	}
}

func TestAPIStorageErrorIsHidden(t *testing.T) {
	h, logs := newTestServer(&fakeBackend{err: errors.New("dial tcp 10.0.0.7:9000: connection refused")})

	rec := get(h, apiURL("платье"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.7") {
		t.Errorf("internal details leaked to client: %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "connection refused") || !strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Errorf("error should be logged, logs: %s", logs.String())
	}
}

func TestAPITimeoutIsGatewayTimeout(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{block: true})

	started := time.Now()
	rec := get(h, apiURL("платье"))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", rec.Code)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("request took %s, timeout is not applied", elapsed)
	}
}

func TestClientDisconnectIsNotAnError(t *testing.T) {
	h, logs := newTestServer(&fakeBackend{block: true})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, apiURL("платье"), nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	h.ServeHTTP(rec, req)

	if rec.Body.Len() != 0 {
		t.Errorf("nothing should be written to a gone client, got %q", rec.Body.String())
	}
	if strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Errorf("client disconnect logged as error: %s", logs.String())
	}
}

func TestAPIRejectsOtherMethods(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, apiURL("платье"), nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{})
	if rec := get(h, "/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("healthy: status = %d, want 200", rec.Code)
	}

	h, _ = newTestServer(&fakeBackend{pingErr: errors.New("down")})
	if rec := get(h, "/healthz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy: status = %d, want 503", rec.Code)
	}
}

func TestPageWithoutQueryShowsForm(t *testing.T) {
	backend := &fakeBackend{}
	h, _ := newTestServer(backend)

	rec := get(h, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`<form action="/" method="get"`, `name="q"`, "Найти"} {
		if !strings.Contains(body, want) {
			t.Errorf("page has no %q", want)
		}
	}
	if strings.Contains(body, "пока нет тегов") {
		t.Error("empty page must not say there are no tags")
	}
	if len(backend.calls) != 0 {
		t.Error("storage must not be queried without q")
	}
}

func TestPageShowsTagsAsLinksThatFillTheField(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{data: map[string][]tags.Tag{"платье": dressTags}})

	body := get(h, "/?q="+url.QueryEscape("  Платье ")).Body.String()

	// Поле хранит введённый текст как есть.
	if !strings.Contains(body, `value="  Платье "`) {
		t.Errorf("input does not keep the typed query")
	}
	// Каждый тег — ссылка, которая открывает страницу с этим тегом в поле.
	for _, tag := range dressTags {
		link := `href="/?q=` + strings.ToLower(url.PathEscape(tag.Tag)) + `"`
		if !strings.Contains(strings.ToLower(body), link) {
			t.Errorf("no link %s", link)
		}
		if !strings.Contains(body, ">"+tag.Tag+"</a>") {
			t.Errorf("no visible tag %q", tag.Tag)
		}
	}
	// Порядок на странице — порядок rank.
	if strings.Index(body, "красное платье") > strings.Index(body, "летнее платье") {
		t.Error("tags are not in rank order")
	}
}

func TestPageNoTagsMessage(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{})

	rec := get(h, "/?q=zzz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Для этого запроса пока нет тегов") {
		t.Error("no «Для этого запроса пока нет тегов» message")
	}
}

func TestPageEscapesUserInputAndTags(t *testing.T) {
	evil := `"><script>alert(1)</script>`
	h, _ := newTestServer(&fakeBackend{data: map[string][]tags.Tag{
		`"><script>alert(1)</script>`: {{Tag: `<img src=x onerror=alert(2)>`, Users: 7}},
	}})

	body := get(h, "/?q="+url.QueryEscape(evil)).Body.String()
	for _, raw := range []string{"<script>", "<img src=x"} {
		if strings.Contains(body, raw) {
			t.Errorf("unescaped %q in page", raw)
		}
	}
	if !strings.Contains(body, "&lt;img src=x onerror=alert(2)&gt;") {
		t.Error("tag text should be shown escaped")
	}
}

func TestPageStorageFailure(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{err: errors.New("boom")})

	rec := get(h, "/?q=платье")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Не получилось загрузить теги") || strings.Contains(body, "boom") {
		t.Errorf("unexpected failure page: %s", body)
	}
	if strings.Contains(body, "пока нет тегов") {
		t.Error("failure must not look like «no tags»")
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	h, _ := newTestServer(&fakeBackend{})
	if rec := get(h, "/admin"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAccessLogHasNoSearchText(t *testing.T) {
	h, logs := newTestServer(&fakeBackend{})
	get(h, apiURL("секретный запрос"))
	out, _ := io.ReadAll(logs)
	if strings.Contains(string(out), "секретный") {
		t.Errorf("search text leaked into logs: %s", out)
	}
	if !strings.Contains(string(out), `"path":"/api/tags"`) || !strings.Contains(string(out), `"status":200`) {
		t.Errorf("access log line missing: %s", out)
	}
}
