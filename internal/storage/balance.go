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

const userTotals = `
	WITH accrued AS (
	    SELECT COALESCE(SUM(accrual), 0) AS total
	    FROM orders
	    WHERE user_id = $1 AND status = 'PROCESSED'
	), withdrawn AS (
	    SELECT COALESCE(SUM(amount), 0) AS total
	    FROM withdrawals
	    WHERE user_id = $1
	)`

// Balance считает баланс пользователя userID одним запросом: начисления
// по обработанным заказам минус списания и отдельно сумму списаний.
// Суммы складывает база в numeric, так что копейки не теряются. Если
// у пользователя нет ни начислений, ни списаний, обе суммы равны нулю.
func (s *PGStorage) Balance(ctx context.Context, userID int64) (model.Balance, error) {
	var b model.Balance
	err := s.db.QueryRowContext(ctx,
		userTotals+`
		SELECT accrued.total - withdrawn.total, withdrawn.total
		FROM accrued, withdrawn`,
		userID,
	).Scan(&b.Current, &b.Withdrawn)
	if err != nil {
		return model.Balance{}, fmt.Errorf("не прочитал баланс: %w", err)
	}

	return b, nil
}

// Withdraw списывает sum баллов пользователя userID в счёт заказа order.
// Сумма округляется до копеек, и с балансом сравнивается уже округлённая сумма.
// Параллельные списания одного пользователя выполняются по очереди. Если баллов
// не хватает, возвращает model.ErrInsufficientFunds, если сумма после округления
// нулевая или не помещается в numeric(12,2), model.ErrInvalidWithdrawSum, а если
// пользователя нет, model.ErrUserNotFound. CTE с суммой материализован, иначе
// приведение типа выполнится при планировании и огромная сумма даст ошибку
// переполнения вместо ErrInsufficientFunds.
func (s *PGStorage) Withdraw(ctx context.Context, userID int64, order string, sum model.Money) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("не начал транзакцию списания: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("не заблокировал счёт пользователя: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		userTotals+`, amount AS MATERIALIZED (
		    SELECT round(CAST($2 AS numeric), 2) AS value
		)
		INSERT INTO withdrawals (user_id, order_number, amount)
		SELECT $1, $3, amount.value
		FROM accrued, withdrawn, amount
		WHERE accrued.total - withdrawn.total >= amount.value`,
		userID, sum, order,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == pgerrcode.CheckViolation || pgErr.Code == pgerrcode.NumericValueOutOfRange) {
		return model.ErrInvalidWithdrawSum
	}
	if err != nil {
		return fmt.Errorf("не сохранил списание: %w", err)
	}

	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("не проверил баланс: %w", err)
	}
	if inserted == 0 {
		return model.ErrInsufficientFunds
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("не завершил списание: %w", err)
	}

	return nil
}

// UserWithdrawals возвращает списания пользователя userID от новых
// к старым. Если списаний нет, возвращает пустой срез.
func (s *PGStorage) UserWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT order_number, amount, processed_at
		 FROM withdrawals
		 WHERE user_id = $1
		 ORDER BY processed_at DESC, id DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("не прочитал списания: %w", err)
	}
	defer rows.Close()

	withdrawals := make([]model.Withdrawal, 0)
	for rows.Next() {
		var w model.Withdrawal
		if err := rows.Scan(&w.Order, &w.Sum, &w.ProcessedAt); err != nil {
			return nil, fmt.Errorf("не прочитал списание: %w", err)
		}
		withdrawals = append(withdrawals, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("не прочитал списания: %w", err)
	}

	return withdrawals, nil
}
