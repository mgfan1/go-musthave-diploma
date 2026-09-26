package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestRouterStubs(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/user/register"},
		{http.MethodPost, "/api/user/login"},
		{http.MethodPost, "/api/user/orders"},
		{http.MethodGet, "/api/user/orders"},
		{http.MethodGet, "/api/user/balance"},
		{http.MethodPost, "/api/user/balance/withdraw"},
		{http.MethodGet, "/api/user/withdrawals"},
	}

	router := Router(zap.NewNop())

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(rt.method, rt.path, nil))

			assert.Equal(t, http.StatusNotImplemented, w.Code)
		})
	}
}

func TestRouterUnknownPath(t *testing.T) {
	w := httptest.NewRecorder()
	Router(zap.NewNop()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/user/unknown", nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
}
