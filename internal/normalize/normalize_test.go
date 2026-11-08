package normalize

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"уже нормализован", "платье", "платье"},
		{"кириллица в верхнем регистре", "ПЛАТЬЕ", "платье"},
		{"смешанный регистр", "Платье", "платье"},
		{"пробел в конце", "платье ", "платье"},
		{"пробелы по краям", "   платье  ", "платье"},
		{"несколько пробелов внутри", "красное    платье", "красное платье"},
		{"табы и переводы строк", "красное\t\tплатье\r\n", "красное платье"},
		{"всё сразу", "  Кроссовки   NIKE\tАкция ", "кроссовки nike акция"},
		{"буква ё сохраняется", "ЧЁРНОЕ платье", "чёрное платье"},
		{"цифры и знаки не трогаем", "iPhone 15 Pro-Max!", "iphone 15 pro-max!"},
		{"пустая строка", "", ""},
		{"только пробелы", " \t\n ", ""},
		// Как в SQL: \s в RE2 не включает неразрывный пробел, поэтому он остаётся частью запроса.
		{"неразрывный пробел не схлопывается", "платье миди", "платье миди"},
		// Ниже — места, где strings.ToLower и lowerUTF8 из ClickHouse расходятся. Ведём себя как ClickHouse.
		{"İ не трогаем, как lowerUTF8", "İSTANBUL", "İstanbul"},
		{"знак кельвина не трогаем", "100 K", "100 K"},
		{"латинская K переводится", "100 K", "100 k"},
		{"ẞ не трогаем", "STRAẞE", "straẞe"},
		{"обычные греческие буквы переводим", "ΣΟΦΙΑ", "σοφια"},
		{"битые байты сохраняются как есть", "A\xff\xfeB", "a\xff\xfeb"},
		{"обрезанная кириллица сохраняется", "Я\xd0", "я\xd0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Query(tt.in); got != tt.want {
				t.Errorf("Query(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Варианты из ТЗ должны превращаться в один и тот же ключ.
func TestSpellingVariantsShareKey(t *testing.T) {
	variants := []string{"Платье", "платье ", "ПЛАТЬЕ", "  платье", "платье\t"}
	for _, v := range variants {
		if got := Query(v); got != "платье" {
			t.Errorf("Query(%q) = %q, want %q", v, got, "платье")
		}
	}
}

var asciiSpaceRun = regexp.MustCompile(`[\t\n\f\r ]{2,}`)

// Свойства, которые должны выполняться для любой строки, а не только для примеров выше.
func FuzzQuery(f *testing.F) {
	for _, seed := range []string{"Платье", "  a  b  ", "\t\n", "ΣΑΣ", "İstanbul", "\xff\xfe", "a  b"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := Query(in)
		if Query(out) != out {
			t.Fatalf("not idempotent: Query(%q) = %q, Query(again) = %q", in, out, Query(out))
		}
		if strings.HasPrefix(out, " ") || strings.HasSuffix(out, " ") {
			t.Fatalf("edge spaces left: %q", out)
		}
		if asciiSpaceRun.MatchString(out) || strings.ContainsAny(out, "\t\n\f\r") {
			t.Fatalf("whitespace not collapsed: %q", out)
		}
		if utf8.ValidString(in) && !utf8.ValidString(out) {
			t.Fatalf("valid UTF-8 became invalid: %q -> %q", in, out)
		}
		// Регистр и пробелы — единственное, что меняется: U+FFFD из ниоткуда не появляется.
		if strings.Count(out, "�") > strings.Count(in, "�") {
			t.Fatalf("replacement characters introduced: %q -> %q", in, out)
		}
	})
}
