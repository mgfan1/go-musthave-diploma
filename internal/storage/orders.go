package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// CreateOrder сохраняет заказ number пользователя userID в статусе NEW
// и возвращает владельца заказа и признак того, что заказ создан сейчас.
// Если номер уже загружен, существующий заказ не меняется: его статус
// и начисление остаются прежними, а владельцем возвращается тот, кто
// загрузил номер первым. Если пользователя userID нет, возвращает
// model.ErrUserNotFound.
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
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
		return 0, false, model.ErrUserNotFound
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

// ClaimPendingOrders выдаёт на опрос до limit заказов в статусах NEW
// и PROCESSING и возвращает их номера. Первыми идут заказы, которые ещё
// не опрашивались или опрашивались раньше остальных, а выданным заказам
// сразу проставляется время опроса, поэтому следующая выдача достанется
// другим заказам. Строки, которые в этот момент выдаёт другой запрос,
// пропускаются, так что параллельные выдачи не пересекаются.
func (s *PGStorage) ClaimPendingOrders(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`UPDATE orders SET polled_at = now()
		 WHERE number IN (
		     SELECT number FROM orders
		     WHERE status IN ('NEW', 'PROCESSING')
		     ORDER BY polled_at NULLS FIRST, uploaded_at
		     LIMIT $1
		     FOR UPDATE SKIP LOCKED
		 )
		 RETURNING number`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("не выбрал заказы для опроса: %w", err)
	}
	defer rows.Close()

	var numbers []string
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			return nil, fmt.Errorf("не прочитал номер заказа: %w", err)
		}
		numbers = append(numbers, number)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("не выбрал заказы для опроса: %w", err)
	}

	return numbers, nil
}

// UpdateOrder записывает заказу number статус status и начисление accrual.
// Оба поля меняются одним UPDATE, то есть атомарно: баланс, который
// считается по начислениям обработанных заказов, видит статус PROCESSED
// только вместе с суммой. Заказы в окончательных статусах INVALID
// и PROCESSED не меняются.
func (s *PGStorage) UpdateOrder(ctx context.Context, number string, status model.OrderStatus, accrual *model.Money) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE orders SET status = $2, accrual = $3
		 WHERE number = $1 AND status IN ('NEW', 'PROCESSING')`,
		number, status, accrual,
	)
	if err != nil {
		return fmt.Errorf("не обновил заказ: %w", err)
	}

	return nil
}
