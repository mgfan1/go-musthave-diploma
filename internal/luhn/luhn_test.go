package luhn

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValid(t *testing.T) {
	cases := []struct {
		name   string
		number string
		want   bool
	}{
		{"пример из ТЗ", "12345678903", true},
		{"пример из ТЗ с испорченной цифрой", "12345678902", false},
		{"классический пример", "79927398713", true},
		{"номер карты", "4561261212345467", true},
		{"номер карты с опечаткой", "4561261212345464", false},
		{"перестановка соседних цифр", "21345678903", false},
		{"одна цифра ноль", "0", true},
		{"одна ненулевая цифра", "7", false},
		{"две цифры", "18", true},
		{"длиннее int64", strings.Repeat("0", 40) + "12345678903", true},
		{"длиннее int64 с опечаткой", strings.Repeat("0", 40) + "12345678902", false},
		{"пустая строка", "", false},
		{"буквы", "abc", false},
		{"буква внутри номера", "1234567a903", false},
		{"пробел внутри номера", "12345 678903", false},
		{"знак минус", "-12345678903", false},
		{"дробная точка", "1234567890.3", false},
		{"арабско-индийские цифры", "١٢٣", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, Valid(c.number))
		})
	}
}

func BenchmarkValid(b *testing.B) {
	cases := []struct {
		name   string
		number string
	}{
		{"пример из ТЗ", "12345678903"},
		{"номер карты", "4561261212345467"},
		{"длиннее int64", strings.Repeat("0", 40) + "12345678903"},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Valid(c.number)
			}
		})
	}
}
