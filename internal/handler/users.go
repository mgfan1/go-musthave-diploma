package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mgfan1/go-musthave-diploma/internal/auth"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

type credentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func readCredentials(r *http.Request) (credentials, bool) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		return c, false
	}
	return c, c.Login != "" && c.Password != "" && len(c.Password) <= auth.MaxPasswordLen
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	c, ok := readCredentials(r)
	if !ok {
		http.Error(w, "неверный формат запроса", http.StatusBadRequest)
		return
	}

	token, err := h.users.Register(r.Context(), c.Login, c.Password)
	if errors.Is(err, model.ErrLoginTaken) {
		http.Error(w, "логин уже занят", http.StatusConflict)
		return
	}
	if err != nil {
		h.internalError(w, "не зарегистрировал пользователя", err)
		return
	}

	authorize(w, token)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	c, ok := readCredentials(r)
	if !ok {
		http.Error(w, "неверный формат запроса", http.StatusBadRequest)
		return
	}

	token, err := h.users.Login(r.Context(), c.Login, c.Password)
	if errors.Is(err, model.ErrInvalidCredentials) {
		http.Error(w, "неверная пара логин и пароль", http.StatusUnauthorized)
		return
	}
	if err != nil {
		h.internalError(w, "не выполнил вход", err)
		return
	}

	authorize(w, token)
}

func authorize(w http.ResponseWriter, token string) {
	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}
