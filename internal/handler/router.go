// Package handler реализует HTTP API системы лояльности Гофермарт.
package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/middleware"
)

// Router собирает маршруты API с журналом запросов и сжатием.
// Пока все маршруты отвечают 501.
func Router(log *zap.Logger) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logging(log))
	r.Use(middleware.Gzip)

	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", notImplemented)
		r.Post("/login", notImplemented)
		r.Post("/orders", notImplemented)
		r.Get("/orders", notImplemented)
		r.Get("/balance", notImplemented)
		r.Post("/balance/withdraw", notImplemented)
		r.Get("/withdrawals", notImplemented)
	})

	return r
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}
