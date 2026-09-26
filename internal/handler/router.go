package handler

import (
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/middleware"
)

// Router собирает маршруты API с журналом запросов и сжатием. Регистрация
// и вход открыты всем, остальные маршруты требуют токена доступа, который
// проверяет tokens. Токен проверяется до чтения тела, поэтому без него
// защищённый маршрут отвечает 401 на любой запрос.
func (h *Handler) Router(log *zap.Logger, tokens middleware.TokenParser) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logging(log))
	r.Use(middleware.Gzip)

	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", h.register)
		r.Post("/login", h.login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(tokens))

			r.Post("/orders", h.uploadOrder)
			r.Get("/orders", h.listOrders)
			r.Get("/balance", h.getBalance)
			r.Post("/balance/withdraw", h.withdraw)
			r.Get("/withdrawals", h.listWithdrawals)
		})
	})

	return r
}
