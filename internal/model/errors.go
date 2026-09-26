package model

import "errors"

var (
	// ErrLoginTaken возвращается при регистрации, если логин уже занят.
	ErrLoginTaken = errors.New("логин уже занят")
	// ErrUserNotFound возвращается хранилищем, если пользователя с таким логином нет.
	ErrUserNotFound = errors.New("пользователь не найден")
	// ErrInvalidCredentials возвращается при входе и для неизвестного логина,
	// и для неверного пароля, чтобы по ответу нельзя было узнать, какие логины есть.
	ErrInvalidCredentials = errors.New("неверная пара логин и пароль")
)
