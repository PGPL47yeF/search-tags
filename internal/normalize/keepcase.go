package normalize

import "unicode"

// keepCase — заглавные буквы, которые strings.ToLower переводит в строчные, а lowerUTF8
// в ClickHouse 24.8 оставляет как есть: у ClickHouse своя, более старая таблица регистров.
// Чтобы ключи в Go и в SQL совпадали, эти буквы не трогаем.
//
// Список получен сравнением unicode.ToLower и lowerUTF8 по всем 1 112 064 кодовым точкам.
// TestNormalizationMatchesSQLForAllRunes (test/integration) повторяет это сравнение на живом
// ClickHouse: если образ обновят и таблица разойдётся, тест покажет, какие символы изменились.
var keepCase = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x0130, Hi: 0x0130, Stride: 1}, // İ
		{Lo: 0x01C5, Hi: 0x01C5, Stride: 1}, // ǅ — «заглавные» диграфы
		{Lo: 0x01C8, Hi: 0x01C8, Stride: 1}, // ǈ
		{Lo: 0x01CB, Hi: 0x01CB, Stride: 1}, // ǋ
		{Lo: 0x01F2, Hi: 0x01F2, Stride: 1}, // ǲ
		{Lo: 0x023A, Hi: 0x023A, Stride: 1}, // Ⱥ
		{Lo: 0x023E, Hi: 0x023E, Stride: 1}, // Ⱦ
		{Lo: 0x13A0, Hi: 0x13F5, Stride: 1}, // чероки
		{Lo: 0x1C89, Hi: 0x1C89, Stride: 1}, // кириллическая заглавная TJE
		{Lo: 0x1C90, Hi: 0x1CBA, Stride: 1}, // грузинский мтаврули
		{Lo: 0x1CBD, Hi: 0x1CBF, Stride: 1},
		{Lo: 0x1E9E, Hi: 0x1E9E, Stride: 1}, // ẞ
		{Lo: 0x1F88, Hi: 0x1F8F, Stride: 1}, // греческие с прогэграммени
		{Lo: 0x1F98, Hi: 0x1F9F, Stride: 1},
		{Lo: 0x1FA8, Hi: 0x1FAF, Stride: 1},
		{Lo: 0x1FBC, Hi: 0x1FBC, Stride: 1},
		{Lo: 0x1FCC, Hi: 0x1FCC, Stride: 1},
		{Lo: 0x1FFC, Hi: 0x1FFC, Stride: 1},
		{Lo: 0x2126, Hi: 0x2126, Stride: 1}, // Ω (знак ома)
		{Lo: 0x212A, Hi: 0x212B, Stride: 1}, // K (кельвин), Å (ангстрем)
		{Lo: 0x2132, Hi: 0x2132, Stride: 1}, // Ⅎ
		{Lo: 0x2160, Hi: 0x216F, Stride: 1}, // римские цифры Ⅰ..Ⅿ
		{Lo: 0x2183, Hi: 0x2183, Stride: 1}, // Ↄ
		{Lo: 0x24B6, Hi: 0x24CF, Stride: 1}, // Ⓐ..Ⓩ
		{Lo: 0x2C00, Hi: 0x2C2F, Stride: 1}, // глаголица
		{Lo: 0x2C60, Hi: 0x2C62, Stride: 2}, // латиница расширенная C
		{Lo: 0x2C63, Hi: 0x2C64, Stride: 1},
		{Lo: 0x2C67, Hi: 0x2C6D, Stride: 2},
		{Lo: 0x2C6E, Hi: 0x2C70, Stride: 1},
		{Lo: 0x2C72, Hi: 0x2C72, Stride: 1},
		{Lo: 0x2C75, Hi: 0x2C75, Stride: 1},
		{Lo: 0x2C7E, Hi: 0x2C80, Stride: 1}, // и начало коптского
		{Lo: 0x2C82, Hi: 0x2CE2, Stride: 2},
		{Lo: 0x2CEB, Hi: 0x2CED, Stride: 2},
		{Lo: 0x2CF2, Hi: 0x2CF2, Stride: 1},
		{Lo: 0xA78D, Hi: 0xA78D, Stride: 1}, // латиница расширенная D
		{Lo: 0xA7AA, Hi: 0xA7AE, Stride: 1},
		{Lo: 0xA7B0, Hi: 0xA7B4, Stride: 1},
		{Lo: 0xA7B6, Hi: 0xA7C4, Stride: 2},
		{Lo: 0xA7C5, Hi: 0xA7C7, Stride: 1},
		{Lo: 0xA7C9, Hi: 0xA7CB, Stride: 2},
		{Lo: 0xA7CC, Hi: 0xA7DC, Stride: 2},
		{Lo: 0xA7F5, Hi: 0xA7F5, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x104B0, Hi: 0x104D3, Stride: 1}, // осейдж
		{Lo: 0x10570, Hi: 0x1057A, Stride: 1}, // виткуки
		{Lo: 0x1057C, Hi: 0x1058A, Stride: 1},
		{Lo: 0x1058C, Hi: 0x10592, Stride: 1},
		{Lo: 0x10594, Hi: 0x10595, Stride: 1},
		{Lo: 0x10C80, Hi: 0x10CB2, Stride: 1}, // древневенгерское письмо
		{Lo: 0x10D50, Hi: 0x10D65, Stride: 1}, // гарай
		{Lo: 0x16E40, Hi: 0x16E5F, Stride: 1}, // медефайдрин
		{Lo: 0x16EA0, Hi: 0x16EB8, Stride: 1},
		{Lo: 0x1E900, Hi: 0x1E921, Stride: 1}, // адлам
	},
}
