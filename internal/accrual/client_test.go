package accrual

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

func newAccrualServer(t *testing.T, status int, header http.Header, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/orders/12345678903", r.URL.Path)

		for k, v := range header {
			w.Header()[k] = v
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestClientOrder(t *testing.T) {
	amount := model.Money(729.98)

	cases := []struct {
		name string
		body string
		want model.AccrualResult
	}{
		{
			name: "расчёт окончен с начислением",
			body: `{"order":"12345678903","status":"PROCESSED","accrual":729.98}`,
			want: model.AccrualResult{Status: model.StatusProcessed, Amount: &amount},
		},
		{
			name: "расчёт окончен без начисления",
			body: `{"order":"12345678903","status":"PROCESSED"}`,
			want: model.AccrualResult{Status: model.StatusProcessed},
		},
		{
			name: "заказ зарегистрирован",
			body: `{"order":"12345678903","status":"REGISTERED"}`,
			want: model.AccrualResult{Status: model.StatusProcessing},
		},
		{
			name: "расчёт идёт",
			body: `{"order":"12345678903","status":"PROCESSING"}`,
			want: model.AccrualResult{Status: model.StatusProcessing},
		},
		{
			name: "отказ в расчёте",
			body: `{"order":"12345678903","status":"INVALID"}`,
			want: model.AccrualResult{Status: model.StatusInvalid},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newAccrualServer(t, http.StatusOK, http.Header{"Content-Type": {"application/json"}}, c.body)

			got, err := NewClient(srv.URL).Order(t.Context(), "12345678903")
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestClientOrderBadBody(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"неизвестный статус", `{"order":"12345678903","status":"DONE"}`},
		{"нет статуса", `{"order":"12345678903"}`},
		{"не JSON", `No more than N requests per minute allowed`},
		{"начисление строкой", `{"order":"12345678903","status":"PROCESSED","accrual":"500"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newAccrualServer(t, http.StatusOK, nil, c.body)

			_, err := NewClient(srv.URL).Order(t.Context(), "12345678903")
			assert.Error(t, err)
		})
	}
}

func TestClientOrderNotRegistered(t *testing.T) {
	srv := newAccrualServer(t, http.StatusNoContent, nil, "")

	_, err := NewClient(srv.URL).Order(t.Context(), "12345678903")
	assert.ErrorIs(t, err, ErrNotRegistered)
}

func TestClientOrderTooManyRequests(t *testing.T) {
	cases := []struct {
		name   string
		header http.Header
		want   time.Duration
	}{
		{"пауза в секундах", http.Header{"Retry-After": {"60"}}, time.Minute},
		{"без заголовка", nil, defaultRetryAfter},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newAccrualServer(t, http.StatusTooManyRequests, c.header, "No more than N requests per minute allowed")

			_, err := NewClient(srv.URL).Order(t.Context(), "12345678903")

			var tooMany *TooManyRequestsError
			require.ErrorAs(t, err, &tooMany)
			assert.Equal(t, c.want, tooMany.RetryAfter)
		})
	}
}

func TestClientOrderTemporaryFailures(t *testing.T) {
	t.Run("внутренняя ошибка системы расчёта", func(t *testing.T) {
		srv := newAccrualServer(t, http.StatusInternalServerError, nil, "")

		_, err := NewClient(srv.URL).Order(t.Context(), "12345678903")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotRegistered)

		var tooMany *TooManyRequestsError
		assert.NotErrorAs(t, err, &tooMany)
	})

	t.Run("система расчёта недоступна", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()

		_, err := NewClient(srv.URL).Order(t.Context(), "12345678903")
		assert.Error(t, err)
	})

	t.Run("отменённый контекст", func(t *testing.T) {
		srv := newAccrualServer(t, http.StatusNoContent, nil, "")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := NewClient(srv.URL).Order(ctx, "12345678903")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestOrderStatus(t *testing.T) {
	cases := []struct {
		accrual string
		want    model.OrderStatus
	}{
		{"REGISTERED", model.StatusProcessing},
		{"PROCESSING", model.StatusProcessing},
		{"INVALID", model.StatusInvalid},
		{"PROCESSED", model.StatusProcessed},
	}

	for _, c := range cases {
		t.Run(c.accrual, func(t *testing.T) {
			got, err := orderStatus(c.accrual)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}

	for _, unknown := range []string{"", "NEW", "registered", "DONE"} {
		t.Run("неизвестный "+unknown, func(t *testing.T) {
			_, err := orderStatus(unknown)
			assert.Error(t, err)
		})
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"секунды", "60", time.Minute},
		{"секунды с пробелами", " 5 ", 5 * time.Second},
		{"ноль", "0", 0},
		{"дата в будущем", now.Add(2 * time.Minute).Format(http.TimeFormat), 2 * time.Minute},
		{"дата в прошлом", now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{"нет заголовка", "", defaultRetryAfter},
		{"мусор", "скоро", defaultRetryAfter},
		{"отрицательное число", "-5", defaultRetryAfter},
		{"дробное число", "1.5", defaultRetryAfter},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, retryAfter(c.header, now))
		})
	}
}
