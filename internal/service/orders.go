package service

import (
	"context"
	"fmt"

	"github.com/mgfan1/go-musthave-diploma/internal/luhn"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// OrderRepository описывает хранилище заказов.
type OrderRepository interface {
	// CreateOrder сохраняет новый заказ в статусе NEW и возвращает владельца
	// заказа и признак того, что заказ создан сейчас. Существующий заказ
	// не меняет.
	CreateOrder(ctx context.Context, userID int64, number string) (int64, bool, error)
	// UserOrders возвращает заказы пользователя от новых к старым.
	UserOrders(ctx context.Context, userID int64) ([]model.Order, error)
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

// Upload принимает номер заказа number от пользователя userID. Возвращает
// true, если заказ принят впервые, и false, если этот пользователь уже
// загружал такой номер: повторная загрузка ничего не меняет. Если номер
// не проходит проверку по алгоритму Луна, возвращает
// model.ErrInvalidOrderNumber, а если его уже загрузил другой пользователь,
// возвращает model.ErrOrderOwnedByOther.
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

// List возвращает заказы пользователя userID от новых к старым.
func (s *Orders) List(ctx context.Context, userID int64) ([]model.Order, error) {
	return s.repo.UserOrders(ctx, userID)
}

// ClaimPending выдаёт фоновому опросу до limit незавершённых заказов
// и возвращает их номера.
func (s *Orders) ClaimPending(ctx context.Context, limit int) ([]string, error) {
	return s.repo.ClaimPendingOrders(ctx, limit)
}

// ApplyAccrual переносит в заказ number результат расчёта result. Начисление
// сохраняется только вместе со статусом PROCESSED и попадает в баланс
// в тот же момент, что и статус. Заказы в окончательных статусах
// не меняются. Для статуса NEW и неизвестных статусов возвращает ошибку:
// результатом расчёта они быть не могут.
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
