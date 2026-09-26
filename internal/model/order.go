package model

import "time"

// OrderStatus задаёт этап расчёта начисления по заказу. Заказ появляется
// в StatusNew, может перейти в StatusProcessing и заканчивает в одном
// из окончательных статусов, StatusInvalid или StatusProcessed, после
// которых больше не меняется. Других значений схема базы не допускает.
type OrderStatus string

const (
	// StatusNew означает, что заказ загружен, но система расчёта о нём
	// ещё ничего не сообщила.
	StatusNew OrderStatus = "NEW"
	// StatusProcessing означает, что система расчёта знает о заказе
	// и считает начисление.
	StatusProcessing OrderStatus = "PROCESSING"
	// StatusInvalid означает, что система расчёта отказала в расчёте.
	// Статус окончательный.
	StatusInvalid OrderStatus = "INVALID"
	// StatusProcessed означает, что расчёт окончен и начисление, если оно
	// положено, уже учтено в балансе. Статус окончательный.
	StatusProcessed OrderStatus = "PROCESSED"
)

// Order описывает номер заказа, который пользователь загрузил, чтобы
// получить за него баллы. Номер принадлежит тому, кто загрузил его первым.
type Order struct {
	// Number — номер заказа из ASCII-цифр любой длины, прошедший проверку
	// Луна. Уникален среди всех пользователей.
	Number string
	// Status показывает этап расчёта начисления. Меняется только по ответам
	// системы расчёта.
	Status OrderStatus
	// Accrual — начисленные по заказу баллы. Бывает только у StatusProcessed.
	// Nil означает, что система расчёта суммы не прислала, и это не то же
	// самое, что начисление в ноль баллов.
	Accrual *Money
	// UploadedAt — момент первой загрузки номера. Повторная загрузка его
	// не меняет.
	UploadedAt time.Time
}
