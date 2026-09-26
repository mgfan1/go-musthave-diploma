package model

import "time"

// OrderStatus - этап расчёта начисления по заказу. Заказ начинает
// в StatusNew и заканчивает в StatusInvalid или StatusProcessed, после чего
// больше не меняется.
type OrderStatus string

const (
	// StatusNew - заказ загружен, система расчёта о нём ещё ничего
	// не сообщила.
	StatusNew OrderStatus = "NEW"
	// StatusProcessing - система расчёта знает о заказе и считает начисление.
	StatusProcessing OrderStatus = "PROCESSING"
	// StatusInvalid - система расчёта отказала в расчёте.
	StatusInvalid OrderStatus = "INVALID"
	// StatusProcessed - расчёт окончен, начисление, если оно положено,
	// учтено в балансе.
	StatusProcessed OrderStatus = "PROCESSED"
)

// Order - заказ, который пользователь загрузил, чтобы получить за него
// баллы. Номер принадлежит тому, кто загрузил его первым.
type Order struct {
	// Number - номер заказа из ASCII-цифр любой длины, прошедший проверку
	// Луна. Уникален среди всех пользователей.
	Number string
	// Status показывает этап расчёта начисления. Меняется только по ответам
	// системы расчёта.
	Status OrderStatus
	// Accrual - начисленные по заказу баллы. Бывает только у StatusProcessed.
	// Nil означает, что система расчёта суммы не прислала, и это не то же
	// самое, что начисление в ноль баллов.
	Accrual *Money
	// UploadedAt - момент первой загрузки номера. Повторная загрузка его
	// не меняет.
	UploadedAt time.Time
}
