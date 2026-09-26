package handler

import (
	"net/http"

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

			r.Post("/orders", notImplemented)
			r.Get("/orders", notImplemented)
			r.Get("/balance", notImplemented)
			r.Post("/balance/withdraw", notImplemented)
			r.Get("/withdrawals", notImplemented)
		})
	})

	return r
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}
