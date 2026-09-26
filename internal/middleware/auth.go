package middleware

import (
	"net/http"
	"strings"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
)

// TokenParser проверяет токен доступа и возвращает идентификатор пользователя.
type TokenParser interface {
	Parse(token string) (int64, error)
}

// Auth пропускает дальше только запросы с действительным токеном в заголовке
// Authorization: Bearer и кладёт идентификатор пользователя в контекст
// запроса, откуда его достаёт auth.UserID. Остальным запросам отвечает 401,
// не читая тело.
func Auth(tokens TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
				unauthorized(w)
				return
			}

			userID, err := tokens.Parse(token)
			if err != nil {
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.WithUserID(r.Context(), userID)))
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}
