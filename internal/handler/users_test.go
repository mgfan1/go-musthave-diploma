package handler

import (
	"errors"
	"net/http"
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

const goodCredentials = `{"login":"gopher","password":"secret"}`

type credentialsCase struct {
	name     string
	token    string
	err      error
	wantCode int
}

func checkCredentialsCase(t *testing.T, method, path string, c credentialsCase) {
	t.Helper()

	users := newMockUserService(t)
	users.On(method, mock.Anything, "gopher", "secret").Return(c.token, c.err)

	w := send(newRouter(users, newMockOrderService(t), newMockBalanceService(t)), http.MethodPost, path, goodCredentials, "")

	assert.Equal(t, c.wantCode, w.Code)
	if c.wantCode == http.StatusOK {
		assert.Equal(t, "Bearer "+c.token, w.Header().Get("Authorization"))
		return
	}
	assert.Empty(t, w.Header().Get("Authorization"))
	switch c.wantCode {
	case http.StatusInternalServerError:
		assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", w.Body.String())
	case http.StatusBadRequest:
		assert.Equal(t, "неверный формат запроса\n", w.Body.String())
	}
}

func TestRegister(t *testing.T) {
	boom := errors.New("база недоступна")

	cases := []credentialsCase{
		{name: "успешная регистрация", token: "token", wantCode: http.StatusOK},
		{name: "логин занят", err: model.ErrLoginTaken, wantCode: http.StatusConflict},
		{name: "слишком длинный пароль", err: model.ErrPasswordTooLong, wantCode: http.StatusBadRequest},
		{name: "сбой сервиса", err: boom, wantCode: http.StatusInternalServerError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkCredentialsCase(t, "Register", "/api/user/register", c)
		})
	}
}

func TestLogin(t *testing.T) {
	boom := errors.New("база недоступна")

	cases := []credentialsCase{
		{name: "успешный вход", token: "token", wantCode: http.StatusOK},
		{name: "неверная пара", err: model.ErrInvalidCredentials, wantCode: http.StatusUnauthorized},
		{name: "сбой сервиса", err: boom, wantCode: http.StatusInternalServerError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkCredentialsCase(t, "Login", "/api/user/login", c)
		})
	}
}

func TestCredentialsBadRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"пустое тело", ""},
		{"битый JSON", `{"login":"gopher",`},
		{"не объект", `["gopher","secret"]`},
		{"нет логина", `{"password":"secret"}`},
		{"нет пароля", `{"login":"gopher"}`},
		{"пустые поля", `{"login":"","password":""}`},
		{"управляющий символ в логине", `{"login":"go\u0000pher","password":"secret"}`},
		{"слишком длинное тело", `{"login":"` + strings.Repeat("a", maxRequestBody) + `","password":"secret"}`},
	}

	for _, path := range []string{"/api/user/register", "/api/user/login"} {
		for _, c := range cases {
			t.Run(path+" "+c.name, func(t *testing.T) {
				w := send(newRouter(newMockUserService(t), newMockOrderService(t), newMockBalanceService(t)), http.MethodPost, path, c.body, "")
				assert.Equal(t, http.StatusBadRequest, w.Code)
			})
		}
	}
}

func TestLoginTooLongPasswordIsUnauthorized(t *testing.T) {
	password := strings.Repeat("a", auth.MaxPasswordLen+1)

	users := newMockUserService(t)
	users.On("Login", mock.Anything, "gopher", password).Return("", model.ErrInvalidCredentials)

	w := send(newRouter(users, newMockOrderService(t), newMockBalanceService(t)), http.MethodPost, "/api/user/login", `{"login":"gopher","password":"`+password+`"}`, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRegisterTokenOpensProtectedRoutes(t *testing.T) {
	tokens := auth.NewTokens(testSecret, time.Hour)
	token, err := tokens.Issue(7)
	require.NoError(t, err)

	users := newMockUserService(t)
	users.On("Register", mock.Anything, "gopher", "secret").Return(token, nil)

	balance := newMockBalanceService(t)
	balance.On("Get", mock.Anything, int64(7)).Return(model.Balance{}, nil)

	router := New(users, newMockOrderService(t), balance, zap.NewNop()).Router(zap.NewNop(), tokens)

	reg := send(router, http.MethodPost, "/api/user/register", goodCredentials, "")
	require.Equal(t, http.StatusOK, reg.Code)

	w := send(router, http.MethodGet, "/api/user/balance", "", reg.Header().Get("Authorization"))
	assert.Equal(t, http.StatusOK, w.Code, "заголовок из ответа на регистрацию должен приниматься как есть")
}
