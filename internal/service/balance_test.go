package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

func TestWithdraw(t *testing.T) {
	boom := errors.New("база недоступна")

	cases := []struct {
		name    string
		repoErr error
	}{
		{"списано", nil},
		{"недостаточно средств", model.ErrInsufficientFunds},
		{"сумма нулевая после округления", model.ErrInvalidWithdrawSum},
		{"сбой хранилища", boom},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newMockBalanceRepository(t)
			repo.On("Withdraw", mock.Anything, int64(7), "2377225624", model.Money(751.5)).Return(c.repoErr)

			err := NewBalance(repo).Withdraw(t.Context(), 7, "2377225624", 751.5)
			if c.repoErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, c.repoErr)
		})
	}
}

func TestWithdrawRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		order   string
		sum     model.Money
		wantErr error
	}{
		{"нулевая сумма", "2377225624", 0, model.ErrInvalidWithdrawSum},
		{"отрицательная сумма", "2377225624", -0.01, model.ErrInvalidWithdrawSum},
		{"номер не проходит проверку Луна", "12345678902", 10, model.ErrInvalidOrderNumber},
		{"буквы в номере", "abc", 10, model.ErrInvalidOrderNumber},
		{"пустой номер", "", 10, model.ErrInvalidOrderNumber},
		{"сумма проверяется раньше номера", "12345678902", -1, model.ErrInvalidWithdrawSum},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newMockBalanceRepository(t)

			err := NewBalance(repo).Withdraw(t.Context(), 7, c.order, c.sum)
			assert.ErrorIs(t, err, c.wantErr)
		})
	}
}

func TestGetBalance(t *testing.T) {
	want := model.Balance{Current: 500.5, Withdrawn: 42}

	repo := newMockBalanceRepository(t)
	repo.On("Balance", mock.Anything, int64(7)).Return(want, nil)

	got, err := NewBalance(repo).Get(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListWithdrawals(t *testing.T) {
	want := []model.Withdrawal{{Order: "2377225624", Sum: 500, ProcessedAt: time.Now()}}

	repo := newMockBalanceRepository(t)
	repo.On("UserWithdrawals", mock.Anything, int64(7)).Return(want, nil)

	got, err := NewBalance(repo).Withdrawals(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
