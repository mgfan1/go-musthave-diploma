package storage

import (
	"database/sql"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

type orderRow struct {
	userID  int64
	status  string
	accrual sql.NullString
}

func readOrder(t *testing.T, db *sql.DB, number string) orderRow {
	t.Helper()

	var r orderRow
	err := db.QueryRowContext(t.Context(),
		`SELECT user_id, status, accrual::text FROM orders WHERE number = $1`, number,
	).Scan(&r.userID, &r.status, &r.accrual)
	require.NoError(t, err)

	return r
}

func TestPGCreateOrder(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")
	bob := insertUser(t, db, "bob")

	owner, created, err := s.CreateOrder(ctx, alice, "12345678903")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, alice, owner)

	owner, created, err = s.CreateOrder(ctx, alice, "12345678903")
	require.NoError(t, err)
	assert.False(t, created, "повторная загрузка не создаёт второй заказ")
	assert.Equal(t, alice, owner)

	owner, created, err = s.CreateOrder(ctx, bob, "12345678903")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, alice, owner, "владельцем остаётся тот, кто загрузил номер первым")

	assert.Equal(t, orderRow{userID: alice, status: "NEW"}, readOrder(t, db, "12345678903"))
}

func TestPGCreateOrderUnknownUser(t *testing.T) {
	s, db := newPGStorage(t)
	alice := insertUser(t, db, "alice")

	_, _, err := s.CreateOrder(t.Context(), alice+1, "12345678903")
	require.ErrorIs(t, err, model.ErrUserNotFound)

	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM orders`).Scan(&count))
	assert.Zero(t, count)
}

func TestPGCreateOrderKeepsProcessedOrder(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")

	_, err := db.ExecContext(ctx,
		`INSERT INTO orders (number, user_id, status, accrual) VALUES ('12345678903', $1, 'PROCESSED', 729.98)`, alice)
	require.NoError(t, err)

	_, created, err := s.CreateOrder(ctx, alice, "12345678903")
	require.NoError(t, err)
	assert.False(t, created)

	want := orderRow{userID: alice, status: "PROCESSED", accrual: sql.NullString{String: "729.98", Valid: true}}
	assert.Equal(t, want, readOrder(t, db, "12345678903"), "повторная загрузка не сбрасывает статус и начисление")
}

func TestPGCreateOrderConcurrently(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	users := []int64{insertUser(t, db, "alice"), insertUser(t, db, "bob")}

	const attempts = 10
	type result struct {
		userID  int64
		ownerID int64
		created bool
		err     error
	}
	results := make(chan result, attempts)

	var wg sync.WaitGroup
	for i := range attempts {
		userID := users[i%len(users)]
		wg.Go(func() {
			owner, created, err := s.CreateOrder(ctx, userID, "12345678903")
			results <- result{userID: userID, ownerID: owner, created: created, err: err}
		})
	}
	wg.Wait()
	close(results)

	var creator int64
	owners := make(map[int64]bool)
	for r := range results {
		require.NoError(t, r.err)
		owners[r.ownerID] = true
		if r.created {
			assert.Zero(t, creator, "создать заказ должен ровно один запрос")
			creator = r.userID
		}
	}

	require.NotZero(t, creator)
	assert.Equal(t, map[int64]bool{creator: true}, owners, "все должны увидеть одного и того же владельца")
}

func TestPGUserOrders(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")
	bob := insertUser(t, db, "bob")
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	rows := []struct {
		number     string
		userID     int64
		status     string
		accrual    any
		uploadedAt time.Time
	}{
		{"12345678903", alice, "NEW", nil, base},
		{"9278923470", alice, "PROCESSED", 729.98, base.Add(2 * time.Minute)},
		{"18", alice, "PROCESSED", 0, base.Add(time.Minute)},
		{"79927398713", bob, "PROCESSED", 100, base.Add(3 * time.Minute)},
	}
	for _, r := range rows {
		_, err := db.ExecContext(ctx,
			`INSERT INTO orders (number, user_id, status, accrual, uploaded_at) VALUES ($1, $2, $3, $4, $5)`,
			r.number, r.userID, r.status, r.accrual, r.uploadedAt)
		require.NoError(t, err)
	}

	orders, err := s.UserOrders(ctx, alice)
	require.NoError(t, err)
	require.Len(t, orders, 3, "чужие заказы в список не попадают")

	assert.Equal(t, []string{"9278923470", "18", "12345678903"},
		[]string{orders[0].Number, orders[1].Number, orders[2].Number}, "сначала новые")

	require.NotNil(t, orders[0].Accrual)
	assert.Equal(t, model.Money(729.98), *orders[0].Accrual)
	assert.Equal(t, model.StatusProcessed, orders[0].Status)

	require.NotNil(t, orders[1].Accrual, "нулевое начисление не то же самое, что его отсутствие")
	assert.Zero(t, *orders[1].Accrual)

	assert.Nil(t, orders[2].Accrual)
	assert.Equal(t, model.StatusNew, orders[2].Status)
	assert.True(t, base.Equal(orders[2].UploadedAt))
}

func insertOrder(t *testing.T, db *sql.DB, number string, userID int64, status string, accrual any, uploadedAt time.Time) {
	t.Helper()

	_, err := db.ExecContext(t.Context(),
		`INSERT INTO orders (number, user_id, status, accrual, uploaded_at) VALUES ($1, $2, $3, $4, $5)`,
		number, userID, status, accrual, uploadedAt)
	require.NoError(t, err)
}

func TestPGClaimPendingOrders(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")
	base := time.Now().Add(-time.Hour)

	insertOrder(t, db, "1", alice, "NEW", nil, base)
	insertOrder(t, db, "2", alice, "PROCESSING", nil, base.Add(time.Minute))
	insertOrder(t, db, "3", alice, "NEW", nil, base.Add(2*time.Minute))
	insertOrder(t, db, "4", alice, "PROCESSED", 10, base)
	insertOrder(t, db, "5", alice, "INVALID", nil, base)

	first, err := s.ClaimPendingOrders(ctx, 2)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1", "2"}, first, "сначала самые старые из неопрошенных")

	second, err := s.ClaimPendingOrders(ctx, 2)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"3", "1"}, second, "неопрошенный заказ идёт раньше уже опрошенных")

	all, err := s.ClaimPendingOrders(ctx, 10)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1", "2", "3"}, all, "заказы в окончательных статусах не опрашиваются")
}

func TestPGClaimPendingOrdersSkipsLockedRows(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")
	insertOrder(t, db, "1", alice, "NEW", nil, time.Now().Add(-time.Minute))
	insertOrder(t, db, "2", alice, "NEW", nil, time.Now())

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `SELECT number FROM orders WHERE number = '1' FOR UPDATE`)
	require.NoError(t, err)

	numbers, err := s.ClaimPendingOrders(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"2"}, numbers, "заказ, который держит другая выдача, пропускается без ожидания")
}

func TestPGUpdateOrder(t *testing.T) {
	amount := model.Money(729.98)
	other := model.Money(1000)

	cases := []struct {
		name        string
		status      string
		accrual     any
		newStatus   model.OrderStatus
		newAccrual  *model.Money
		wantStatus  string
		wantAccrual sql.NullString
	}{
		{"NEW в PROCESSING", "NEW", nil, model.StatusProcessing, nil, "PROCESSING", sql.NullString{}},
		{"NEW сразу в PROCESSED", "NEW", nil, model.StatusProcessed, &amount, "PROCESSED", sql.NullString{String: "729.98", Valid: true}},
		{"PROCESSING в INVALID", "PROCESSING", nil, model.StatusInvalid, nil, "INVALID", sql.NullString{}},
		{"PROCESSED без начисления", "PROCESSING", nil, model.StatusProcessed, nil, "PROCESSED", sql.NullString{}},
		{"PROCESSED не откатывается в PROCESSING", "PROCESSED", 729.98, model.StatusProcessing, nil, "PROCESSED", sql.NullString{String: "729.98", Valid: true}},
		{"PROCESSED не получает второе начисление", "PROCESSED", 729.98, model.StatusProcessed, &other, "PROCESSED", sql.NullString{String: "729.98", Valid: true}},
		{"INVALID окончательный", "INVALID", nil, model.StatusProcessed, &other, "INVALID", sql.NullString{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, db := newPGStorage(t)
			alice := insertUser(t, db, "alice")
			insertOrder(t, db, "12345678903", alice, c.status, c.accrual, time.Now())

			require.NoError(t, s.UpdateOrder(t.Context(), "12345678903", c.newStatus, c.newAccrual))

			want := orderRow{userID: alice, status: c.wantStatus, accrual: c.wantAccrual}
			assert.Equal(t, want, readOrder(t, db, "12345678903"))
		})
	}
}

func TestPGUpdateOrderAccrualIsInBalanceAtOnce(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")
	insertOrder(t, db, "12345678903", alice, "PROCESSING", nil, time.Now())

	amount := model.Money(729.98)
	require.NoError(t, s.UpdateOrder(ctx, "12345678903", model.StatusProcessed, &amount))

	b, err := s.Balance(ctx, alice)
	require.NoError(t, err)
	assert.Equal(t, model.Balance{Current: 729.98}, b)
}

func TestPGUpdateOrderIsAtomicForReaders(t *testing.T) {
	ctx := t.Context()
	s, db := newPGStorage(t)

	alice := insertUser(t, db, "alice")

	const count = 20
	numbers := make([]string, 0, count)
	for i := range count {
		number := strconv.Itoa(1000 + i)
		insertOrder(t, db, number, alice, "PROCESSING", nil, time.Now())
		numbers = append(numbers, number)
	}

	stop := make(chan struct{})
	var checks, broken int
	var readErr error

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			var consistent bool
			err := db.QueryRowContext(ctx,
				`SELECT COALESCE(SUM(accrual), 0) = count(*) * 1.01
				 FROM orders WHERE user_id = $1 AND status = 'PROCESSED'`, alice,
			).Scan(&consistent)
			if err != nil {
				readErr = err
				return
			}
			checks++
			if !consistent {
				broken++
			}

			select {
			case <-stop:
				return
			default:
			}
		}
	})

	amount := model.Money(1.01)
	for _, number := range numbers {
		if err := s.UpdateOrder(ctx, number, model.StatusProcessed, &amount); err != nil {
			t.Errorf("не обновил заказ %s: %v", number, err)
		}
	}
	close(stop)
	wg.Wait()

	require.NoError(t, readErr)
	assert.Positive(t, checks)
	assert.Zero(t, broken, "статус PROCESSED не должен быть виден без начисления")
}

func TestPGUserOrdersEmpty(t *testing.T) {
	s, db := newPGStorage(t)
	alice := insertUser(t, db, "alice")

	orders, err := s.UserOrders(t.Context(), alice)
	require.NoError(t, err)
	assert.NotNil(t, orders)
	assert.Empty(t, orders)
}
