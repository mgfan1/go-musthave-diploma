package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

func balanceRouter(t *testing.T, balance BalanceService) http.Handler {
	t.Helper()
	return newRouter(newMockUserService(t), newMockOrderService(t), balance)
}

func TestGetBalance(t *testing.T) {
	cases := []struct {
		name    string
		balance model.Balance
		want    string
	}{
		{"пример из ТЗ", model.Balance{Current: 500.5, Withdrawn: 42}, `{"current":500.5,"withdrawn":42}`},
		{"пустой счёт", model.Balance{}, `{"current":0,"withdrawn":0}`},
		{"начисление с копейками", model.Balance{Current: 729.98}, `{"current":729.98,"withdrawn":0}`},
		{"крупные суммы без экспоненты", model.Balance{Current: 1234567890.12, Withdrawn: 100000000}, `{"current":1234567890.12,"withdrawn":100000000}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			balance := newMockBalanceService(t)
			balance.On("Get", mock.Anything, int64(7)).Return(c.balance, nil)

			w := send(balanceRouter(t, balance), http.MethodGet, "/api/user/balance", "", bearer(t, 7))

			require.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
			assert.Equal(t, c.want, w.Body.String())
		})
	}
}

func TestGetBalanceFailure(t *testing.T) {
	balance := newMockBalanceService(t)
	balance.On("Get", mock.Anything, int64(7)).Return(model.Balance{}, errors.New("база недоступна"))

	w := send(balanceRouter(t, balance), http.MethodGet, "/api/user/balance", "", bearer(t, 7))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", w.Body.String())
}

func TestWithdraw(t *testing.T) {
	boom := errors.New("база недоступна")

	cases := []struct {
		name     string
		body     string
		sum      model.Money
		err      error
		wantCode int
	}{
		{name: "списано", body: `{"order":"2377225624","sum":751}`, sum: 751, wantCode: http.StatusOK},
		{name: "сумма с копейками", body: `{"order":"2377225624","sum":123.45}`, sum: 123.45, wantCode: http.StatusOK},
		{name: "недостаточно средств", body: `{"order":"2377225624","sum":751}`, sum: 751, err: model.ErrInsufficientFunds, wantCode: http.StatusPaymentRequired},
		{name: "номер не проходит проверку Луна", body: `{"order":"2377225624","sum":751}`, sum: 751, err: model.ErrInvalidOrderNumber, wantCode: http.StatusUnprocessableEntity},
		{name: "отрицательная сумма", body: `{"order":"2377225624","sum":-751}`, sum: -751, err: model.ErrInvalidWithdrawSum, wantCode: http.StatusBadRequest},
		{name: "нет суммы", body: `{"order":"2377225624"}`, sum: 0, err: model.ErrInvalidWithdrawSum, wantCode: http.StatusBadRequest},
		{name: "пользователя из токена нет", body: `{"order":"2377225624","sum":751}`, sum: 751, err: model.ErrUserNotFound, wantCode: http.StatusUnauthorized},
		{name: "сбой сервиса", body: `{"order":"2377225624","sum":751}`, sum: 751, err: boom, wantCode: http.StatusInternalServerError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			balance := newMockBalanceService(t)
			balance.On("Withdraw", mock.Anything, int64(7), "2377225624", c.sum).Return(c.err)

			w := send(balanceRouter(t, balance), http.MethodPost, "/api/user/balance/withdraw", c.body, bearer(t, 7))

			assert.Equal(t, c.wantCode, w.Code)
			switch c.wantCode {
			case http.StatusInternalServerError:
				assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", w.Body.String())
			case http.StatusUnauthorized:
				assertUnauthorized(t, w)
			}
		})
	}
}

func TestWithdrawBadRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"пустое тело", ""},
		{"битый JSON", `{"order":"2377225624",`},
		{"не объект", `["2377225624",751]`},
		{"сумма строкой", `{"order":"2377225624","sum":"751"}`},
		{"номер числом", `{"order":2377225624,"sum":751}`},
		{"нет номера", `{"sum":751}`},
		{"пустой номер", `{"order":"","sum":751}`},
		{"слишком длинное тело", `{"order":"2377225624","sum":751,"comment":"` + strings.Repeat("a", maxRequestBody) + `"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := send(balanceRouter(t, newMockBalanceService(t)), http.MethodPost, "/api/user/balance/withdraw", c.body, bearer(t, 7))
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestListWithdrawals(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)

	balance := newMockBalanceService(t)
	balance.On("Withdrawals", mock.Anything, int64(7)).Return(seqOf([]model.Withdrawal{
		{Order: "2377225624", Sum: 500, ProcessedAt: time.Date(2020, 12, 9, 16, 9, 57, 0, msk)},
		{Order: "12345678903", Sum: 729.98, ProcessedAt: time.Date(2020, 12, 9, 16, 5, 1, 500, msk)},
		{Order: "79927398713", Sum: 0.01, ProcessedAt: time.Date(2020, 12, 8, 10, 0, 0, 0, time.UTC)},
	}, nil))

	w := send(balanceRouter(t, balance), http.MethodGet, "/api/user/withdrawals", "", bearer(t, 7))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.Equal(t,
		`[{"order":"2377225624","sum":500,"processed_at":"2020-12-09T16:09:57+03:00"},`+
			`{"order":"12345678903","sum":729.98,"processed_at":"2020-12-09T16:05:01+03:00"},`+
			`{"order":"79927398713","sum":0.01,"processed_at":"2020-12-08T10:00:00Z"}]`,
		w.Body.String())
}

func TestListWithdrawalsEmpty(t *testing.T) {
	balance := newMockBalanceService(t)
	balance.On("Withdrawals", mock.Anything, int64(7)).Return(seqOf[model.Withdrawal](nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/user/withdrawals", nil)
	req.Header.Set("Authorization", bearer(t, 7))
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	balanceRouter(t, balance).ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json", "автотест разбирает тело только с JSON в Content-Type")
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Empty(t, w.Body.String())
}

func TestListWithdrawalsFailure(t *testing.T) {
	cases := []struct {
		name        string
		withdrawals []model.Withdrawal
	}{
		{"сбой до первой строки", nil},
		{"сбой посреди чтения", []model.Withdrawal{{Order: "2377225624", Sum: 500, ProcessedAt: time.Now()}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			balance := newMockBalanceService(t)
			balance.On("Withdrawals", mock.Anything, int64(7)).Return(seqOf(c.withdrawals, errors.New("база недоступна")))

			w := send(balanceRouter(t, balance), http.MethodGet, "/api/user/withdrawals", "", bearer(t, 7))

			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", w.Body.String(), "прочитанная часть списка не уходит клиенту")
		})
	}
}
