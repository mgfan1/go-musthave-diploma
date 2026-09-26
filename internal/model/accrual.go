package model

// AccrualResult описывает ответ системы расчёта по заказу, уже переведённый
// в статусы Гофермарта.
type AccrualResult struct {
	// Status хранит статус заказа, который следует из ответа системы расчёта.
	Status OrderStatus
	// Amount хранит рассчитанное начисление. Равен nil, если система расчёта
	// его не прислала.
	Amount *Money
}
