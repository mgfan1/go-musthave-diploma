package model

import "time"

// Balance описывает счёт баллов пользователя.
type Balance struct {
	// Current хранит баллы, доступные для списания: сумму начислений
	// по обработанным заказам за вычетом всех списаний.
	Current Money
	// Withdrawn хранит сумму всех списаний за время жизни счёта.
	Withdrawn Money
}

// Withdrawal описывает списание баллов в счёт оплаты заказа.
type Withdrawal struct {
	// Order хранит номер заказа, в счёт которого списаны баллы.
	Order string
	// Sum хранит списанную сумму.
	Sum Money
	// ProcessedAt хранит время списания.
	ProcessedAt time.Time
}
