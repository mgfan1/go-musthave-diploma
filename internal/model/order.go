package model

import "time"

// OrderStatus задаёт статус обработки заказа в системе лояльности.
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

// Order описывает загруженный пользователем заказ.
type Order struct {
	// Number хранит номер заказа: строку из цифр произвольной длины.
	Number string
	// Status хранит статус обработки заказа.
	Status OrderStatus
	// Accrual хранит начисление за заказ. Равен nil, пока начисления нет,
	// и это не то же самое, что нулевое начисление.
	Accrual *Money
	// UploadedAt хранит время загрузки заказа.
	UploadedAt time.Time
}
