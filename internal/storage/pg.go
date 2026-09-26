package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/retry"
	"github.com/mgfan1/go-musthave-diploma/migrations"
)

// PGStorage работает с PostgreSQL через database/sql.
type PGStorage struct {
	db *sql.DB
}

// NewPGStorage применяет миграции схемы и возвращает готовое хранилище.
// Пока база недоступна, попытки повторяются с паузами.
func NewPGStorage(ctx context.Context, db *sql.DB, log *zap.Logger) (*PGStorage, error) {
	err := retry.New(log).Do(ctx, retriablePG, func() error {
		return applyMigrations(ctx, db, migrations.FS, ".")
	})
	if err != nil {
		return nil, err
	}
	log.Info("схема базы готова")

	return &PGStorage{db: db}, nil
}

func retriablePG(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return retriableCode(pgErr.Code)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, driver.ErrBadConn)
}

func retriableCode(code string) bool {
	if pgerrcode.IsConnectionException(code) || pgerrcode.IsTransactionRollback(code) {
		return true
	}

	switch code {
	case pgerrcode.CannotConnectNow, pgerrcode.AdminShutdown, pgerrcode.CrashShutdown, pgerrcode.TooManyConnections:
		return true
	default:
		return false
	}
}

func applyMigrations(ctx context.Context, db *sql.DB, src fs.FS, path string) error {
	source, err := iofs.New(src, path)
	if err != nil {
		return fmt.Errorf("не прочитал миграции: %w", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("не получил соединение для миграций: %w", err)
	}
	defer conn.Close()

	drv, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("не подготовил драйвер миграций: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", drv)
	if err != nil {
		return fmt.Errorf("не создал мигратор: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("не применил миграции: %w", err)
	}

	return nil
}
