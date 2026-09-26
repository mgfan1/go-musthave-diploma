package auth

import "context"

type userIDKey struct{}

// WithUserID возвращает копию ctx с идентификатором аутентифицированного пользователя.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

// UserID достаёт из ctx идентификатор пользователя, положенный WithUserID.
// Второе значение равно false, если пользователя в контексте нет.
func UserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey{}).(int64)
	return id, ok
}
