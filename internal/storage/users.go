package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

// CreateUser сохраняет пользователя и возвращает его идентификатор.
// Если логин уже занят, возвращает model.ErrLoginTaken.
func (s *PGStorage) CreateUser(ctx context.Context, login, passwordHash string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`,
		login, passwordHash,
	).Scan(&id)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return 0, model.ErrLoginTaken
	}
	if err != nil {
		return 0, fmt.Errorf("не сохранил пользователя: %w", err)
	}

	return id, nil
}

// UserByLogin ищет пользователя по логину с учётом регистра.
// Если такого пользователя нет, возвращает model.ErrUserNotFound.
func (s *PGStorage) UserByLogin(ctx context.Context, login string) (model.User, error) {
	var u model.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, login, password_hash FROM users WHERE login = $1`, login,
	).Scan(&u.ID, &u.Login, &u.PasswordHash)

	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, model.ErrUserNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("не прочитал пользователя: %w", err)
	}

	return u, nil
}
