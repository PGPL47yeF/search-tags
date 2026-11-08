// Package normalize приводит поисковый запрос к виду, в котором он хранится в query_tags.
package normalize

import (
	"regexp"
	"strings"
)

// spaces повторяет '\s+' из SQL-функции normalize_search. Go и ClickHouse
// используют RE2, в обоих \s — это ASCII-пробелы [\t\n\f\r ]; неразрывный пробел сюда не входит.
var spaces = regexp.MustCompile(`\s+`)

// Query переводит запрос в нижний регистр, сжимает пробельные последовательности
// до одного пробела и обрезает пробелы по краям — так же, как db/001_schema.sql.
func Query(raw string) string {
	q := strings.ToLower(raw)
	q = spaces.ReplaceAllString(q, " ")
	return strings.Trim(q, " ")
}
