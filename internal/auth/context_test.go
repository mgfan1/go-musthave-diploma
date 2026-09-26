package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserIDInContext(t *testing.T) {
	_, ok := UserID(t.Context())
	assert.False(t, ok, "в пустом контексте пользователя нет")

	id, ok := UserID(WithUserID(t.Context(), 42))
	assert.True(t, ok)
	assert.Equal(t, int64(42), id)
}
