package handler

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	if body == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		return
	}

	data, err := json.Marshal(body)
	if err != nil {
		h.internalError(w, r, "не сериализовал ответ", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		h.log.Warn("не отправил ответ", append(requestFields(r), zap.Error(err))...)
	}
}
