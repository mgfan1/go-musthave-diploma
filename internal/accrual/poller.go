package accrual

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

const (
	pollInterval = time.Second
	batchSize    = 10
	workers      = 4
)

// Orders - очередь незавершённых заказов, которую разбирает Poller.
type Orders interface {
	// ClaimPending выдаёт на опрос до limit незавершённых заказов
	// и возвращает их номера.
	ClaimPending(ctx context.Context, limit int) ([]string, error)
	// ApplyAccrual переносит в заказ number результат расчёта.
	ApplyAccrual(ctx context.Context, number string, result model.AccrualResult) error
}

// Fetcher запрашивает расчёт начисления по номеру заказа. Его реализует Client.
type Fetcher interface {
	// Order возвращает расчёт по заказу number в статусах Гофермарта.
	Order(ctx context.Context, number string) (model.AccrualResult, error)
}

// Poller периодически опрашивает систему расчёта по незавершённым заказам
// и переносит результаты в заказы. Ответ 429 приостанавливает весь опрос
// на время из Retry-After.
type Poller struct {
	orders   Orders
	fetcher  Fetcher
	log      *zap.Logger
	interval time.Duration
	workers  int

	mu          sync.Mutex
	pausedUntil time.Time
}

// NewPoller создаёт опрос, который берёт заказы из orders, а результаты
// расчёта запрашивает у fetcher.
func NewPoller(orders Orders, fetcher Fetcher, log *zap.Logger) *Poller {
	return &Poller{
		orders:   orders,
		fetcher:  fetcher,
		log:      log,
		interval: pollInterval,
		workers:  workers,
	}
}

// Run опрашивает систему расчёта, пока не отменён ctx, и возвращается,
// когда начатые запросы завершены.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

func (p *Poller) tick(ctx context.Context) {
	if p.paused() {
		return
	}

	numbers, err := p.orders.ClaimPending(ctx, batchSize)
	if err != nil {
		p.warn(ctx, "не выбрал заказы для опроса", zap.Error(err))
		return
	}

	jobs := make(chan string)

	var wg sync.WaitGroup
	for range min(p.workers, len(numbers)) {
		wg.Go(func() {
			for number := range jobs {
				p.poll(ctx, number)
			}
		})
	}

	for _, number := range numbers {
		jobs <- number
	}
	close(jobs)
	wg.Wait()
}

func (p *Poller) poll(ctx context.Context, number string) {
	if ctx.Err() != nil || p.paused() {
		return
	}

	result, err := p.fetcher.Order(ctx, number)

	var tooMany *TooManyRequestsError
	switch {
	case errors.Is(err, ErrNotRegistered):
		return
	case errors.As(err, &tooMany):
		p.pause(tooMany.RetryAfter)
		p.log.Warn("система расчёта просит паузу", zap.Duration("retry_after", tooMany.RetryAfter))
		return
	case err != nil:
		p.warn(ctx, "не получил расчёт по заказу", zap.String("order", number), zap.Error(err))
		return
	}

	if err := p.orders.ApplyAccrual(ctx, number, result); err != nil {
		p.warn(ctx, "не обновил заказ", zap.String("order", number), zap.Error(err))
		return
	}

	p.log.Debug("обновил заказ", zap.String("order", number), zap.String("status", string(result.Status)))
}

func (p *Poller) pause(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if until := time.Now().Add(d); until.After(p.pausedUntil) {
		p.pausedUntil = until
	}
}

func (p *Poller) paused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return time.Now().Before(p.pausedUntil)
}

func (p *Poller) warn(ctx context.Context, msg string, fields ...zap.Field) {
	if ctx.Err() != nil {
		return
	}
	p.log.Warn(msg, fields...)
}
