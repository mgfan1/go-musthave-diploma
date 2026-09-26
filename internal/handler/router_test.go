package handler

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
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

func gzipped(t *testing.T, body string) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return &buf
}

func sendGzip(router http.Handler, method, path string, body io.Reader, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Encoding", "gzip")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestRouterLimitsUnpackedBody(t *testing.T) {
	bomb := `{"login":"` + strings.Repeat("a", 256<<10) + `","password":"secret"}`

	routes := []struct {
		path   string
		method string
	}{
		{"/api/user/register", "Register"},
		{"/api/user/login", "Login"},
	}

	for _, rt := range routes {
		t.Run(rt.path+" сжатое тело в пределах лимита", func(t *testing.T) {
			users := newMockUserService(t)
			users.On(rt.method, mock.Anything, "gopher", "secret").Return("token", nil)

			w := sendGzip(newRouter(users, newMockOrderService(t), newMockBalanceService(t)), http.MethodPost, rt.path, gzipped(t, goodCredentials), "")
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "Bearer token", w.Header().Get("Authorization"))
		})

		t.Run(rt.path+" распакованное тело больше лимита", func(t *testing.T) {
			body := gzipped(t, bomb)
			require.Less(t, body.Len(), maxRequestBody, "сжатое тело должно укладываться в лимит, иначе проверяется не распаковка")

			w := sendGzip(newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t)), http.MethodPost, rt.path, body, "")
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestProtectedRoutesCheckTokenBeforeUnpacking(t *testing.T) {
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

	router := newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t))

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			w := sendGzip(router, rt.method, rt.path, strings.NewReader("это не gzip"), "")
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Equal(t, "Bearer", w.Header().Get("WWW-Authenticate"))
		})
	}
}

func TestProtectedRoutesUnpackBody(t *testing.T) {
	t.Run("сжатое тело", func(t *testing.T) {
		orders := newMockOrderService(t)
		orders.On("Upload", mock.Anything, int64(7), "12345678903").Return(true, nil)

		w := sendGzip(newRouter(newMockUserService(t), orders, newMockBalanceService(t)), http.MethodPost, "/api/user/orders", gzipped(t, "12345678903"), bearer(t, 7))
		assert.Equal(t, http.StatusAccepted, w.Code)
	})

	t.Run("битое сжатое тело", func(t *testing.T) {
		w := sendGzip(newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t)), http.MethodPost, "/api/user/orders", strings.NewReader("это не gzip"), bearer(t, 7))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("распакованное тело больше лимита", func(t *testing.T) {
		body := gzipped(t, strings.Repeat("1", 256<<10))
		require.Less(t, body.Len(), maxRequestBody)

		w := sendGzip(newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t)), http.MethodPost, "/api/user/orders", body, bearer(t, 7))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestRouterCompressesResponses(t *testing.T) {
	balance := newMockBalanceService(t)
	balance.On("Get", mock.Anything, int64(7)).Return(model.Balance{Current: 500.5, Withdrawn: 42}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	req.Header.Set("Authorization", bearer(t, 7))
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	newRouter(newMockUserService(t), newMockOrderService(t), balance).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

	zr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	got, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.JSONEq(t, `{"current":500.5,"withdrawn":42}`, string(got))
}
