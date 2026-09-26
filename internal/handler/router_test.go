package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
	"github.com/mgfan1/go-musthave-diploma/internal/handler/mocks"
)

const testSecret = "секрет для тестов"

func newRouter(users UserService) http.Handler {
	return New(users, zap.NewNop()).Router(zap.NewNop(), auth.NewTokens(testSecret, time.Hour))
}

func send(router http.Handler, method, path, body, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestProtectedRoutesRequireToken(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/user/orders"},
		{http.MethodGet, "/api/user/orders"},
		{http.MethodGet, "/api/user/balance"},
		{http.MethodPost, "/api/user/balance/withdraw"},
		{http.MethodGet, "/api/user/withdrawals"},
	}

	valid, err := auth.NewTokens(testSecret, time.Hour).Issue(7)
	require.NoError(t, err)
	foreign, err := auth.NewTokens("чужой секрет", time.Hour).Issue(7)
	require.NoError(t, err)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"без токена", "", http.StatusUnauthorized},
		{"чужая подпись", "Bearer " + foreign, http.StatusUnauthorized},
		{"действительный токен", "Bearer " + valid, http.StatusNotImplemented},
	}

	router := newRouter(mocks.NewUserService(t))

	for _, rt := range routes {
		for _, c := range cases {
			t.Run(rt.method+" "+rt.path+" "+c.name, func(t *testing.T) {
				w := send(router, rt.method, rt.path, "12345678903", c.header)
				assert.Equal(t, c.want, w.Code)
			})
		}
	}
}

func TestRouterUnknownPath(t *testing.T) {
	w := send(newRouter(mocks.NewUserService(t)), http.MethodGet, "/api/user/unknown", "", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}
