package model

// AccrualResult описывает ответ системы расчёта по заказу, уже переведённый
// в статусы Гофермарта.
type AccrualResult struct {
	// Status - новый статус заказа. StatusNew здесь не бывает.
	Status OrderStatus
	// Amount - сумма из ответа системы расчёта при любом статусе. Nil, если
	// суммы в ответе не было.
	Amount *Money
}
