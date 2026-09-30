package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MaxPasswordLen задаёт предельную длину пароля в байтах: более длинные
// пароли bcrypt не принимает.
const MaxPasswordLen = 72

// HashPassword возвращает bcrypt-хеш пароля. Соль хранится внутри хеша.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("не захешировал пароль: %w", err)
	}
	return string(hash), nil
}

// CheckPassword сообщает, подходит ли пароль к bcrypt-хешу. Пароль длиннее
// MaxPasswordLen не подходит ни к какому хешу: при сравнении bcrypt молча
// обрезал бы его до MaxPasswordLen байт.
func CheckPassword(hash, password string) bool {
	if len(password) > MaxPasswordLen {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
