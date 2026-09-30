package service

import (
	"context"
	"iter"

	"github.com/mgfan1/go-musthave-diploma/internal/luhn"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// BalanceRepository хранит списания и считает баланс.
type BalanceRepository interface {
	// Balance возвращает баланс пользователя.
	Balance(ctx context.Context, userID int64) (model.Balance, error)
	// Withdraw атомарно проверяет баланс и сохраняет списание. Возвращает
	// model.ErrInsufficientFunds, model.ErrInvalidWithdrawSum или
	// model.ErrUserNotFound, если списание невозможно.
	Withdraw(ctx context.Context, userID int64, order string, sum model.Money) error
	// UserWithdrawals отдаёт списания пользователя от новых к старым.
	UserWithdrawals(ctx context.Context, userID int64) iter.Seq2[model.Withdrawal, error]
}

// Balance показывает баланс и списания пользователя и списывает баллы.
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
// Неположительная сумма даёт model.ErrInvalidWithdrawSum, номер, не прошедший
// проверку Луна, даёт model.ErrInvalidOrderNumber. Остальные ошибки приходят
// из хранилища.
func (s *Balance) Withdraw(ctx context.Context, userID int64, order string, sum model.Money) error {
	if sum <= 0 {
		return model.ErrInvalidWithdrawSum
	}
	if !luhn.Valid(order) {
		return model.ErrInvalidOrderNumber
	}

	return s.repo.Withdraw(ctx, userID, order, sum)
}

// Withdrawals отдаёт списания пользователя userID от новых к старым.
func (s *Balance) Withdrawals(ctx context.Context, userID int64) iter.Seq2[model.Withdrawal, error] {
	return s.repo.UserWithdrawals(ctx, userID)
}
