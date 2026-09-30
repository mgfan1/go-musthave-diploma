package service

import (
	"context"
	"fmt"
	"iter"

	"github.com/mgfan1/go-musthave-diploma/internal/luhn"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// OrderRepository хранит заказы.
type OrderRepository interface {
	// CreateOrder сохраняет новый заказ в статусе NEW и возвращает владельца
	// номера и признак того, что заказ создан сейчас. Существующий заказ
	// не меняет. Если пользователя нет, а номер новый, возвращает
	// model.ErrUserNotFound.
	CreateOrder(ctx context.Context, userID int64, number string) (int64, bool, error)
	// UserOrders отдаёт заказы пользователя от новых к старым.
	UserOrders(ctx context.Context, userID int64) iter.Seq2[model.Order, error]
	// ClaimPendingOrders выдаёт на опрос до limit незавершённых заказов,
	// которые дольше остальных не опрашивались, и возвращает их номера.
	ClaimPendingOrders(ctx context.Context, limit int) ([]string, error)
	// UpdateOrder атомарно записывает статус и начисление незавершённого
	// заказа. Заказы в окончательных статусах не меняет.
	UpdateOrder(ctx context.Context, number string, status model.OrderStatus, accrual *model.Money) error
}

// Orders принимает номера заказов от пользователей, отдаёт их списки
// и переносит в заказы результаты расчёта начислений.
type Orders struct {
	repo OrderRepository
}

// NewOrders создаёт сервис заказов поверх хранилища repo.
func NewOrders(repo OrderRepository) *Orders {
	return &Orders{repo: repo}
}

// Upload принимает номер заказа number от пользователя userID и возвращает
// true, если заказ новый, и false, если этот пользователь уже загружал такой
// номер. Номер, не прошедший проверку Луна, даёт model.ErrInvalidOrderNumber,
// номер другого пользователя даёт model.ErrOrderOwnedByOther. Ошибки хранилища
// возвращаются как есть.
func (s *Orders) Upload(ctx context.Context, userID int64, number string) (bool, error) {
	if !luhn.Valid(number) {
		return false, model.ErrInvalidOrderNumber
	}

	ownerID, created, err := s.repo.CreateOrder(ctx, userID, number)
	if err != nil {
		return false, err
	}
	if ownerID != userID {
		return false, model.ErrOrderOwnedByOther
	}

	return created, nil
}

// List отдаёт заказы пользователя userID от новых к старым.
func (s *Orders) List(ctx context.Context, userID int64) iter.Seq2[model.Order, error] {
	return s.repo.UserOrders(ctx, userID)
}

// ClaimPending выдаёт фоновому опросу до limit незавершённых заказов
// и возвращает их номера.
func (s *Orders) ClaimPending(ctx context.Context, limit int) ([]string, error) {
	return s.repo.ClaimPendingOrders(ctx, limit)
}

// ApplyAccrual переносит в заказ number результат расчёта. Начисление
// сохраняется только вместе со статусом PROCESSED. Для NEW и неизвестных
// статусов возвращает ошибку.
func (s *Orders) ApplyAccrual(ctx context.Context, number string, result model.AccrualResult) error {
	switch result.Status {
	case model.StatusProcessed:
		return s.repo.UpdateOrder(ctx, number, result.Status, result.Amount)
	case model.StatusProcessing, model.StatusInvalid:
		return s.repo.UpdateOrder(ctx, number, result.Status, nil)
	default:
		return fmt.Errorf("недопустимый статус результата расчёта %q", result.Status)
	}
}
