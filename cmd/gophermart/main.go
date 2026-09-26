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

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
	"github.com/mgfan1/go-musthave-diploma/internal/config"
	"github.com/mgfan1/go-musthave-diploma/internal/handler"
	"github.com/mgfan1/go-musthave-diploma/internal/server"
	"github.com/mgfan1/go-musthave-diploma/internal/service"
	"github.com/mgfan1/go-musthave-diploma/internal/storage"
)

const tokenTTL = 24 * time.Hour

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

	store, err := storage.NewPGStorage(ctx, db, logger.With(zap.String("component", "storage")))
	if err != nil {
		return err
	}

	tokens := auth.NewTokens(cfg.JWTSecret, tokenTTL)
	users := service.NewUsers(store, tokens)

	api := handler.New(users, logger.With(zap.String("component", "handler")))
	router := api.Router(logger.With(zap.String("component", "middleware")), tokens)
	srv := server.New(cfg.Addr, router, logger.With(zap.String("component", "server")))

	return srv.Run(ctx)
}
