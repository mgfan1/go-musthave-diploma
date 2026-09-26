package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
	"github.com/mgfan1/go-musthave-diploma/internal/service/mocks"
)

const password = "пароль гофера"

func TestRegister(t *testing.T) {
	tokens := auth.NewTokens("секрет", time.Hour)
	boom := errors.New("база недоступна")

	cases := []struct {
		name    string
		id      int64
		repoErr error
		wantErr error
	}{
		{"новый пользователь", 7, nil, nil},
		{"логин занят", 0, model.ErrLoginTaken, model.ErrLoginTaken},
		{"сбой хранилища", 0, boom, boom},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := mocks.NewUserRepository(t)
			repo.On("CreateUser", mock.Anything, "gopher", mock.MatchedBy(func(hash string) bool {
				return auth.CheckPassword(hash, password)
			})).Return(c.id, c.repoErr)

			token, err := NewUsers(repo, tokens).Register(t.Context(), "gopher", password)
			if c.wantErr != nil {
				require.ErrorIs(t, err, c.wantErr)
				assert.Empty(t, token)
				return
			}
			require.NoError(t, err)

			id, err := tokens.Parse(token)
			require.NoError(t, err)
			assert.Equal(t, c.id, id)
		})
	}
}

func TestRegisterTooLongPassword(t *testing.T) {
	repo := mocks.NewUserRepository(t)

	_, err := NewUsers(repo, auth.NewTokens("секрет", time.Hour)).
		Register(t.Context(), "gopher", strings.Repeat("a", auth.MaxPasswordLen+1))
	assert.Error(t, err)
}

func TestLogin(t *testing.T) {
	tokens := auth.NewTokens("секрет", time.Hour)
	boom := errors.New("база недоступна")

	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	gopher := model.User{ID: 7, Login: "gopher", PasswordHash: hash}

	cases := []struct {
		name     string
		password string
		user     model.User
		repoErr  error
		wantErr  error
	}{
		{"верный пароль", password, gopher, nil, nil},
		{"неверный пароль", "чужой пароль", gopher, nil, model.ErrInvalidCredentials},
		{"неизвестный логин", password, model.User{}, model.ErrUserNotFound, model.ErrInvalidCredentials},
		{"сбой хранилища", password, model.User{}, boom, boom},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := mocks.NewUserRepository(t)
			repo.On("UserByLogin", mock.Anything, "gopher").Return(c.user, c.repoErr)

			token, err := NewUsers(repo, tokens).Login(t.Context(), "gopher", c.password)
			if c.wantErr != nil {
				require.ErrorIs(t, err, c.wantErr)
				assert.Empty(t, token)
				return
			}
			require.NoError(t, err)

			id, err := tokens.Parse(token)
			require.NoError(t, err)
			assert.Equal(t, gopher.ID, id)
		})
	}
}
