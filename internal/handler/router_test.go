package handler

import (
	"bytes"
	"compress/gzip"
	"context"
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
	"go.uber.org/zap/zaptest/observer"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

const testSecret = "секрет для тестов"

func newRouter(users UserService, orders OrderService, balance BalanceService) http.Handler {
	return New(users, orders, balance, zap.NewNop()).Router(zap.NewNop(), auth.NewTokens(testSecret, time.Hour))
}

func loggedRouter(users UserService, orders OrderService, balance BalanceService, timeout time.Duration) (http.Handler, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)
	router := New(users, orders, balance, zap.NewNop()).router(zap.New(core), auth.NewTokens(testSecret, time.Hour), timeout)
	return router, logs
}

func loggedStatus(t *testing.T, logs *observer.ObservedLogs) int64 {
	t.Helper()

	entries := logs.FilterMessage("обработан запрос").All()
	require.Len(t, entries, 1)

	status, ok := entries[0].ContextMap()["status"].(int64)
	require.True(t, ok)

	return status
}

const panicMessage = "паника при обработке запроса"

func panicStack(t *testing.T, logs *observer.ObservedLogs) string {
	t.Helper()

	entries := logs.FilterMessage(panicMessage).All()
	require.Len(t, entries, 1)

	stack, ok := entries[0].ContextMap()["stack"].(string)
	require.True(t, ok)

	return stack
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
		{http.MethodHead, "/api/user/orders"},
		{http.MethodGet, "/api/user/balance"},
		{http.MethodHead, "/api/user/balance"},
		{http.MethodPost, "/api/user/balance/withdraw"},
		{http.MethodGet, "/api/user/withdrawals"},
		{http.MethodHead, "/api/user/withdrawals"},
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
	cases := []string{
		"/api/user/unknown",
		"/api/user/orders/",
		"/api/user",
	}

	router := newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t))

	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			w := send(router, http.MethodGet, c, "", "")
			assert.Equal(t, http.StatusNotFound, w.Code)
		})
	}
}

func TestRouterMethodNotAllowed(t *testing.T) {
	cases := []struct {
		method string
		path   string
		allow  string
	}{
		{http.MethodGet, "/api/user/register", "POST"},
		{http.MethodGet, "/api/user/login", "POST"},
		{http.MethodDelete, "/api/user/orders", "GET, HEAD, POST"},
		{http.MethodPost, "/api/user/balance", "GET, HEAD"},
		{http.MethodGet, "/api/user/balance/withdraw", "POST"},
		{http.MethodPost, "/api/user/withdrawals", "GET, HEAD"},
	}

	router := newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t))

	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			w := send(router, c.method, c.path, "", "")
			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
			assert.Equal(t, c.allow, w.Header().Get("Allow"))
		})
	}
}

func TestRouterRecoversPanic(t *testing.T) {
	users := newMockUserService(t)
	users.On("Register", mock.Anything, "gopher", "secret").Run(func(mock.Arguments) { panic("boom") })

	router, logs := loggedRouter(users, newMockOrderService(t), newMockBalanceService(t), handlerTimeout)

	w := send(router, http.MethodPost, "/api/user/register", goodCredentials, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", w.Body.String())
	assert.Equal(t, int64(http.StatusInternalServerError), loggedStatus(t, logs))
	assert.Contains(t, panicStack(t, logs), "(*mockUserService).Register")
}

func TestRouterRecoversPanicAfterTimeout(t *testing.T) {
	balance := newMockBalanceService(t)
	balance.On("Get", mock.Anything, int64(7)).
		Run(func(args mock.Arguments) {
			<-args.Get(0).(context.Context).Done()
			panic("boom")
		})

	router, logs := loggedRouter(newMockUserService(t), newMockOrderService(t), balance, 50*time.Millisecond)

	w := send(router, http.MethodGet, "/api/user/balance", "", bearer(t, 7))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, int64(http.StatusServiceUnavailable), loggedStatus(t, logs))

	require.Eventually(t, func() bool {
		return logs.FilterMessage(panicMessage).Len() == 1
	}, time.Second, time.Millisecond, "паника после истечения срока не попала в журнал")
	assert.Contains(t, panicStack(t, logs), "(*mockBalanceService).Get")
}

func TestRouterLimitsHandlerTime(t *testing.T) {
	var (
		deadline time.Time
		ok       bool
	)
	balance := newMockBalanceService(t)
	balance.On("Get", mock.Anything, int64(7)).
		Run(func(args mock.Arguments) { deadline, ok = args.Get(0).(context.Context).Deadline() }).
		Return(model.Balance{}, nil)

	send(newRouter(newMockUserService(t), newMockOrderService(t), balance), http.MethodGet, "/api/user/balance", "", bearer(t, 7))

	require.True(t, ok, "у контекста обработчика нет дедлайна")
	assert.WithinDuration(t, time.Now().Add(handlerTimeout), deadline, time.Second)
}

func TestRouterLogsTimeout(t *testing.T) {
	finished := make(chan struct{})
	balance := newMockBalanceService(t)
	balance.On("Get", mock.Anything, int64(7)).
		Run(func(args mock.Arguments) {
			<-args.Get(0).(context.Context).Done()
			close(finished)
		}).
		Return(model.Balance{}, context.DeadlineExceeded)

	router, logs := loggedRouter(newMockUserService(t), newMockOrderService(t), balance, 50*time.Millisecond)

	w := send(router, http.MethodGet, "/api/user/balance", "", bearer(t, 7))
	<-finished

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, http.StatusText(http.StatusServiceUnavailable), w.Body.String())
	assert.Equal(t, int64(http.StatusServiceUnavailable), loggedStatus(t, logs))
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
