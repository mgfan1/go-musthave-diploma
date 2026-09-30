package handler

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/middleware"
)

const (
	// maxRequestBody ограничивает тело любого запроса. Лимит стоит после
	// распаковки gzip, поэтому считается по распакованным байтам.
	maxRequestBody = 1 << 10
	handlerTimeout = 10 * time.Second
)

// Router собирает маршруты API. Регистрация и вход открыты всем, остальные
// маршруты требуют токена доступа, который проверяет tokens, и без него
// отвечают 401, не читая тело. Паника в хендлере даёт 500, обработка дольше
// десяти секунд даёт 503. Тело запроса читается не больше чем на килобайт
// после распаковки.
func (h *Handler) Router(log *zap.Logger, tokens middleware.TokenParser) http.Handler {
	return h.router(log, tokens, handlerTimeout)
}

func (h *Handler) router(log *zap.Logger, tokens middleware.TokenParser, timeout time.Duration) http.Handler {
	public := func(next http.HandlerFunc) http.Handler {
		return middleware.Gzip(http.MaxBytesHandler(next, maxRequestBody))
	}
	protected := func(next http.HandlerFunc) http.Handler {
		return middleware.Auth(tokens)(public(next))
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/user/register", public(h.register))
	mux.Handle("POST /api/user/login", public(h.login))
	mux.Handle("POST /api/user/orders", protected(h.uploadOrder))
	mux.Handle("GET /api/user/orders", protected(h.listOrders))
	mux.Handle("GET /api/user/balance", protected(h.getBalance))
	mux.Handle("POST /api/user/balance/withdraw", protected(h.withdraw))
	mux.Handle("GET /api/user/withdrawals", protected(h.listWithdrawals))

	timed := http.TimeoutHandler(middleware.Recover(log)(mux), timeout, http.StatusText(http.StatusServiceUnavailable))
	return middleware.Logging(log)(timed)
}
