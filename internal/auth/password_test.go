package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("пароль гофера")
	require.NoError(t, err)

	assert.NotContains(t, hash, "пароль гофера")
	assert.True(t, CheckPassword(hash, "пароль гофера"))
	assert.False(t, CheckPassword(hash, "чужой пароль"))
	assert.False(t, CheckPassword("не хеш", "пароль гофера"))
}

func TestHashPasswordIsSalted(t *testing.T) {
	first, err := HashPassword("secret")
	require.NoError(t, err)
	second, err := HashPassword("secret")
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "одинаковые пароли должны давать разные хеши")
}

func TestHashPasswordLength(t *testing.T) {
	_, err := HashPassword(strings.Repeat("a", MaxPasswordLen))
	require.NoError(t, err)

	_, err = HashPassword(strings.Repeat("a", MaxPasswordLen+1))
	assert.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
}

func TestCheckPasswordTooLong(t *testing.T) {
	password := strings.Repeat("a", MaxPasswordLen)
	hash, err := HashPassword(password)
	require.NoError(t, err)

	assert.True(t, CheckPassword(hash, password))
	assert.False(t, CheckPassword(hash, password+"b"), "пароль не должен совпадать по первым MaxPasswordLen байтам")
}
