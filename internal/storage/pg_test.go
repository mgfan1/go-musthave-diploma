package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN не задан, тесты с базой пропущены")
	}

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	require.NoError(t, db.PingContext(t.Context()), "база из TEST_DATABASE_DSN недоступна")

	return db
}

func newPGStorage(t *testing.T) (*PGStorage, *sql.DB) {
	t.Helper()

	db := openTestDB(t)

	store, err := NewPGStorage(t.Context(), db, zap.NewNop())
	require.NoError(t, err)

	_, err = db.ExecContext(t.Context(), "TRUNCATE TABLE users, orders, withdrawals CASCADE")
	require.NoError(t, err)

	return store, db
}

func insertUser(t *testing.T, db *sql.DB, login string) int64 {
	t.Helper()

	var id int64
	err := db.QueryRowContext(t.Context(),
		`INSERT INTO users (login, password_hash) VALUES ($1, 'hash') RETURNING id`, login,
	).Scan(&id)
	require.NoError(t, err)

	return id
}

func TestPGMigrationsAreIdempotent(t *testing.T) {
	newPGStorage(t)

	db := openTestDB(t)
	_, err := NewPGStorage(t.Context(), db, zap.NewNop())
	assert.NoError(t, err, "повторный запуск миграций не должен быть ошибкой")
}

func TestPGSchemaCascadesUserDeletion(t *testing.T) {
	ctx := t.Context()
	_, db := newPGStorage(t)

	userID := insertUser(t, db, "gopher")

	_, err := db.ExecContext(ctx, `INSERT INTO orders (number, user_id) VALUES ('12345678903', $1)`, userID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO withdrawals (user_id, order_number, amount) VALUES ($1, '2377225624', 751)`, userID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	require.NoError(t, err)

	var orders, withdrawals int
	err = db.QueryRowContext(ctx,
		`SELECT (SELECT count(*) FROM orders), (SELECT count(*) FROM withdrawals)`,
	).Scan(&orders, &withdrawals)
	require.NoError(t, err)

	assert.Zero(t, orders, "заказы удалённого пользователя должны уйти вместе с ним")
	assert.Zero(t, withdrawals, "списания удалённого пользователя должны уйти вместе с ним")
}

func TestPGSchemaRejectsBadRows(t *testing.T) {
	cases := []struct {
		name  string
		query string
		code  string
	}{
		{"неизвестный статус заказа", `INSERT INTO orders (number, user_id, status) VALUES ('1', $1, 'REGISTERED')`, pgerrcode.CheckViolation},
		{"отрицательное начисление", `INSERT INTO orders (number, user_id, accrual) VALUES ('2', $1, -1)`, pgerrcode.CheckViolation},
		{"нулевое списание", `INSERT INTO withdrawals (user_id, order_number, amount) VALUES ($1, '3', 0)`, pgerrcode.CheckViolation},
		{"заказ несуществующего пользователя", `INSERT INTO orders (number, user_id) VALUES ('4', $1 + 1)`, pgerrcode.ForeignKeyViolation},
		{"занятый логин", `INSERT INTO users (login, password_hash) SELECT login, 'hash' FROM users WHERE id = $1`, pgerrcode.UniqueViolation},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, db := newPGStorage(t)
			userID := insertUser(t, db, "gopher")

			_, err := db.ExecContext(t.Context(), c.query, userID)

			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			assert.Equal(t, c.code, pgErr.Code)
		})
	}
}

func TestPGSchemaKeepsKopecks(t *testing.T) {
	ctx := t.Context()
	_, db := newPGStorage(t)

	userID := insertUser(t, db, "gopher")

	_, err := db.ExecContext(ctx, `INSERT INTO orders (number, user_id, accrual) VALUES ('12345678903', $1, $2)`, userID, 729.98)
	require.NoError(t, err)

	var accrual float64
	var text string
	err = db.QueryRowContext(ctx, `SELECT accrual, accrual::text FROM orders WHERE number = '12345678903'`).Scan(&accrual, &text)
	require.NoError(t, err)

	assert.Equal(t, 729.98, accrual)
	assert.Equal(t, "729.98", text)
}

func TestRetriablePG(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"обрыв соединения", &pgconn.PgError{Code: pgerrcode.ConnectionException}, true},
		{"сбой при установлении соединения", &pgconn.PgError{Code: pgerrcode.SQLClientUnableToEstablishSQLConnection}, true},
		{"взаимная блокировка", &pgconn.PgError{Code: pgerrcode.DeadlockDetected}, true},
		{"сбой сериализации", &pgconn.PgError{Code: pgerrcode.SerializationFailure}, true},
		{"база ещё не принимает соединения", &pgconn.PgError{Code: pgerrcode.CannotConnectNow}, true},
		{"база выключена администратором", &pgconn.PgError{Code: pgerrcode.AdminShutdown}, true},
		{"нет свободных подключений", &pgconn.PgError{Code: pgerrcode.TooManyConnections}, true},
		{"нарушение уникальности", &pgconn.PgError{Code: pgerrcode.UniqueViolation}, false},
		{"нарушение внешнего ключа", &pgconn.PgError{Code: pgerrcode.ForeignKeyViolation}, false},
		{"запрос отменён", &pgconn.PgError{Code: pgerrcode.QueryCanceled}, false},
		{"обёрнутая ошибка сети", fmt.Errorf("не подключился: %w", &net.OpError{Err: syscall.ECONNREFUSED}), true},
		{"разорванное соединение пула", driver.ErrBadConn, true},
		{"обычная ошибка", errors.New("сбой"), false},
		{"отмена контекста", context.Canceled, false},
		{"без ошибки", nil, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, retriablePG(c.err))
		})
	}
}
