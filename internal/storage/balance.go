package storage

import (
	"context"
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
// Проверка баланса и запись списания идут в одной транзакции, которая
// первым делом блокирует строку пользователя: параллельные списания
// одного пользователя выполняются по очереди, и каждое видит баланс
// с учётом предыдущих. Если баллов не хватает, возвращает
// model.ErrInsufficientFunds. Сумма сохраняется с округлением до копеек,
// и если после округления она нулевая, возвращает model.ErrInvalidWithdrawSum.
func (s *PGStorage) Withdraw(ctx context.Context, userID int64, order string, sum model.Money) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("не начал транзакцию списания: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&id)
	if err != nil {
		return fmt.Errorf("не заблокировал счёт пользователя: %w", err)
	}

	var enough bool
	err = tx.QueryRowContext(ctx,
		userTotals+`
		SELECT accrued.total - withdrawn.total >= $2
		FROM accrued, withdrawn`,
		userID, sum,
	).Scan(&enough)
	if err != nil {
		return fmt.Errorf("не проверил баланс: %w", err)
	}
	if !enough {
		return model.ErrInsufficientFunds
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO withdrawals (user_id, order_number, amount) VALUES ($1, $2, $3)`,
		userID, order, sum,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.CheckViolation {
		return model.ErrInvalidWithdrawSum
	}
	if err != nil {
		return fmt.Errorf("не сохранил списание: %w", err)
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
