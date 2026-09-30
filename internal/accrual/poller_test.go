package accrual

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

func newTestPoller(t *testing.T) (*Poller, *mockOrders, *mockFetcher) {
	t.Helper()

	orders := newMockOrders(t)
	fetcher := newMockFetcher(t)

	return NewPoller(orders, fetcher, zap.NewNop()), orders, fetcher
}

func TestPollerTickAppliesResults(t *testing.T) {
	p, orders, fetcher := newTestPoller(t)

	amount := model.Money(729.98)
	processed := model.AccrualResult{Status: model.StatusProcessed, Amount: &amount}
	processing := model.AccrualResult{Status: model.StatusProcessing}
	invalid := model.AccrualResult{Status: model.StatusInvalid}

	orders.On("ClaimPending", mock.Anything, batchSize).Return([]string{"1", "2", "3", "4", "5"}, nil)
	fetcher.On("Order", mock.Anything, "1").Return(processed, nil)
	fetcher.On("Order", mock.Anything, "2").Return(processing, nil)
	fetcher.On("Order", mock.Anything, "3").Return(invalid, nil)
	fetcher.On("Order", mock.Anything, "4").Return(model.AccrualResult{}, ErrNotRegistered)
	fetcher.On("Order", mock.Anything, "5").Return(model.AccrualResult{}, errors.New("система расчёта ответила 500"))
	orders.On("ApplyAccrual", mock.Anything, "1", processed).Return(nil)
	orders.On("ApplyAccrual", mock.Anything, "2", processing).Return(nil)
	orders.On("ApplyAccrual", mock.Anything, "3", invalid).Return(nil)

	p.tick(t.Context())

	orders.AssertNotCalled(t, "ApplyAccrual", mock.Anything, "4", mock.Anything)
	orders.AssertNotCalled(t, "ApplyAccrual", mock.Anything, "5", mock.Anything)
	assert.False(t, p.paused(), "обычные ошибки опрос не останавливают")
}

func TestPollerTickNothingToPoll(t *testing.T) {
	cases := []struct {
		name    string
		numbers []string
		err     error
	}{
		{"нет незавершённых заказов", nil, nil},
		{"сбой хранилища", nil, errors.New("база недоступна")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, orders, _ := newTestPoller(t)
			orders.On("ClaimPending", mock.Anything, batchSize).Return(c.numbers, c.err)

			p.tick(t.Context())
		})
	}
}

func TestPollerTickContinuesAfterApplyError(t *testing.T) {
	p, orders, fetcher := newTestPoller(t)
	p.workers = 1
	result := model.AccrualResult{Status: model.StatusProcessing}

	orders.On("ClaimPending", mock.Anything, batchSize).Return([]string{"1", "2"}, nil)
	fetcher.On("Order", mock.Anything, mock.Anything).Return(result, nil)
	orders.On("ApplyAccrual", mock.Anything, "1", result).Return(errors.New("база недоступна"))
	orders.On("ApplyAccrual", mock.Anything, "2", result).Return(nil)

	p.tick(t.Context())
}

func TestPollerPausesOnTooManyRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, orders, fetcher := newTestPoller(t)
		p.workers = 1

		orders.On("ClaimPending", mock.Anything, batchSize).Return([]string{"1", "2", "3"}, nil).Once()
		fetcher.On("Order", mock.Anything, "1").Return(model.AccrualResult{}, &TooManyRequestsError{RetryAfter: time.Minute}).Once()

		p.tick(t.Context())
		fetcher.AssertNumberOfCalls(t, "Order", 1)

		time.Sleep(time.Minute - time.Nanosecond)
		p.tick(t.Context())
		orders.AssertNumberOfCalls(t, "ClaimPending", 1)

		time.Sleep(time.Nanosecond)
		orders.On("ClaimPending", mock.Anything, batchSize).Return(nil, nil).Once()
		p.tick(t.Context())
		orders.AssertNumberOfCalls(t, "ClaimPending", 2)
	})
}

func TestPollerPauseIsSharedAndNeverShortened(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, _, fetcher := newTestPoller(t)

		p.pause(time.Minute)
		p.pause(time.Second)

		time.Sleep(time.Minute - time.Nanosecond)
		assert.True(t, p.paused(), "короткая пауза не должна отменять длинную")
		p.poll(t.Context(), "12345678903")
		fetcher.AssertNotCalled(t, "Order", mock.Anything, mock.Anything)

		time.Sleep(time.Nanosecond)
		assert.False(t, p.paused())
	})
}

func TestPollerLimitsConcurrentRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, orders, fetcher := newTestPoller(t)

		numbers := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
		release := make(chan struct{})
		orders.On("ClaimPending", mock.Anything, batchSize).Return(numbers, nil)
		orders.On("ApplyAccrual", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		fetcher.On("Order", mock.Anything, mock.Anything).
			Run(func(mock.Arguments) { <-release }).
			Return(model.AccrualResult{Status: model.StatusProcessing}, nil)

		go p.tick(t.Context())

		synctest.Wait()
		fetcher.AssertNumberOfCalls(t, "Order", workers)

		release <- struct{}{}
		synctest.Wait()
		fetcher.AssertNumberOfCalls(t, "Order", workers+1)

		close(release)
		synctest.Wait()
		fetcher.AssertNumberOfCalls(t, "Order", len(numbers))
	})
}

func TestPollerTickStopsOnCancel(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	orders := newMockOrders(t)
	fetcher := newMockFetcher(t)
	p := NewPoller(orders, fetcher, zap.New(core))
	p.workers = 1

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	orders.On("ClaimPending", mock.Anything, batchSize).Return([]string{"1", "2", "3"}, nil)
	fetcher.On("Order", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { cancel() }).
		Return(model.AccrualResult{}, context.Canceled)

	p.tick(ctx)

	fetcher.AssertNumberOfCalls(t, "Order", 1)
	assert.Zero(t, logs.Len(), "после отмены опрос не пишет предупреждений")
}

func TestPollerRunTicksUntilCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, orders, _ := newTestPoller(t)
		orders.On("ClaimPending", mock.Anything, batchSize).Return(nil, nil)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		done := make(chan struct{})
		go func() {
			p.Run(ctx)
			close(done)
		}()

		time.Sleep(pollInterval)
		synctest.Wait()
		orders.AssertNumberOfCalls(t, "ClaimPending", 1)

		time.Sleep(pollInterval)
		synctest.Wait()
		orders.AssertNumberOfCalls(t, "ClaimPending", 2)

		cancel()
		synctest.Wait()

		select {
		case <-done:
		default:
			t.Fatal("Run не вернулся после отмены контекста")
		}
	})
}
