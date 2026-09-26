package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
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
func (h *Handler) Router(log *zap.Logger, tokens middleware.TokenParser) chi.Router {
	return h.router(log, tokens, handlerTimeout)
}

func (h *Handler) router(log *zap.Logger, tokens middleware.TokenParser, timeout time.Duration) chi.Router {
	body := []func(http.Handler) http.Handler{
		middleware.Gzip,
		chimw.RequestSize(maxRequestBody),
	}

	r := chi.NewRouter()
	r.Use(middleware.Logging(log))
	r.Use(func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, timeout, http.StatusText(http.StatusServiceUnavailable))
	})
	r.Use(middleware.Recover(log))

	r.Route("/api/user", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(body...)

			r.Post("/register", h.register)
			r.Post("/login", h.login)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(tokens))
			r.Use(body...)

			r.Post("/orders", h.uploadOrder)
			r.Get("/orders", h.listOrders)
			r.Get("/balance", h.getBalance)
			r.Post("/balance/withdraw", h.withdraw)
			r.Get("/withdrawals", h.listWithdrawals)
		})
	})

	return r
}
