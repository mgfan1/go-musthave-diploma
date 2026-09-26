package handler

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// UserService описывает регистрацию и вход пользователей.
type UserService interface {
	// Register создаёт пользователя и возвращает токен доступа.
	Register(ctx context.Context, login, password string) (string, error)
	// Login проверяет пару логин и пароль и возвращает токен доступа.
	Login(ctx context.Context, login, password string) (string, error)
}

// OrderService описывает приём и выдачу заказов пользователя.
type OrderService interface {
	// Upload принимает номер заказа и возвращает true, если заказ новый,
	// и false, если пользователь уже загружал этот номер.
	Upload(ctx context.Context, userID int64, number string) (bool, error)
	// List возвращает заказы пользователя от новых к старым.
	List(ctx context.Context, userID int64) ([]model.Order, error)
}

// Handler обслуживает запросы HTTP API.
type Handler struct {
	users  UserService
	orders OrderService
	log    *zap.Logger
}

// New создаёт обработчик API поверх сервисов пользователей users
// и заказов orders.
func New(users UserService, orders OrderService, log *zap.Logger) *Handler {
	return &Handler{users: users, orders: orders, log: log}
}

func currentUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	}
	return userID, ok
}

func (h *Handler) internalError(w http.ResponseWriter, msg string, err error) {
	h.log.Warn(msg, zap.Error(err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
