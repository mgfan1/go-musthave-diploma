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

// Config описывает настройки сервиса в том виде, в каком их вернул Parse.
// Parse проверяет, что заданы строка подключения к базе и секрет токенов,
// а адрес системы расчёта нормализует и проверяет. Остальные значения
// передаются как есть.
type Config struct {
	// Addr задаёт адрес, который слушает HTTP-сервер: флаг -a или
	// RUN_ADDRESS, по умолчанию localhost:8080. Parse адрес не проверяет,
	// неверный обнаружится только при запуске сервера.
	Addr string
	// DatabaseURI задаёт строку подключения к PostgreSQL: флаг -d или
	// DATABASE_URI. Обязательна, без неё Parse возвращает ошибку.
	DatabaseURI string
	// AccrualAddress задаёт базовый адрес системы расчёта начислений: флаг -r
	// или ACCRUAL_SYSTEM_ADDRESS. Parse дописывает схему http://, если её
	// нет, и убирает слеш в конце. Адрес со схемой, отличной от http и https,
	// или без хоста Parse отвергает. Пустой адрес допустим, тогда заказы
	// не опрашиваются.
	AccrualAddress string
	// JWTSecret задаёт секрет подписи токенов доступа HS256: флаг -s или
	// JWT_SECRET. Значение по умолчанию годится только для локального
	// запуска: оно лежит в исходниках, и с ним токен подделает кто угодно.
	JWTSecret string
}

// UsesDefaultSecret сообщает, что JWTSecret совпадает с секретом
// по умолчанию из исходников и токены может подделать кто угодно.
func (c Config) UsesDefaultSecret() bool {
	return c.JWTSecret == defaultJWTSecret
}

// Parse разбирает флаги из args и переменные окружения и возвращает готовую
// конфигурацию. args передаются без имени программы, обычно os.Args[1:].
// Возвращает ошибку разбора флагов (для -h это flag.ErrHelp), а также ошибку,
// если не задана строка подключения к базе или секрет токенов либо адрес
// системы расчёта не является http(s)-адресом.
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
