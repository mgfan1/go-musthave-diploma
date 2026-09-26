package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/middleware"
)

// maxRequestBody ограничивает тело любого запроса. Лимит стоит после
// распаковки gzip, поэтому считается по распакованным байтам.
const maxRequestBody = 1 << 10

// Router собирает маршруты API с журналом запросов и сжатием. Тело запроса
// после распаковки не может быть длиннее килобайта, более длинное хендлер
// отклоняет с кодом 400. Регистрация и вход открыты всем, остальные маршруты
// требуют токена доступа, который проверяет tokens. На защищённых маршрутах
// токен проверяется раньше распаковки и чтения тела, поэтому без него они
// отвечают 401 на любой запрос, даже с битым gzip.
func (h *Handler) Router(log *zap.Logger, tokens middleware.TokenParser) chi.Router {
	body := []func(http.Handler) http.Handler{
		middleware.Gzip,
		chimw.RequestSize(maxRequestBody),
	}

	r := chi.NewRouter()
	r.Use(middleware.Logging(log))

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
