package service

import (
	"context"

	"github.com/mgfan1/go-musthave-diploma/internal/luhn"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// BalanceRepository описывает хранилище счетов баллов.
type BalanceRepository interface {
	// Balance возвращает баланс пользователя.
	Balance(ctx context.Context, userID int64) (model.Balance, error)
	// Withdraw атомарно проверяет баланс и списывает баллы. Если баллов
	// не хватает, возвращает model.ErrInsufficientFunds.
	Withdraw(ctx context.Context, userID int64, order string, sum model.Money) error
	// UserWithdrawals возвращает списания пользователя от новых к старым.
	UserWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

// Balance показывает пользователю его баланс и списания и списывает баллы
// в счёт оплаты новых заказов.
type Balance struct {
	repo BalanceRepository
}

// NewBalance создаёт сервис баланса поверх хранилища repo.
func NewBalance(repo BalanceRepository) *Balance {
	return &Balance{repo: repo}
}

// Get возвращает баланс пользователя userID.
func (s *Balance) Get(ctx context.Context, userID int64) (model.Balance, error) {
	return s.repo.Balance(ctx, userID)
}

// Withdraw списывает sum баллов пользователя userID в счёт заказа order.
// Если сумма не положительная, возвращает model.ErrInvalidWithdrawSum,
// если номер заказа не проходит проверку по алгоритму Луна, возвращает
// model.ErrInvalidOrderNumber, а если баллов не хватает, возвращает
// model.ErrInsufficientFunds. Хватает ли баллов, решает хранилище.
func (s *Balance) Withdraw(ctx context.Context, userID int64, order string, sum model.Money) error {
	if sum <= 0 {
		return model.ErrInvalidWithdrawSum
	}
	if !luhn.Valid(order) {
		return model.ErrInvalidOrderNumber
	}

	return s.repo.Withdraw(ctx, userID, order, sum)
}

// Withdrawals возвращает списания пользователя userID от новых к старым.
func (s *Balance) Withdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return s.repo.UserWithdrawals(ctx, userID)
}
