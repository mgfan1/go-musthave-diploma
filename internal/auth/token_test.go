package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "секрет для тестов"

func sign(t *testing.T, method jwt.SigningMethod, key any, c jwt.Claims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(method, c).SignedString(key)
	require.NoError(t, err)
	return token
}

func TestTokensRoundTrip(t *testing.T) {
	tokens := NewTokens(testSecret, time.Hour)

	token, err := tokens.Issue(42)
	require.NoError(t, err)

	id, err := tokens.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), id)
}

func TestTokensExpired(t *testing.T) {
	token, err := NewTokens(testSecret, -time.Minute).Issue(42)
	require.NoError(t, err)

	_, err = NewTokens(testSecret, time.Hour).Parse(token)
	assert.ErrorIs(t, err, jwt.ErrTokenExpired)
}

func TestTokensParseRejects(t *testing.T) {
	alive := jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	valid := claims{RegisteredClaims: alive, UserID: 42}

	cases := []struct {
		name  string
		token string
	}{
		{"пустая строка", ""},
		{"мусор вместо токена", "не токен"},
		{"чужая подпись", sign(t, jwt.SigningMethodHS256, []byte("чужой секрет"), valid)},
		{"алгоритм none", sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid)},
		{"другой алгоритм HMAC", sign(t, jwt.SigningMethodHS512, []byte(testSecret), valid)},
		{"без срока жизни", sign(t, jwt.SigningMethodHS256, []byte(testSecret), claims{UserID: 42})},
		{"без пользователя", sign(t, jwt.SigningMethodHS256, []byte(testSecret), claims{RegisteredClaims: alive})},
	}

	tokens := NewTokens(testSecret, time.Hour)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, err := tokens.Parse(c.token)
			assert.Error(t, err)
			assert.Zero(t, id)
		})
	}
}

func BenchmarkTokensParse(b *testing.B) {
	tokens := NewTokens(testSecret, time.Hour)
	token, err := tokens.Issue(42)
	require.NoError(b, err)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := tokens.Parse(token); err != nil {
			b.Fatal(err)
		}
	}
}
