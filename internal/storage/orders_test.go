package storage

import (
	"database/sql"
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

func TestPGUserOrdersEmpty(t *testing.T) {
	s, db := newPGStorage(t)
	alice := insertUser(t, db, "alice")

	orders, err := s.UserOrders(t.Context(), alice)
	require.NoError(t, err)
	assert.NotNil(t, orders)
	assert.Empty(t, orders)
}
