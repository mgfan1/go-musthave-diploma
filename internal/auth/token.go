package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Tokens выпускает и проверяет токены доступа: JWT с подписью HS256,
// идентификатором пользователя и сроком жизни.
type Tokens struct {
	secret []byte
	ttl    time.Duration
}

type claims struct {
	jwt.RegisteredClaims
	UserID int64 `json:"user_id"`
}

// NewTokens создаёт Tokens, подписывающий токены секретом secret.
// Выпущенные токены действуют в течение ttl.
func NewTokens(secret string, ttl time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), ttl: ttl}
}

// Issue выпускает токен доступа для пользователя userID.
func (t *Tokens) Issue(userID int64) (string, error) {
	now := time.Now()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
		UserID: userID,
	})

	signed, err := token.SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("не подписал токен: %w", err)
	}
	return signed, nil
}

// Parse проверяет подпись и срок жизни токена и возвращает идентификатор
// пользователя. Принимается только HS256: токены с любым другим алгоритмом,
// в том числе none, и токены без срока жизни отклоняются.
func (t *Tokens) Parse(token string) (int64, error) {
	var c claims

	_, err := jwt.ParseWithClaims(token, &c,
		func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return 0, fmt.Errorf("недействительный токен: %w", err)
	}
	if c.UserID <= 0 {
		return 0, errors.New("в токене нет пользователя")
	}

	return c.UserID, nil
}
