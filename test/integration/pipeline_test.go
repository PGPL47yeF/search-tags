//go:build integration

// Интеграционные тесты: настоящие SQL-файлы из db/ на настоящем ClickHouse.
// Каждый прогон создаёт отдельную базу и удаляет её в конце.
//
//	docker compose up -d clickhouse
//	CLICKHOUSE_USER=tags CLICKHOUSE_PASSWORD=tags go test -tags integration ./test/integration
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/PGPL47yeF/search-tags/internal/normalize"
	"github.com/PGPL47yeF/search-tags/internal/tags"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func connConfig(database string) tags.ConnConfig {
	return tags.ConnConfig{
		Addr:     env("CLICKHOUSE_ADDR", "localhost:9000"),
		Database: database,
		User:     env("CLICKHOUSE_USER", "default"),
		Password: env("CLICKHOUSE_PASSWORD", ""),
		Timeout:  10 * time.Second,
	}
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// runSQLFile выполняет файл из db/ по одному оператору. Комментарии -- только целыми строками,
// операторы заканчиваются ';' в конце строки — этого соглашения держатся все файлы в db/.
func runSQLFile(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(), "db", name))
	if err != nil {
		t.Fatal(err)
	}
	var stmt strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		stmt.WriteString(line)
		stmt.WriteByte('\n')
		if strings.HasSuffix(strings.TrimSpace(line), ";") {
			if _, err := db.Exec(stmt.String()); err != nil {
				t.Fatalf("%s: %v\n%s", name, err, stmt.String())
			}
			stmt.Reset()
		}
	}
	if strings.TrimSpace(stmt.String()) != "" {
		t.Fatalf("%s: statement without trailing ';': %s", name, stmt.String())
	}
}

// newDatabase создаёт пустую базу со схемой из db/001_schema.sql.
func newDatabase(t *testing.T) *sql.DB {
	t.Helper()
	admin := tags.Connect(connConfig("default"))
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("ClickHouse is not reachable at %s: %v", env("CLICKHOUSE_ADDR", "localhost:9000"), err)
	}

	name := fmt.Sprintf("tags_it_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + name + " SYNC"); err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
	})

	db := tags.Connect(connConfig(name))
	t.Cleanup(func() { db.Close() })
	runSQLFile(t, db, "001_schema.sql")
	return db
}

type event struct {
	at    time.Time
	user  uint64
	query string
}

func insert(t *testing.T, db *sql.DB, events []event) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare("INSERT INTO search_log (timestamp, user_id, search_query)")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if _, err := stmt.Exec(e.at, e.user, e.query); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// fixture собирает логи, где каждый сценарий живёт на своих пользователях и своих запросах.
type fixture struct {
	events   []event
	nextUser uint64
}

var base = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

// session добавляет одному новому пользователю цепочку запросов с заданными паузами между ними.
func (f *fixture) session(gaps []time.Duration, queries ...string) uint64 {
	f.nextUser++
	user := f.nextUser
	at := base.Add(time.Duration(user) * time.Hour) // сессии разных людей не пересекаются по времени
	for i, q := range queries {
		if i > 0 {
			at = at.Add(gaps[i-1])
		}
		f.events = append(f.events, event{at: at, user: user, query: q})
	}
	return user
}

// users заводит n разных пользователей с одной и той же цепочкой и паузой в минуту.
func (f *fixture) users(n int, queries ...string) {
	gaps := make([]time.Duration, len(queries))
	for i := range gaps {
		gaps[i] = time.Minute
	}
	for i := 0; i < n; i++ {
		f.session(gaps, queries...)
	}
}

func lookup(t *testing.T, store *tags.Store, raw string) []tags.Tag {
	t.Helper()
	found, err := store.Lookup(context.Background(), normalize.Query(raw))
	if err != nil {
		t.Fatalf("lookup %q: %v", raw, err)
	}
	return found
}

