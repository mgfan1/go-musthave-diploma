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
			repo := newMockUserRepository(t)
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

func TestRegisterTokenIssueFails(t *testing.T) {
	boom := errors.New("не подписал токен")

	repo := newMockUserRepository(t)
	repo.On("CreateUser", mock.Anything, "gopher", mock.Anything).Return(int64(7), nil).Once()

	tokens := newMockTokenIssuer(t)
	tokens.On("Issue", int64(7)).Return("", boom)

	token, err := NewUsers(repo, tokens).Register(t.Context(), "gopher", password)
	require.ErrorIs(t, err, boom)
	assert.Empty(t, token)
	repo.AssertCalled(t, "CreateUser", mock.Anything, "gopher", mock.Anything)
}

func TestLoginTokenIssueFails(t *testing.T) {
	boom := errors.New("не подписал токен")

	hash, err := auth.HashPassword(password)
	require.NoError(t, err)

	repo := newMockUserRepository(t)
	repo.On("UserByLogin", mock.Anything, "gopher").Return(model.User{ID: 7, Login: "gopher", PasswordHash: hash}, nil)

	tokens := newMockTokenIssuer(t)
	tokens.On("Issue", int64(7)).Return("", boom)

	token, err := NewUsers(repo, tokens).Login(t.Context(), "gopher", password)
	require.ErrorIs(t, err, boom)
	assert.Empty(t, token)
}

func TestRegisterTooLongPassword(t *testing.T) {
	repo := newMockUserRepository(t)

	token, err := NewUsers(repo, auth.NewTokens("секрет", time.Hour)).
		Register(t.Context(), "gopher", strings.Repeat("a", auth.MaxPasswordLen+1))
	require.ErrorIs(t, err, model.ErrPasswordTooLong)
	assert.Empty(t, token)
}

func TestLogin(t *testing.T) {
	tokens := auth.NewTokens("секрет", time.Hour)
	boom := errors.New("база недоступна")

	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	gopher := model.User{ID: 7, Login: "gopher", PasswordHash: hash}

	longPassword := strings.Repeat("a", auth.MaxPasswordLen)
	longHash, err := auth.HashPassword(longPassword)
	require.NoError(t, err)
	longGopher := model.User{ID: 7, Login: "gopher", PasswordHash: longHash}

	cases := []struct {
		name     string
		password string
		user     model.User
		repoErr  error
		wantErr  error
	}{
		{"верный пароль", password, gopher, nil, nil},
		{"неверный пароль", "чужой пароль", gopher, nil, model.ErrInvalidCredentials},
		{"пароль предельной длины", longPassword, longGopher, nil, nil},
		{"пароль длиннее предела с верным началом", longPassword + "b", longGopher, nil, model.ErrInvalidCredentials},
		{"неизвестный логин", password, model.User{}, model.ErrUserNotFound, model.ErrInvalidCredentials},
		{"сбой хранилища", password, model.User{}, boom, boom},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newMockUserRepository(t)
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
