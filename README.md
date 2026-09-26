# go-musthave-diploma-tpl

Шаблон репозитория для индивидуального дипломного проекта курса «Go-разработчик»

# Начало работы

1. Склонируйте репозиторий в любую подходящую директорию на вашем компьютере.
2. В корне репозитория выполните команду `go mod init <name>` (где `<name>` — адрес вашего репозитория на GitHub без
   префикса `https://`) для создания модуля

# Обновление шаблона

Чтобы иметь возможность получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m master template https://github.com/yandex-praktikum/go-musthave-diploma-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/master .github
```

Затем добавьте полученные изменения в свой репозиторий.

# Разработка

## Запуск сервиса

Нужен PostgreSQL. Миграции схемы применяются при старте сервиса.

Настройки задаются переменными окружения или флагами, переменные окружения важнее флагов:

| Переменная               | Флаг | Назначение                                             |
|--------------------------|------|--------------------------------------------------------|
| `RUN_ADDRESS`            | `-a` | адрес и порт HTTP-сервера                              |
| `DATABASE_URI`           | `-d` | строка подключения к PostgreSQL                        |
| `ACCRUAL_SYSTEM_ADDRESS` | `-r` | адрес системы расчёта начислений                       |
| `JWT_SECRET`             | `-s` | ключ подписи токенов доступа (есть локальный по умолчанию) |

Пример:

```
docker run -d --name pg-gophermart -p 56432:5432 \
  -e POSTGRES_USER=gophermart -e POSTGRES_PASSWORD=gophermart -e POSTGRES_DB=gophermart postgres

RUN_ADDRESS=localhost:8080 \
DATABASE_URI="postgres://gophermart:gophermart@localhost:56432/gophermart?sslmode=disable" \
ACCRUAL_SYSTEM_ADDRESS=http://localhost:8081 \
JWT_SECRET=secret \
go run ./cmd/gophermart
```

## Тесты

```
go test ./...
```

Тесты хранилища работают с настоящей базой и без неё пропускаются. Чтобы их включить, задайте `TEST_DATABASE_DSN`. Тесты очищают таблицы, поэтому базу для них лучше держать отдельной:

```
TEST_DATABASE_DSN="postgres://gophermart:gophermart@localhost:56432/gophermart?sslmode=disable" go test ./...
```

## Покрытие

```
go test -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1
```

## Моки

Моки интерфейсов генерирует [mockery](https://github.com/vektra/mockery) v2 по конфигу `.mockery.yaml`. Они лежат в файлах `mock_*_test.go` рядом с тестами пакета, который объявляет интерфейс, и не попадают в бинарь. После изменения интерфейса:

```
go install github.com/vektra/mockery/v2@v2.53.6
mockery
```
