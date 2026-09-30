# Гофермарт

HTTP-сервис накопительной системы лояльности. Пользователь регистрируется и входит, загружает номера своих заказов, смотрит баланс и списывает баллы в счёт оплаты других заказов. Начисления по заказам сервис получает из внешней системы расчёта accrual.

Техническое задание: [SPECIFICATION.md](SPECIFICATION.md).

# Разработка

## Запуск сервиса

Нужен PostgreSQL. Миграции схемы применяются при старте сервиса.

Настройки задаются флагами или переменными окружения, непустая переменная окружения важнее флага:

| Переменная | Флаг | Назначение |
|---|---|---|
| `RUN_ADDRESS` | `-a` | адрес и порт HTTP-сервера, по умолчанию `localhost:8080` |
| `DATABASE_URI` | `-d` | строка подключения к PostgreSQL, обязательна |
| `ACCRUAL_SYSTEM_ADDRESS` | `-r` | адрес системы расчёта начислений; без него заказы не опрашиваются |
| `JWT_SECRET` | `-s` | секрет подписи токенов; без него берётся локальный, о чём пишется в журнал |

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

## Проверка с системой расчёта

Система расчёта начислений лежит готовым бинарём в `cmd/accrual` (на Windows его нужно скопировать с расширением `.exe`). Она читает те же `RUN_ADDRESS` и `DATABASE_URI` и может работать в той же базе:

```
RUN_ADDRESS=localhost:8081 \
DATABASE_URI="postgres://gophermart:gophermart@localhost:56432/gophermart?sslmode=disable" \
./cmd/accrual/accrual_linux_amd64
```

Чтобы заказ получил начисление, в системе расчёта регистрируют механику вознаграждения и сам заказ:

```
curl -X POST localhost:8081/api/goods -H 'Content-Type: application/json' \
  -d '{"match":"LG","reward":5,"reward_type":"%"}'
curl -X POST localhost:8081/api/orders -H 'Content-Type: application/json' \
  -d '{"order":"12345678903","goods":[{"description":"Телевизор LG","price":14599.50}]}'
```

После загрузки номера `12345678903` в Гофермарт заказ через секунду-другую переходит в `PROCESSED` с начислением 729.98.

## Тесты

```
go test ./...
```

Тесты хранилища работают с настоящей базой и без неё пропускаются. Чтобы их включить, задайте `TEST_DATABASE_DSN`. Тесты очищают таблицы, поэтому не указывайте в нём базу, с которой работает сервис. Заведите для тестов отдельную:

```
docker exec pg-gophermart createdb -U gophermart gophermart_test
TEST_DATABASE_DSN="postgres://gophermart:gophermart@localhost:56432/gophermart_test?sslmode=disable" go test ./...
```

## Покрытие

```
go test -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1
```

## Бенчмарки

```
go test -run '^$' -bench . -benchmem ./internal/luhn ./internal/auth ./internal/middleware
```

Замеряются проверка Луна, разбор токена, сжатие ответа и распаковка запроса.

## Моки

Моки интерфейсов генерирует [mockery](https://github.com/vektra/mockery) v2 по конфигу `.mockery.yaml`. Они лежат в файлах `mock_*_test.go` рядом с тестами пакета, который объявляет интерфейс, и не попадают в бинарь. После изменения интерфейса:

```
go install github.com/vektra/mockery/v2@v2.53.6
mockery
```

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
