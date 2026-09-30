package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"

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

// withdrawIfEnough сохраняет списание, только если баллов на балансе хватает.
// CTE amount объявлен AS MATERIALIZED, иначе PostgreSQL встроит его в запрос,
// приведение суммы к типу колонки выполнится ещё при планировании, и огромная
// сумма даст ошибку переполнения вместо ErrInsufficientFunds.
const withdrawIfEnough = userTotals + `, amount AS MATERIALIZED (
	    SELECT round(CAST($2 AS numeric), 2) AS value
	)
	INSERT INTO withdrawals (user_id, order_number, amount)
	SELECT $1, $3, amount.value
	FROM accrued, withdrawn, amount
	WHERE accrued.total - withdrawn.total >= amount.value`

// Balance возвращает баланс пользователя userID: начисления по обработанным
// заказам минус списания, и сумму списаний. Если нет ни того, ни другого,
// обе суммы нулевые.
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

// Withdraw списывает sum баллов пользователя userID в счёт заказа order,
// округлив сумму до копеек. Списания одного пользователя идут по очереди.
// Возвращает model.ErrInsufficientFunds, если баллов не хватает,
// model.ErrInvalidWithdrawSum, если округлённая сумма нулевая или
// не помещается в numeric(12,2), и model.ErrUserNotFound, если пользователя нет.
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

	res, err := tx.ExecContext(ctx, withdrawIfEnough, userID, sum, order)
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

// UserWithdrawals отдаёт списания пользователя userID от новых к старым.
func (s *PGStorage) UserWithdrawals(ctx context.Context, userID int64) iter.Seq2[model.Withdrawal, error] {
	return queryRows(ctx, s.db, "списания", withdrawalFields,
		`SELECT order_number, amount, processed_at
		 FROM withdrawals
		 WHERE user_id = $1
		 ORDER BY processed_at DESC, id DESC`,
		userID,
	)
}

func withdrawalFields(w *model.Withdrawal) []any {
	return []any{&w.Order, &w.Sum, &w.ProcessedAt}
}
