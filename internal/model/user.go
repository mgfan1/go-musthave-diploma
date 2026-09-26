package model

// User описывает зарегистрированного пользователя.
type User struct {
	// ID задаёт идентификатор пользователя в базе.
	ID int64
	// Login хранит логин с учётом регистра.
	Login string
	// PasswordHash хранит bcrypt-хеш пароля.
	PasswordHash string
}
