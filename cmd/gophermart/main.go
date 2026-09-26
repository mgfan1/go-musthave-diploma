// Gophermart запускает сервис накопительной системы лояльности: HTTP API
// пользователей поверх PostgreSQL и фоновый опрос системы расчёта начислений.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/accrual"
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
	orders := service.NewOrders(store)
	balance := service.NewBalance(store)

	var wg sync.WaitGroup
	if cfg.AccrualAddress == "" {
		logger.Warn("не задан адрес системы расчёта начислений, заказы не будут опрашиваться")
	} else {
		poller := accrual.NewPoller(orders, accrual.NewClient(cfg.AccrualAddress), logger.With(zap.String("component", "accrual")))
		wg.Go(func() { poller.Run(ctx) })
	}

	api := handler.New(users, orders, balance, logger.With(zap.String("component", "handler")))
	router := api.Router(logger.With(zap.String("component", "middleware")), tokens)
	srv := server.New(cfg.Addr, router, logger.With(zap.String("component", "server")))

	err = srv.Run(ctx)
	stop()
	wg.Wait()

	return err
}
