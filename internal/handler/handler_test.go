package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
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

func observedRouter(users UserService, orders OrderService, balance BalanceService) (http.Handler, *observer.ObservedLogs) {
	core, logs := observer.New(zap.WarnLevel)
	router := New(users, orders, balance, zap.New(core)).Router(zap.NewNop(), auth.NewTokens(testSecret, time.Hour))
	return router, logs
}

func assertUnauthorized(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	assert.Equal(t, "Bearer", w.Header().Get("WWW-Authenticate"))
	assert.Equal(t, http.StatusText(http.StatusUnauthorized)+"\n", w.Body.String())
}

func TestInternalErrorLogsRequest(t *testing.T) {
	boom := errors.New("база недоступна")

	t.Run("защищённый маршрут", func(t *testing.T) {
		orders := newMockOrderService(t)
		orders.On("List", mock.Anything, int64(7)).Return(seqOf[model.Order](nil, boom))

		router, logs := observedRouter(newMockUserService(t), orders, newMockBalanceService(t))
		w := send(router, http.MethodGet, "/api/user/orders", "", bearer(t, 7))
		require.Equal(t, http.StatusInternalServerError, w.Code)

		entries := logs.FilterMessage("не прочитал заказы").All()
		require.Len(t, entries, 1)
		assert.Equal(t, zap.ErrorLevel, entries[0].Level)

		fields := entries[0].ContextMap()
		assert.Equal(t, http.MethodGet, fields["method"])
		assert.Equal(t, "/api/user/orders", fields["path"])
		assert.Equal(t, int64(7), fields["user_id"])
		assert.Equal(t, boom.Error(), fields["error"])
	})

	t.Run("открытый маршрут", func(t *testing.T) {
		users := newMockUserService(t)
		users.On("Register", mock.Anything, "gopher", "secret").Return("", boom)

		router, logs := observedRouter(users, newMockOrderService(t), newMockBalanceService(t))
		w := send(router, http.MethodPost, "/api/user/register", goodCredentials, "")
		require.Equal(t, http.StatusInternalServerError, w.Code)

		entries := logs.FilterMessage("не зарегистрировал пользователя").All()
		require.Len(t, entries, 1)
		assert.Equal(t, zap.ErrorLevel, entries[0].Level)

		fields := entries[0].ContextMap()
		assert.Equal(t, http.MethodPost, fields["method"])
		assert.Equal(t, "/api/user/register", fields["path"])
		assert.NotContains(t, fields, "user_id", "до входа пользователь неизвестен")
	})
}
