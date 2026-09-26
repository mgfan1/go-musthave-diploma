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

// BalanceService описывает баланс пользователя и списание баллов.
type BalanceService interface {
	// Get возвращает баланс пользователя.
	Get(ctx context.Context, userID int64) (model.Balance, error)
	// Withdraw списывает баллы в счёт заказа. Возвращает
	// model.ErrInvalidWithdrawSum, model.ErrInvalidOrderNumber
	// или model.ErrInsufficientFunds, если списание невозможно.
	Withdraw(ctx context.Context, userID int64, order string, sum model.Money) error
	// Withdrawals возвращает списания пользователя от новых к старым.
	Withdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

// Handler переводит запросы HTTP API в вызовы сервисов, а их ошибки в коды
// ответа. Сам он http.Handler не реализует, маршруты к нему собирает Router.
type Handler struct {
	users   UserService
	orders  OrderService
	balance BalanceService
	log     *zap.Logger
}

// New создаёт обработчик API поверх сервисов пользователей users,
// заказов orders и баланса balance. Сбои сервисов пишутся в log на уровне
// Warn вместе с методом и путём запроса и пользователем, если он известен,
// а клиент получает 500 без подробностей.
func New(users UserService, orders OrderService, balance BalanceService, log *zap.Logger) *Handler {
	return &Handler{users: users, orders: orders, balance: balance, log: log}
}

func currentUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	}
	return userID, ok
}

func requestFields(r *http.Request) []zap.Field {
	fields := []zap.Field{
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
	}
	if userID, ok := auth.UserID(r.Context()); ok {
		fields = append(fields, zap.Int64("user_id", userID))
	}
	return fields
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	h.log.Warn(msg, append(requestFields(r), zap.Error(err))...)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
