package storage

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

func insertWithdrawal(t *testing.T, db *sql.DB, userID int64, order string, amount any, processedAt time.Time) {
	t.Helper()

	_, err := db.ExecContext(t.Context(),
		`INSERT INTO withdrawals (user_id, order_number, amount, processed_at) VALUES ($1, $2, $3, $4)`,
		userID, order, amount, processedAt)
	require.NoError(t, err)
}

func storedAmounts(t *testing.T, db *sql.DB, userID int64) string {
	t.Helper()

	var amounts sql.NullString
	err := db.QueryRowContext(t.Context(),
		`SELECT string_agg(amount::text, ' ' ORDER BY id) FROM withdrawals WHERE user_id = $1`, userID,
	).Scan(&amounts)
	require.NoError(t, err)

	return amounts.String
}

func TestPGBalanceEmpty(t *testing.T) {
	s, db := newPGStorage(t)
	alice := insertUser(t, db, "alice")

	b, err := s.Balance(t.Context(), alice)
	require.NoError(t, err)
	assert.Equal(t, model.Balance{}, b)
}

func TestPGBalanceIsExact(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")
	bob := insertUser(t, db, "bob")
	now := time.Now()

	insertOrder(t, db, "1", alice, "PROCESSED", 0.1, now)
	insertOrder(t, db, "2", alice, "PROCESSED", 0.2, now)
	insertOrder(t, db, "3", alice, "PROCESSED", 729.98, now)
	insertOrder(t, db, "4", alice, "PROCESSED", nil, now)
	insertOrder(t, db, "5", alice, "PROCESSING", nil, now)
	insertOrder(t, db, "6", alice, "INVALID", nil, now)
	insertOrder(t, db, "7", alice, "NEW", nil, now)
	insertOrder(t, db, "8", bob, "PROCESSED", 1000, now)

	for _, sum := range []model.Money{0.1, 0.1, 0.1, 100.01} {
		require.NoError(t, s.Withdraw(ctx, alice, "2377225624", sum))
	}
	require.NoError(t, s.Withdraw(ctx, bob, "2377225624", 1))

	b, err := s.Balance(ctx, alice)
	require.NoError(t, err)
	assert.Equal(t, model.Balance{Current: 629.97, Withdrawn: 100.31}, b,
		"730.28 начислено по обработанным заказам, 100.31 списано, чужие начисления и списания не считаются")
	assert.Equal(t, "0.10 0.10 0.10 100.01", storedAmounts(t, db, alice), "суммы списаний доезжают до базы без хвостов")
}

func TestPGWithdrawInsufficientFunds(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")
	insertOrder(t, db, "1", alice, "PROCESSED", 100, time.Now())
	insertOrder(t, db, "2", alice, "PROCESSING", nil, time.Now())

	err := s.Withdraw(ctx, alice, "2377225624", 100.01)
	require.ErrorIs(t, err, model.ErrInsufficientFunds, "не хватает одной копейки")

	require.NoError(t, s.Withdraw(ctx, alice, "2377225624", 100), "списать весь баланс можно")

	err = s.Withdraw(ctx, alice, "2377225624", 0.01)
	require.ErrorIs(t, err, model.ErrInsufficientFunds)

	b, err := s.Balance(ctx, alice)
	require.NoError(t, err)
	assert.Equal(t, model.Balance{Current: 0, Withdrawn: 100}, b)
	assert.Equal(t, "100.00", storedAmounts(t, db, alice), "отказ не оставляет следов в базе")
}

func TestPGWithdrawUnknownUser(t *testing.T) {
	s, db := newPGStorage(t)
	alice := insertUser(t, db, "alice")

	err := s.Withdraw(t.Context(), alice+1, "2377225624", 1)
	require.ErrorIs(t, err, model.ErrUserNotFound)
}

func TestPGWithdrawRoundsToKopecks(t *testing.T) {
	cases := []struct {
		name    string
		sum     model.Money
		wantErr error
		want    string
	}{
		{name: "полкопейки округляются вверх", sum: 10.005, want: "10.01"},
		{name: "меньше полкопейки округляется до нуля", sum: 0.004, wantErr: model.ErrInvalidWithdrawSum},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, db := newPGStorage(t)
			alice := insertUser(t, db, "alice")
			insertOrder(t, db, "1", alice, "PROCESSED", 100, time.Now())

			err := s.Withdraw(t.Context(), alice, "2377225624", c.sum)
			if c.wantErr != nil {
				require.ErrorIs(t, err, c.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, c.want, storedAmounts(t, db, alice))
		})
	}
}

