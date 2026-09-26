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
)

const testSecret = "секрет для тестов"

func newRouter(users UserService, orders OrderService, balance BalanceService) http.Handler {
	return New(users, orders, balance, zap.NewNop()).Router(zap.NewNop(), auth.NewTokens(testSecret, time.Hour))
}

func bearer(t *testing.T, userID int64) string {
	t.Helper()

	token, err := auth.NewTokens(testSecret, time.Hour).Issue(userID)
	require.NoError(t, err)

	return "Bearer " + token
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

	foreign, err := auth.NewTokens("чужой секрет", time.Hour).Issue(7)
	require.NoError(t, err)

	headers := []struct {
		name   string
		header string
	}{
		{"без токена", ""},
		{"чужая подпись", "Bearer " + foreign},
	}

	router := newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t))

	for _, rt := range routes {
		for _, h := range headers {
			t.Run(rt.method+" "+rt.path+" "+h.name, func(t *testing.T) {
				w := send(router, rt.method, rt.path, "12345678902", h.header)
				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})
		}
	}
}

func TestRouterUnknownPath(t *testing.T) {
	w := send(newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t)), http.MethodGet, "/api/user/unknown", "", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}
