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

func TestUploadOrder(t *testing.T) {
	const userID, otherID int64 = 7, 8
	boom := errors.New("база недоступна")

	cases := []struct {
		name        string
		ownerID     int64
		created     bool
		repoErr     error
		wantCreated bool
		wantErr     error
	}{
		{name: "новый заказ", ownerID: userID, created: true, wantCreated: true},
		{name: "повторная загрузка своего заказа", ownerID: userID},
		{name: "заказ другого пользователя", ownerID: otherID, wantErr: model.ErrOrderOwnedByOther},
		{name: "сбой хранилища", repoErr: boom, wantErr: boom},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newMockOrderRepository(t)
			repo.On("CreateOrder", mock.Anything, userID, "12345678903").Return(c.ownerID, c.created, c.repoErr)

			created, err := NewOrders(repo).Upload(t.Context(), userID, "12345678903")
			if c.wantErr != nil {
				require.ErrorIs(t, err, c.wantErr)
				assert.False(t, created)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.wantCreated, created)
		})
	}
}

func TestUploadOrderRejectsInvalidNumber(t *testing.T) {
	for _, number := range []string{"12345678902", "abc", "", "1234 5678 903"} {
		t.Run(number, func(t *testing.T) {
			repo := newMockOrderRepository(t)

			_, err := NewOrders(repo).Upload(t.Context(), 7, number)
			assert.ErrorIs(t, err, model.ErrInvalidOrderNumber)
		})
	}
}

func TestApplyAccrual(t *testing.T) {
	amount := model.Money(729.98)

	cases := []struct {
		name        string
		result      model.AccrualResult
		wantAccrual *model.Money
	}{
		{"расчёт окончен с начислением", model.AccrualResult{Status: model.StatusProcessed, Amount: &amount}, &amount},
		{"расчёт окончен без начисления", model.AccrualResult{Status: model.StatusProcessed}, nil},
		{"расчёт идёт", model.AccrualResult{Status: model.StatusProcessing, Amount: &amount}, nil},
		{"отказ в расчёте", model.AccrualResult{Status: model.StatusInvalid, Amount: &amount}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newMockOrderRepository(t)
			repo.On("UpdateOrder", mock.Anything, "12345678903", c.result.Status, c.wantAccrual).Return(nil)

			require.NoError(t, NewOrders(repo).ApplyAccrual(t.Context(), "12345678903", c.result))
		})
	}
}

func TestApplyAccrualRejectsImpossibleStatus(t *testing.T) {
	for _, status := range []model.OrderStatus{model.StatusNew, "REGISTERED", ""} {
		t.Run(string(status), func(t *testing.T) {
			repo := newMockOrderRepository(t)

			err := NewOrders(repo).ApplyAccrual(t.Context(), "12345678903", model.AccrualResult{Status: status})
			assert.Error(t, err)
		})
	}
}

func TestApplyAccrualRepositoryError(t *testing.T) {
	boom := errors.New("база недоступна")

	repo := newMockOrderRepository(t)
	repo.On("UpdateOrder", mock.Anything, "12345678903", model.StatusInvalid, (*model.Money)(nil)).Return(boom)

	err := NewOrders(repo).ApplyAccrual(t.Context(), "12345678903", model.AccrualResult{Status: model.StatusInvalid})
	assert.ErrorIs(t, err, boom)
}

func TestClaimPending(t *testing.T) {
	repo := newMockOrderRepository(t)
	repo.On("ClaimPendingOrders", mock.Anything, 10).Return([]string{"12345678903"}, nil)

	got, err := NewOrders(repo).ClaimPending(t.Context(), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"12345678903"}, got)
}

func TestListOrders(t *testing.T) {
	want := []model.Order{{Number: "12345678903", Status: model.StatusNew, UploadedAt: time.Now()}}

	repo := newMockOrderRepository(t)
	repo.On("UserOrders", mock.Anything, int64(7)).Return(want, nil)

	got, err := NewOrders(repo).List(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
