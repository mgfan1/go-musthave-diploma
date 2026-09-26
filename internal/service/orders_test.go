package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
	"github.com/mgfan1/go-musthave-diploma/internal/service/mocks"
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
			repo := mocks.NewOrderRepository(t)
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
			repo := mocks.NewOrderRepository(t)

			_, err := NewOrders(repo).Upload(t.Context(), 7, number)
			assert.ErrorIs(t, err, model.ErrInvalidOrderNumber)
		})
	}
}

func TestListOrders(t *testing.T) {
	want := []model.Order{{Number: "12345678903", Status: model.StatusNew, UploadedAt: time.Now()}}

	repo := mocks.NewOrderRepository(t)
	repo.On("UserOrders", mock.Anything, int64(7)).Return(want, nil)

	got, err := NewOrders(repo).List(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
