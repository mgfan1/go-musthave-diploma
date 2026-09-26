package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// CreateOrder сохраняет заказ number пользователя userID в статусе NEW
// и возвращает владельца заказа и признак того, что заказ создан сейчас.
// Если номер уже загружен, существующий заказ не меняется: его статус
// и начисление остаются прежними, а владельцем возвращается тот, кто
// загрузил номер первым.
func (s *PGStorage) CreateOrder(ctx context.Context, userID int64, number string) (int64, bool, error) {
	var ownerID int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO orders (number, user_id) VALUES ($1, $2)
		 ON CONFLICT (number) DO NOTHING
		 RETURNING user_id`,
		number, userID,
	).Scan(&ownerID)
	if err == nil {
		return ownerID, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("не сохранил заказ: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `SELECT user_id FROM orders WHERE number = $1`, number).Scan(&ownerID)
	if err != nil {
		return 0, false, fmt.Errorf("не прочитал владельца заказа: %w", err)
	}

	return ownerID, false, nil
}

// UserOrders возвращает заказы пользователя userID от новых к старым.
// Если заказов нет, возвращает пустой срез.
func (s *PGStorage) UserOrders(ctx context.Context, userID int64) ([]model.Order, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT number, status, accrual, uploaded_at
		 FROM orders
		 WHERE user_id = $1
		 ORDER BY uploaded_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("не прочитал заказы: %w", err)
	}
	defer rows.Close()

	orders := make([]model.Order, 0)
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.Number, &o.Status, &o.Accrual, &o.UploadedAt); err != nil {
			return nil, fmt.Errorf("не прочитал заказ: %w", err)
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("не прочитал заказы: %w", err)
	}

	return orders, nil
}