func tagNames(found []tags.Tag) []string {
	names := make([]string, len(found))
	for i, t := range found {
		names[i] = t.Tag
	}
	return names
}

func TestTagsPipeline(t *testing.T) {
	db := newDatabase(t)
	var f fixture

	// Порог: 5 разных людей — тег, 4 — нет.
	f.users(5, "порог", "пять человек")
	f.users(4, "порог", "четыре человека")

	// Граница 30 минут включительно, с точностью до миллисекунды.
	for i := 0; i < 5; i++ {
		f.session([]time.Duration{30 * time.Minute}, "граница", "ровно 30 минут")
		f.session([]time.Duration{30*time.Minute + time.Millisecond}, "граница", "на миллисекунду дольше")
	}

	// Пара — только соседние запросы: из «а б в» получаются «а->б» и «б->в», но не «а->в».
	f.users(5, "сосед а", "сосед б", "сосед в")

	// Повтор того же запроса не тег, но следующий за повтором запрос считается.
	f.users(5, "повтор", "повтор", "после повтора")

	// Бот повторяет пару 100 раз: это 1 пользователь, а не 100.
	f.users(3, "бот", "цель бота")
	gaps := make([]time.Duration, 199)
	for i := range gaps {
		gaps[i] = 10 * time.Second
	}
	chain := make([]string, 200)
	for i := range chain {
		chain[i] = map[bool]string{true: "бот", false: "цель бота"}[i%2 == 0]
	}
	f.session(gaps, chain...)

	// Разные написания сводятся к одному запросу и одному тегу.
	for _, pair := range [][2]string{
		{"Платье", "КРАСНОЕ платье"}, {"платье ", "красное   платье"}, {"  ПЛАТЬЕ", "красное платье "},
		{"платье\t", "Красное Платье"}, {"Платье  ", "красное\tплатье"},
	} {
		f.users(1, pair[0], pair[1])
	}

	// Запрос из одних пробелов выпадает и не разрывает пару вокруг себя.
	f.users(5, "до пустого", "   ", "после пустого")

	// Топ-10: users по убыванию, при равенстве — tag по алфавиту.
	f.users(7, "топ", "топ лидер")
	for i := 1; i <= 12; i++ {
		f.users(5, "топ", fmt.Sprintf("топ %02d", i))
	}

	// 300 кандидатов у одного запроса: rank не должен переполниться после 255.
	for i := 1; i <= 300; i++ {
		f.users(5, "много", fmt.Sprintf("много %03d", i))
	}

	// Порядок определяется временем, а не порядком вставки: меняем строки местами в батче.
	for i := 0; i < 5; i++ {
		f.session([]time.Duration{time.Minute}, "раньше", "позже")
		n := len(f.events)
		f.events[n-1], f.events[n-2] = f.events[n-2], f.events[n-1]
	}

	// Кавычки и обратный слэш доходят до базы как есть.
	f.users(5, `книги o'reilly \ go`, `go "в действии"`)

	insert(t, db, f.events)
	runSQLFile(t, db, "003_tags.sql")
	store := tags.NewStore(db)

	expect := func(raw string, want ...string) {
		t.Helper()
		got := tagNames(lookup(t, store, raw))
		if len(want) == 0 {
			want = []string{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("tags for %q = %q, want %q", raw, got, want)
		}
	}

	expect("порог", "пять человек")
	expect("граница", "ровно 30 минут")
	expect("сосед а", "сосед б")
	expect("сосед б", "сосед в")
	expect("повтор", "после повтора")
	expect("бот")
	expect("платье", "красное платье")
	expect("Платье", "красное платье")
	expect("до пустого", "после пустого")
	// Вставлены в обратном порядке, но по времени «раньше» идёт первым.
	expect("раньше", "позже")
	expect("позже")
	expect(`Книги O'Reilly \ GO`, `go "в действии"`)
	expect("не было такого запроса")

	top := lookup(t, store, "топ")
	wantTop := []string{"топ лидер", "топ 01", "топ 02", "топ 03", "топ 04", "топ 05", "топ 06", "топ 07", "топ 08", "топ 09"}
	if !reflect.DeepEqual(tagNames(top), wantTop) {
		t.Errorf("top10 = %q, want %q", tagNames(top), wantTop)
	}
	if len(top) > 0 && top[0].Users != 7 {
		t.Errorf("leader users = %d, want 7", top[0].Users)
	}

	many := lookup(t, store, "много")
	if len(many) != 10 || many[0].Tag != "много 001" || many[9].Tag != "много 010" {
		t.Errorf("300 candidates: got %q", tagNames(many))
	}

	// Те же инварианты, что в 004_checks.sql, на фикстуре.
	var violations uint64
	err := db.QueryRow(`
		SELECT
			(SELECT countIf(query = tag OR users < 5) FROM query_tags)
		  + (SELECT count() FROM (SELECT query FROM query_tags GROUP BY query HAVING count() > 10))
		  + (SELECT count() FROM (SELECT query, arraySort(groupArray(rank)) AS r FROM query_tags GROUP BY query HAVING r != range(1, length(r) + 1)))
	`).Scan(&violations)
	if err != nil {
		t.Fatal(err)
	}
	if violations != 0 {
		t.Errorf("invariants violated %d times", violations)
	}
}

// Пересборка тегов идемпотентна и не оставляет за собой временную таблицу.
func TestRebuildIsRepeatable(t *testing.T) {
	db := newDatabase(t)
	var f fixture
	f.users(6, "раз", "два")
	insert(t, db, f.events)

	runSQLFile(t, db, "003_tags.sql")
	runSQLFile(t, db, "003_tags.sql")

	store := tags.NewStore(db)
	got := lookup(t, store, "раз")
	if len(got) != 1 || got[0] != (tags.Tag{Tag: "два", Users: 6}) {
		t.Fatalf("after two rebuilds: %+v", got)
	}
	var leftovers uint64
	if err := db.QueryRow("SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = 'query_tags_next'").Scan(&leftovers); err != nil {
		t.Fatal(err)
	}
	if leftovers != 0 {
		t.Error("query_tags_next was not dropped")
	}
}

// Go и SQL нормализуют независимо; на кириллице, латинице и ASCII-пробелах результат обязан совпадать.
func TestNormalizationMatchesSQL(t *testing.T) {
	db := newDatabase(t)
	inputs := []string{
		"Платье", "платье ", "  ПЛАТЬЕ  красное ", "Кроссовки\tNIKE\nАкция", "ЁЛКА", "ёжик",
		"iPhone 15 PRO", "o'reilly \\ go", `"кавычки"`, "\r\n\t\f", "", "a  b   c", "Ünïcödé Ÿ", "платье миди",
	}
	for _, in := range inputs {
		var sqlResult string
		if err := db.QueryRow("SELECT normalize_search(?)", in).Scan(&sqlResult); err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if goResult := normalize.Query(in); goResult != sqlResult {
			t.Errorf("%q: Go %q, SQL %q", in, goResult, sqlResult)
		}
	}
}

// То же на всех настоящих написаниях из логов, если они загружены в базу search.
func TestNormalizationMatchesSQLOnRealLogs(t *testing.T) {
	db := tags.Connect(connConfig(env("CLICKHOUSE_DB", "search")))
	defer db.Close()

	rows, err := db.Query("SELECT DISTINCT search_query, query_norm FROM search_log")
	if err != nil {
		t.Skipf("real logs are not loaded: %v", err)
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var raw, norm string
		if err := rows.Scan(&raw, &norm); err != nil {
			t.Fatal(err)
		}
		if got := normalize.Query(raw); got != norm {
			t.Errorf("%q: Go %q, SQL %q", raw, got, norm)
		}
		checked++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Skip("search_log is empty")
	}
	t.Logf("checked %d distinct spellings", checked)
}
