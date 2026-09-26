package storage

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

func TestPGCreateUser(t *testing.T) {
	ctx := t.Context()
	s, _ := newPGStorage(t)

	id, err := s.CreateUser(ctx, "gopher", "hash")
	require.NoError(t, err)
	assert.Positive(t, id)

	u, err := s.UserByLogin(ctx, "gopher")
	require.NoError(t, err)
	assert.Equal(t, model.User{ID: id, Login: "gopher", PasswordHash: "hash"}, u)
}

func TestPGCreateUserLoginTaken(t *testing.T) {
	ctx := t.Context()
	s, _ := newPGStorage(t)

	_, err := s.CreateUser(ctx, "gopher", "hash")
	require.NoError(t, err)

	_, err = s.CreateUser(ctx, "gopher", "другой хеш")
	assert.ErrorIs(t, err, model.ErrLoginTaken)
}

func TestPGCreateUserConcurrently(t *testing.T) {
	ctx := t.Context()
	s, _ := newPGStorage(t)

	const attempts = 10
	errs := make(chan error, attempts)

	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			_, err := s.CreateUser(ctx, "gopher", "hash")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	created, taken := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, model.ErrLoginTaken):
			taken++
		default:
			t.Errorf("неожиданная ошибка: %v", err)
		}
	}

	assert.Equal(t, 1, created, "зарегистрироваться должен ровно один")
	assert.Equal(t, attempts-1, taken)
}

func TestPGLoginIsCaseSensitive(t *testing.T) {
	ctx := t.Context()
	s, _ := newPGStorage(t)

	lower, err := s.CreateUser(ctx, "gopher", "hash")
	require.NoError(t, err)
	upper, err := s.CreateUser(ctx, "Gopher", "hash")
	require.NoError(t, err)
	assert.NotEqual(t, lower, upper)

	_, err = s.UserByLogin(ctx, "GOPHER")
	assert.ErrorIs(t, err, model.ErrUserNotFound)
}

func TestPGUserNotFound(t *testing.T) {
	s, _ := newPGStorage(t)

	_, err := s.UserByLogin(t.Context(), "nobody")
	assert.ErrorIs(t, err, model.ErrUserNotFound)
}
