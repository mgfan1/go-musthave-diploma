// Package retry повторяет операцию с паузами, пока её ошибка считается временной.
package retry

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Retrier повторяет операции по заданному расписанию пауз.
type Retrier struct {
	log    *zap.Logger
	delays []time.Duration
}

// New создаёт Retrier с паузами delays между попытками. Если паузы
// не заданы, берётся расписание 1, 3 и 5 секунд.
func New(log *zap.Logger, delays ...time.Duration) *Retrier {
	if len(delays) == 0 {
		delays = []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}
	}
	return &Retrier{log: log, delays: delays}
}

// Do выполняет op и повторяет её после каждой паузы, пока retriable признаёт
// ошибку временной. Возвращает ошибку последней попытки или ошибку контекста,
// если ctx отменён во время ожидания.
func (r *Retrier) Do(ctx context.Context, retriable func(error) bool, op func() error) error {
	err := op()

	for i, delay := range r.delays {
		if err == nil || !retriable(err) {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		r.log.Warn("повторная попытка", zap.Int("attempt", i+1), zap.Duration("delay", delay), zap.Error(err))

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		err = op()
	}

	return err
}
