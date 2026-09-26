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

func TestUploadOrder(t *testing.T) {
	boom := errors.New("база недоступна")

	cases := []struct {
		name     string
		body     string
		number   string
		created  bool
		err      error
		wantCode int
	}{
		{name: "новый заказ", body: "12345678903", number: "12345678903", created: true, wantCode: http.StatusAccepted},
		{name: "перевод строки после номера", body: "12345678903\n", number: "12345678903", created: true, wantCode: http.StatusAccepted},
		{name: "повторная загрузка тем же пользователем", body: "12345678903", number: "12345678903", wantCode: http.StatusOK},
		{name: "номер загружен другим пользователем", body: "12345678903", number: "12345678903", err: model.ErrOrderOwnedByOther, wantCode: http.StatusConflict},
		{name: "номер не проходит проверку Луна", body: "12345678902", number: "12345678902", err: model.ErrInvalidOrderNumber, wantCode: http.StatusUnprocessableEntity},
		{name: "буквы вместо цифр", body: "abc", number: "abc", err: model.ErrInvalidOrderNumber, wantCode: http.StatusUnprocessableEntity},
		{name: "пользователя из токена нет", body: "12345678903", number: "12345678903", err: model.ErrUserNotFound, wantCode: http.StatusUnauthorized},
		{name: "сбой сервиса", body: "12345678903", number: "12345678903", err: boom, wantCode: http.StatusInternalServerError},
		{name: "пустое тело", body: "", wantCode: http.StatusBadRequest},
		{name: "одни пробелы", body: " \r\n", wantCode: http.StatusBadRequest},
		{name: "слишком длинное тело", body: strings.Repeat("1", maxRequestBody+1), wantCode: http.StatusBadRequest},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			orders := newMockOrderService(t)
			if c.number != "" {
				orders.On("Upload", mock.Anything, int64(7), c.number).Return(c.created, c.err)
			}

			w := send(newRouter(newMockUserService(t), orders, newMockBalanceService(t)), http.MethodPost, "/api/user/orders", c.body, bearer(t, 7))

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

func TestListOrders(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)
	accrual := model.Money(729.98)
	zero := model.Money(0)

	orders := newMockOrderService(t)
	orders.On("List", mock.Anything, int64(7)).Return([]model.Order{
		{Number: "9278923470", Status: model.StatusProcessed, Accrual: &accrual, UploadedAt: time.Date(2020, 12, 10, 15, 15, 45, 0, msk)},
		{Number: "18", Status: model.StatusProcessed, Accrual: &zero, UploadedAt: time.Date(2020, 12, 10, 15, 14, 0, 0, msk)},
		{Number: "12345678903", Status: model.StatusProcessing, UploadedAt: time.Date(2020, 12, 10, 15, 12, 1, 500, msk)},
		{Number: "346436439", Status: model.StatusInvalid, UploadedAt: time.Date(2020, 12, 9, 16, 9, 53, 0, msk)},
	}, nil)

	w := send(newRouter(newMockUserService(t), orders, newMockBalanceService(t)), http.MethodGet, "/api/user/orders", "", bearer(t, 7))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.JSONEq(t, `[
		{"number": "9278923470", "status": "PROCESSED", "accrual": 729.98, "uploaded_at": "2020-12-10T15:15:45+03:00"},
		{"number": "18", "status": "PROCESSED", "accrual": 0, "uploaded_at": "2020-12-10T15:14:00+03:00"},
		{"number": "12345678903", "status": "PROCESSING", "uploaded_at": "2020-12-10T15:12:01+03:00"},
		{"number": "346436439", "status": "INVALID", "uploaded_at": "2020-12-09T16:09:53+03:00"}
	]`, w.Body.String())
}

func TestListOrdersEmpty(t *testing.T) {
	cases := []struct {
		name   string
		orders []model.Order
	}{
		{"пустой срез", []model.Order{}},
		{"nil вместо среза", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			orders := newMockOrderService(t)
			orders.On("List", mock.Anything, int64(7)).Return(c.orders, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
			req.Header.Set("Authorization", bearer(t, 7))
			req.Header.Set("Accept-Encoding", "gzip")
			w := httptest.NewRecorder()

			newRouter(newMockUserService(t), orders, newMockBalanceService(t)).ServeHTTP(w, req)

			assert.Equal(t, http.StatusNoContent, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), "application/json", "автотест проверяет Content-Type и на 204")
			assert.Empty(t, w.Header().Get("Content-Encoding"))
			assert.Empty(t, w.Body.String())
		})
	}
}

func TestListOrdersFailure(t *testing.T) {
	orders := newMockOrderService(t)
	orders.On("List", mock.Anything, int64(7)).Return(nil, errors.New("база недоступна"))

	w := send(newRouter(newMockUserService(t), orders, newMockBalanceService(t)), http.MethodGet, "/api/user/orders", "", bearer(t, 7))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", w.Body.String())
}
