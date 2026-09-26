// Package luhn проверяет номера по алгоритму Луна.
package luhn

// Valid сообщает, проходит ли number проверку по алгоритму Луна. Номер
// разбирается посимвольно, поэтому длина не ограничена разрядностью чисел.
// Пустая строка и строка с любым символом, кроме ASCII-цифр, проверку
// не проходят.
func Valid(number string) bool {
	if number == "" {
		return false
	}

	sum := 0
	double := false
	for i := len(number) - 1; i >= 0; i-- {
		c := number[i]
		if c < '0' || c > '9' {
			return false
		}

		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}

	return sum%10 == 0
}
