// Package normalize приводит поисковый запрос к виду, в котором он хранится в query_tags.
package normalize

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// spaces повторяет '\s+' из SQL-функции normalize_search. Go и ClickHouse
// используют RE2, в обоих \s — это ASCII-пробелы [\t\n\f\r ]; неразрывный пробел сюда не входит.
var spaces = regexp.MustCompile(`\s+`)

// Query переводит запрос в нижний регистр, сжимает пробельные последовательности
// до одного пробела и обрезает пробелы по краям — так же, как normalize_search в db/001_schema.sql.
func Query(raw string) string {
	q := lowerLikeClickHouse(raw)
	q = spaces.ReplaceAllString(q, " ")
	return strings.Trim(q, " ")
}

// lowerLikeClickHouse — нижний регистр по правилам lowerUTF8 из ClickHouse, а не strings.ToLower:
//   - буквы из keepCase ClickHouse не меняет, и мы тоже;
//   - байты, которые не складываются в корректный UTF-8, остаются как есть
//     (strings.ToLower заменил бы их на U+FFFD, и ключ разошёлся бы с SQL).
func lowerLikeClickHouse(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			b.WriteByte(c)
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteByte(c)
			i++
			continue
		}
		if !unicode.Is(keepCase, r) {
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
		i += size
	}
	return b.String()
}