func TestPGWithdrawComparesRoundedSum(t *testing.T) {
	cases := []struct {
		name    string
		sum     model.Money
		wantErr error
		stored  string
		balance model.Balance
	}{
		{name: "после округления равна балансу", sum: 100.004, stored: "100.00", balance: model.Balance{Current: 0, Withdrawn: 100}},
		{name: "после округления больше баланса", sum: 100.006, wantErr: model.ErrInsufficientFunds, balance: model.Balance{Current: 100}},
		{name: "не помещается в колонку", sum: 1e10, wantErr: model.ErrInsufficientFunds, balance: model.Balance{Current: 100}},
		{name: "после округления не помещается в колонку", sum: 9999999999.995, wantErr: model.ErrInsufficientFunds, balance: model.Balance{Current: 100}},
		{name: "огромная сумма", sum: 1e300, wantErr: model.ErrInsufficientFunds, balance: model.Balance{Current: 100}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, db := newPGStorage(t)
			alice := insertUser(t, db, "alice")
			insertOrder(t, db, "1", alice, "PROCESSED", 100, time.Now())

			err := s.Withdraw(t.Context(), alice, "2377225624", c.sum)
			if c.wantErr != nil {
				require.ErrorIs(t, err, c.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, c.stored, storedAmounts(t, db, alice))

			b, err := s.Balance(t.Context(), alice)
			require.NoError(t, err)
			assert.Equal(t, c.balance, b)
		})
	}
}

func TestPGWithdrawTooLargeForColumn(t *testing.T) {
	s, db := newPGStorage(t)
	alice := insertUser(t, db, "alice")
	insertOrder(t, db, "1", alice, "PROCESSED", 9999999999.99, time.Now())
	insertOrder(t, db, "2", alice, "PROCESSED", 9999999999.99, time.Now())

	err := s.Withdraw(t.Context(), alice, "2377225624", 1e10)
	require.ErrorIs(t, err, model.ErrInvalidWithdrawSum, "баллов хватает, но сумма не помещается в колонку")
	assert.Empty(t, storedAmounts(t, db, alice))
}

func TestPGWithdrawConcurrently(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	const (
		accrued  = 1000
		sum      = 75
		attempts = 20
		fits     = accrued / sum
	)

	alice := insertUser(t, db, "alice")
	insertOrder(t, db, "1", alice, "PROCESSED", accrued, time.Now())

	start := make(chan struct{})
	errs := make(chan error, attempts)

	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			<-start
			errs <- s.Withdraw(ctx, alice, "2377225624", sum)
		})
	}
	close(start)
	wg.Wait()
	close(errs)

	var succeeded, refused int
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, model.ErrInsufficientFunds):
			refused++
		default:
			t.Errorf("неожиданная ошибка списания: %v", err)
		}
	}

	assert.Equal(t, fits, succeeded, "пройти должно ровно столько списаний, сколько помещается в баланс")
	assert.Equal(t, attempts-fits, refused)

	b, err := s.Balance(ctx, alice)
	require.NoError(t, err)
	assert.Equal(t, model.Balance{Current: accrued - fits*sum, Withdrawn: fits * sum}, b, "баланс не уходит в минус")
}

func TestPGUserWithdrawals(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")
	bob := insertUser(t, db, "bob")
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	insertWithdrawal(t, db, alice, "2377225624", 500, base)
	insertWithdrawal(t, db, alice, "12345678903", 0.01, base.Add(2*time.Minute))
	insertWithdrawal(t, db, alice, "79927398713", 729.98, base.Add(time.Minute))
	insertWithdrawal(t, db, alice, "346436439", 42, base)
	insertWithdrawal(t, db, bob, "18", 1, base.Add(3*time.Minute))

	got, err := s.UserWithdrawals(ctx, alice)
	require.NoError(t, err)

	want := []model.Withdrawal{
		{Order: "12345678903", Sum: 0.01, ProcessedAt: base.Add(2 * time.Minute)},
		{Order: "79927398713", Sum: 729.98, ProcessedAt: base.Add(time.Minute)},
		{Order: "346436439", Sum: 42, ProcessedAt: base},
		{Order: "2377225624", Sum: 500, ProcessedAt: base},
	}
	require.Len(t, got, len(want), "чужие списания в список не попадают")
	for i := range want {
		assert.Equal(t, want[i].Order, got[i].Order, "сначала новые, при равном времени позже записанные")
		assert.Equal(t, want[i].Sum, got[i].Sum)
		assert.WithinDuration(t, want[i].ProcessedAt, got[i].ProcessedAt, 0)
	}
}

func TestPGUserWithdrawalsEmpty(t *testing.T) {
	s, db := newPGStorage(t)
	alice := insertUser(t, db, "alice")

	withdrawals, err := s.UserWithdrawals(t.Context(), alice)
	require.NoError(t, err)
	assert.NotNil(t, withdrawals)
	assert.Empty(t, withdrawals)
}
