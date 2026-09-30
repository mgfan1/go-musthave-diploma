package storage

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
)

func queryRows[T any](ctx context.Context, db *sql.DB, what string, fields func(*T) []any, query string, args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			yield(zero, fmt.Errorf("не прочитал %s: %w", what, err))
			return
		}
		defer rows.Close()

		for rows.Next() {
			var v T
			if err := rows.Scan(fields(&v)...); err != nil {
				yield(zero, fmt.Errorf("не прочитал %s: %w", what, err))
				return
			}
			if !yield(v, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(zero, fmt.Errorf("не прочитал %s: %w", what, err))
		}
	}
}
