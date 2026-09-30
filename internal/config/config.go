// Package config собирает настройки сервиса из флагов командной строки
// и переменных окружения. Непустая переменная окружения перекрывает флаг.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"
)

const defaultJWTSecret = "gophermart-local-secret"

// Config хранит настройки сервиса, которые вернул Parse.
type Config struct {
	// Addr - адрес HTTP-сервера: флаг -a или RUN_ADDRESS,
	// по умолчанию localhost:8080.
	Addr string
	// DatabaseURI - строка подключения к PostgreSQL: флаг -d или
	// DATABASE_URI. Обязательна.
	DatabaseURI string
	// AccrualAddress - адрес системы расчёта начислений: флаг -r или
	// ACCRUAL_SYSTEM_ADDRESS. Схема http:// дописывается, если её нет,
	// слеш в конце убирается. Пустой адрес выключает опрос заказов.
	AccrualAddress string
	// JWTSecret - секрет подписи токенов доступа: флаг -s или JWT_SECRET.
	// Значение по умолчанию лежит в исходниках и годится только для
	// локального запуска.
	JWTSecret string
}

// UsesDefaultSecret сообщает, что токены подписываются секретом
// по умолчанию.
func (c Config) UsesDefaultSecret() bool {
	return c.JWTSecret == defaultJWTSecret
}

// Parse читает настройки из флагов args (без имени программы) и переменных
// окружения. Для -h возвращает flag.ErrHelp. Возвращает ошибку, если не заданы
// строка подключения к базе или секрет токенов либо адрес системы расчёта
// не http(s).
func Parse(args []string) (Config, error) {
	var cfg Config

	fs := flag.NewFlagSet("gophermart", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "a", "localhost:8080", "адрес и порт запуска сервиса")
	fs.StringVar(&cfg.DatabaseURI, "d", "", "строка подключения к базе данных")
	fs.StringVar(&cfg.AccrualAddress, "r", "", "адрес системы расчёта начислений")
	fs.StringVar(&cfg.JWTSecret, "s", defaultJWTSecret, "секрет подписи токенов доступа")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}

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

	addr, err := normalizeURL(cfg.AccrualAddress)
	if err != nil {
		return cfg, err
	}
	cfg.AccrualAddress = addr

	return cfg, nil
}

func normalizeURL(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", nil
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	addr = strings.TrimRight(addr, "/")

	u, err := url.Parse(addr)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("неверный адрес системы расчёта %q: флаг -r или ACCRUAL_SYSTEM_ADDRESS", addr)
	}

	return addr, nil
}
