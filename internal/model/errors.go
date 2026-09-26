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
	// ErrInvalidOrderNumber возвращается, если номер заказа не состоит из цифр
	// или не проходит проверку по алгоритму Луна.
	ErrInvalidOrderNumber = errors.New("неверный номер заказа")
	// ErrOrderOwnedByOther возвращается, если номер заказа уже загрузил
	// другой пользователь.
	ErrOrderOwnedByOther = errors.New("заказ загружен другим пользователем")
	// ErrInsufficientFunds возвращается, если на счету меньше баллов,
	// чем пользователь хочет списать.
	ErrInsufficientFunds = errors.New("на счету недостаточно баллов")
	// ErrInvalidWithdrawSum возвращается, если сумма списания не положительная
	// или после округления до копеек становится нулевой.
	ErrInvalidWithdrawSum = errors.New("неверная сумма списания")
)
