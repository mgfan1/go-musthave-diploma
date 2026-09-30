package accrual

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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

func TestClientReusesConnections(t *testing.T) {
	const rounds = 5

	var (
		mu       sync.Mutex
		arrived  int
		release  = make(chan struct{})
		newConns atomic.Int32
	)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		arrived++
		wait := release
		if arrived == workers {
			close(release)
			release = make(chan struct{})
			arrived = 0
		}
		mu.Unlock()

		select {
		case <-wait:
		case <-time.After(time.Second):
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL)
	for range rounds {
		var wg sync.WaitGroup
		for range workers {
			wg.Go(func() {
				_, err := client.Order(t.Context(), "12345678903")
				assert.ErrorIs(t, err, ErrNotRegistered)
			})
		}
		wg.Wait()
	}

	assert.Less(t, int(newConns.Load()), 2*workers,
		"воркеры опроса должны переиспользовать соединения между проходами")
}

func TestRetryAfter(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"секунды", "60", time.Minute},
		{"секунды с пробелами", " 5 ", 5 * time.Second},
		{"ноль", "0", 0},
		{"ровно предел", "600", maxRetryAfter},
		{"секунд больше предела", "3600", maxRetryAfter},
		{"огромное число секунд", "10000000000", maxRetryAfter},
		{"нет заголовка", "", defaultRetryAfter},
		{"дата вместо секунд", "Wed, 21 Oct 2026 07:28:00 GMT", defaultRetryAfter},
		{"мусор", "скоро", defaultRetryAfter},
		{"отрицательное число", "-5", defaultRetryAfter},
		{"дробное число", "1.5", defaultRetryAfter},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, retryAfter(c.header))
		})
	}
}
