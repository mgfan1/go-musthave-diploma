package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mgfan1/go-musthave-diploma/internal/middleware"
	"github.com/mgfan1/go-musthave-diploma/internal/model"
)

type orderResponse struct {
	Number     string       `json:"number"`
	Status     string       `json:"status"`
	Accrual    *model.Money `json:"accrual,omitempty"`
	UploadedAt string       `json:"uploaded_at"`
}

func (h *Handler) uploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUser(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(r.Body)
	number := strings.TrimSpace(string(body))
	if err != nil || number == "" {
		http.Error(w, "неверный формат запроса", http.StatusBadRequest)
		return
	}

	created, err := h.orders.Upload(r.Context(), userID, number)
	switch {
	case errors.Is(err, model.ErrInvalidOrderNumber):
		http.Error(w, "неверный номер заказа", http.StatusUnprocessableEntity)
	case errors.Is(err, model.ErrOrderOwnedByOther):
		http.Error(w, "номер заказа уже загружен другим пользователем", http.StatusConflict)
	case errors.Is(err, model.ErrUserNotFound):
		middleware.Unauthorized(w)
	case err != nil:
		h.internalError(w, r, "не принял заказ", err)
	case created:
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUser(w, r)
	if !ok {
		return
	}

	orders, err := h.orders.List(r.Context(), userID)
	if err != nil {
		h.internalError(w, r, "не прочитал заказы", err)
		return
	}
	if len(orders) == 0 {
		h.writeJSON(w, r, http.StatusNoContent, nil)
		return
	}

	resp := make([]orderResponse, 0, len(orders))
	for _, o := range orders {
		resp = append(resp, orderResponse{
			Number:     o.Number,
			Status:     string(o.Status),
			Accrual:    o.Accrual,
			UploadedAt: o.UploadedAt.Format(time.RFC3339),
		})
	}

	h.writeJSON(w, r, http.StatusOK, resp)
}
