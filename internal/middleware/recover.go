package middleware

import (
	"net/http"

	"go.uber.org/zap"
)

// Recover перехватывает панику обработчика, пишет её в журнал вместе со стеком
// и отвечает 500. Панику http.ErrAbortHandler пропускает дальше: так
// обработчик намеренно обрывает ответ.
func Recover(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				log.Error("паника при обработке запроса",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Any("panic", rec),
					zap.Stack("stack"),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
