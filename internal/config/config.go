// Package config собирает настройки сервиса из флагов командной строки
// и переменных окружения. Непустая переменная окружения перекрывает флаг.
package config

import (
	"errors"
	"flag"
	"strings"
)

const defaultJWTSecret = "gophermart-local-secret"

// Config хранит настройки сервиса.
type Config struct {
	// Addr задаёт адрес и порт HTTP-сервера: флаг -a или RUN_ADDRESS.
	Addr string
	// DatabaseURI задаёт строку подключения к PostgreSQL: флаг -d или DATABASE_URI.
	DatabaseURI string
	// AccrualAddress задаёт базовый адрес системы расчёта начислений: флаг -r
	// или ACCRUAL_SYSTEM_ADDRESS. Хранится со схемой и без слеша в конце.
	AccrualAddress string
	// JWTSecret задаёт секрет подписи токенов доступа: флаг -s или JWT_SECRET.
	JWTSecret string
}

// Parse читает флаги и переменные окружения и возвращает готовую конфигурацию.
// Возвращает ошибку, если не задана строка подключения к базе или секрет токенов.
func Parse() (Config, error) {
	var cfg Config

	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "адрес и порт запуска сервиса")
	flag.StringVar(&cfg.DatabaseURI, "d", "", "строка подключения к базе данных")
	flag.StringVar(&cfg.AccrualAddress, "r", "", "адрес системы расчёта начислений")
	flag.StringVar(&cfg.JWTSecret, "s", defaultJWTSecret, "секрет подписи токенов доступа")
	flag.Parse()

	envString("RUN_ADDRESS", &cfg.Addr)
	envString("DATABASE_URI", &cfg.DatabaseURI)
	envString("ACCRUAL_SYSTEM_ADDRESS", &cfg.AccrualAddress)
	envString("JWT_SECRET", &cfg.JWTSecret)

	if cfg.DatabaseURI == "" {
		return cfg, errors.New("не задана строка подключения к базе: флаг -d или DATABASE_URI")
	}
	if cfg.JWTSecret == "" {
		return cfg, errors.New("не задан секрет токенов: флаг -s или JWT_SECRET")
	}

	cfg.AccrualAddress = normalizeURL(cfg.AccrualAddress)

	return cfg, nil
}

func normalizeURL(addr string) string {
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	if addr == "" {
		return ""
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return addr
}
