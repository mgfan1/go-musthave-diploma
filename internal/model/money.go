package model

import (
	"strconv"
	"strings"
)

// Money хранит сумму баллов лояльности с точностью до копейки. В базе суммы
// лежат в numeric(12,2), и складывает и сравнивает их сама база: в Go Money
// только переносит значение между базой, системой расчёта и JSON.
type Money float64

// MarshalJSON выводит сумму числом без экспоненты, округлённым до копейки
// и без незначащих нулей: 729.98, 500.5, 42.
func (m Money) MarshalJSON() ([]byte, error) {
	s := strconv.FormatFloat(float64(m), 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return []byte(s), nil
}
