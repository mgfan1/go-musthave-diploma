package service

import (
	"context"
	"errors"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// UserRepository описывает хранилище пользователей.
type UserRepository interface {
	// CreateUser сохраняет пользователя и возвращает его идентификатор.
	// Если логин занят, возвращает model.ErrLoginTaken.
	CreateUser(ctx context.Context, login, passwordHash string) (int64, error)
	// UserByLogin ищет пользователя по логину. Если его нет,
	// возвращает model.ErrUserNotFound.
	UserByLogin(ctx context.Context, login string) (model.User, error)
}

// TokenIssuer выпускает токены доступа. Его реализует auth.Tokens.
type TokenIssuer interface {
	// Issue возвращает подписанный токен доступа для пользователя userID.
	Issue(userID int64) (string, error)
}

// Users регистрирует пользователей и проверяет их пароли. И регистрация,
// и вход заканчиваются выдачей токена доступа, поэтому после регистрации
// отдельный вход не нужен.
type Users struct {
	repo   UserRepository
	tokens TokenIssuer
}

// NewUsers создаёт сервис пользователей поверх хранилища repo.
// Токены доступа выпускает tokens.
func NewUsers(repo UserRepository, tokens TokenIssuer) *Users {
	return &Users{repo: repo, tokens: tokens}
}

// Register создаёт пользователя и сразу выпускает для него токен доступа.
// Если логин занят, возвращает model.ErrLoginTaken. Если токен выпустить
// не удалось, возвращает ошибку, но пользователь уже сохранён: повторная
// регистрация с тем же логином получит model.ErrLoginTaken, а войти
// можно через Login.
func (s *Users) Register(ctx context.Context, login, password string) (string, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}

	id, err := s.repo.CreateUser(ctx, login, hash)
	if err != nil {
		return "", err
	}

	return s.tokens.Issue(id)
}

// Login проверяет пару логин и пароль и выпускает токен доступа.
// Для неизвестного логина и для неверного пароля возвращает одну и ту же
// ошибку model.ErrInvalidCredentials.
func (s *Users) Login(ctx context.Context, login, password string) (string, error) {
	user, err := s.repo.UserByLogin(ctx, login)
	if errors.Is(err, model.ErrUserNotFound) {
		return "", model.ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}

	if !auth.CheckPassword(user.PasswordHash, password) {
		return "", model.ErrInvalidCredentials
	}

	return s.tokens.Issue(user.ID)
}
