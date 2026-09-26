package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

type balanceResponse struct {
	Current   model.Money `json:"current"`
	Withdrawn model.Money `json:"withdrawn"`
}

type withdrawRequest struct {
	Order string      `json:"order"`
	Sum   model.Money `json:"sum"`
}

type withdrawalResponse struct {
	Order       string      `json:"order"`
	Sum         model.Money `json:"sum"`
	ProcessedAt string      `json:"processed_at"`
}

func (h *Handler) getBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUser(w, r)
	if !ok {
		return
	}

	b, err := h.balance.Get(r.Context(), userID)
	if err != nil {
		h.internalError(w, r, "не прочитал баланс", err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, balanceResponse{Current: b.Current, Withdrawn: b.Withdrawn})
}

func (h *Handler) withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUser(w, r)
	if !ok {
		return
	}

	var req withdrawRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.Order == "" {
		http.Error(w, "неверный формат запроса", http.StatusBadRequest)
		return
	}

	err = h.balance.Withdraw(r.Context(), userID, req.Order, req.Sum)
	switch {
	case errors.Is(err, model.ErrInvalidWithdrawSum):
		http.Error(w, "неверная сумма списания", http.StatusBadRequest)
	case errors.Is(err, model.ErrInvalidOrderNumber):
		http.Error(w, "неверный номер заказа", http.StatusUnprocessableEntity)
	case errors.Is(err, model.ErrInsufficientFunds):
		http.Error(w, "на счету недостаточно средств", http.StatusPaymentRequired)
	case errors.Is(err, model.ErrUserNotFound):
		unauthorized(w)
	case err != nil:
		h.internalError(w, r, "не списал баллы", err)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) listWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUser(w, r)
	if !ok {
		return
	}

	withdrawals, err := h.balance.Withdrawals(r.Context(), userID)
	if err != nil {
		h.internalError(w, r, "не прочитал списания", err)
		return
	}
	if len(withdrawals) == 0 {
		h.writeJSON(w, r, http.StatusNoContent, nil)
		return
	}

	resp := make([]withdrawalResponse, 0, len(withdrawals))
	for _, wd := range withdrawals {
		resp = append(resp, withdrawalResponse{
			Order:       wd.Order,
			Sum:         wd.Sum,
			ProcessedAt: wd.ProcessedAt.Format(time.RFC3339),
		})
	}

	h.writeJSON(w, r, http.StatusOK, resp)
}
