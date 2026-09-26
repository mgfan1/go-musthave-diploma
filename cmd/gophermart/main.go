// Gophermart запускает сервис накопительной системы лояльности: HTTP API
// пользователей поверх PostgreSQL.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/config"
	"github.com/mgfan1/go-musthave-diploma/internal/handler"
	"github.com/mgfan1/go-musthave-diploma/internal/server"
	"github.com/mgfan1/go-musthave-diploma/internal/storage"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	code := 0
	if err := run(logger); err != nil {
		logger.Error("сервис остановлен с ошибкой", zap.Error(err))
		code = 1
	}

	_ = logger.Sync()
	os.Exit(code)
}

func run(logger *zap.Logger) error {
	cfg, err := config.Parse()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := sql.Open("pgx", cfg.DatabaseURI)
	if err != nil {
		return fmt.Errorf("не открыл базу: %w", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxIdleTime(4 * time.Minute)

	if _, err := storage.NewPGStorage(ctx, db, logger.With(zap.String("component", "storage"))); err != nil {
		return err
	}

	router := handler.Router(logger.With(zap.String("component", "middleware")))
	srv := server.New(cfg.Addr, router, logger.With(zap.String("component", "server")))

	return srv.Run(ctx)
}
