package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
)

func TestAuth(t *testing.T) {
	tokens := auth.NewTokens("секрет для тестов", time.Hour)

	valid, err := tokens.Issue(42)
	require.NoError(t, err)
	foreign, err := auth.NewTokens("чужой секрет", time.Hour).Issue(42)
	require.NoError(t, err)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"нет заголовка", "", http.StatusUnauthorized},
		{"другая схема", "Basic " + valid, http.StatusUnauthorized},
		{"токен без схемы", valid, http.StatusUnauthorized},
		{"пустой токен", "Bearer ", http.StatusUnauthorized},
		{"мусор вместо токена", "Bearer не-токен", http.StatusUnauthorized},
		{"чужая подпись", "Bearer " + foreign, http.StatusUnauthorized},
		{"действительный токен", "Bearer " + valid, http.StatusOK},
		{"схема в нижнем регистре", "bearer " + valid, http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			var userID int64
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				userID, _ = auth.UserID(r.Context())
			})

			req := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			w := httptest.NewRecorder()

			Auth(tokens)(next).ServeHTTP(w, req)

			assert.Equal(t, c.want, w.Code)
			if c.want == http.StatusOK {
				assert.Equal(t, int64(42), userID)
				return
			}
			assert.False(t, called, "без действительного токена хендлер вызываться не должен")
			assert.Equal(t, "Bearer", w.Header().Get("WWW-Authenticate"))
		})
	}
}
