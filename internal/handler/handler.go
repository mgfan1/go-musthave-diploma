package handler

import (
	"context"
	"net/http"

	"go.uber.org/zap"
)

// UserService описывает регистрацию и вход пользователей.
type UserService interface {
	// Register создаёт пользователя и возвращает токен доступа.
	Register(ctx context.Context, login, password string) (string, error)
	// Login проверяет пару логин и пароль и возвращает токен доступа.
	Login(ctx context.Context, login, password string) (string, error)
}

// Handler обслуживает запросы HTTP API.
type Handler struct {
	users UserService
	log   *zap.Logger
}

// New создаёт обработчик API поверх сервиса пользователей users.
func New(users UserService, log *zap.Logger) *Handler {
	return &Handler{users: users, log: log}
}

func (h *Handler) internalError(w http.ResponseWriter, msg string, err error) {
	h.log.Warn(msg, zap.Error(err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
