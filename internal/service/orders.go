package service

import (
	"context"

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
}

// Orders принимает номера заказов от пользователей и отдаёт их списки.
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
