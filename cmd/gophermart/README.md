# cmd/gophermart

Точка входа сервиса: читает настройки, подключается к PostgreSQL и применяет миграции, запускает HTTP-сервер и опрос системы расчёта начислений.

Собрать и запустить: `go build -o cmd/gophermart/gophermart ./cmd/gophermart` или `go run ./cmd/gophermart`. Настройки и пример запуска — в [корневом README](../../README.md#запуск-сервиса).
