package model

// AccrualResult описывает ответ системы расчёта по заказу, уже переведённый
// в статусы Гофермарта.
type AccrualResult struct {
	// Status задаёт новый статус заказа: REGISTERED и PROCESSING системы
	// расчёта становятся StatusProcessing, INVALID и PROCESSED переносятся
	// как есть. StatusNew здесь не бывает.
	Status OrderStatus
	// Amount переносит сумму из ответа системы расчёта как есть, при любом
	// статусе. Nil означает, что суммы в ответе не было. В заказ сумма
	// попадает только вместе со статусом PROCESSED, при остальных статусах
	// её отбрасывает сервис заказов.
	Amount *Money
}
