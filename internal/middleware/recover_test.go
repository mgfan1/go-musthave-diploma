package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func panicking(v any) http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {
		panic(v)
	}
}

func TestRecoverRespondsWithInternalError(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	w := httptest.NewRecorder()

	Recover(zap.New(core))(panicking("boom")).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/user/orders", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", w.Body.String())

	entries := logs.All()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	assert.Equal(t, "boom", fields["panic"])
	assert.Equal(t, http.MethodGet, fields["method"])
	assert.Equal(t, "/api/user/orders", fields["path"])
	assert.NotEmpty(t, fields["stack"])
}

func TestRecoverPassesAbortHandler(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	h := Recover(zap.New(core))(panicking(http.ErrAbortHandler))

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/user/orders", nil))
	})
	assert.Zero(t, logs.Len())
}

func TestRecoverInsideLogging(t *testing.T) {
	fields := logged(t, Recover(zap.NewNop())(panicking("boom")).ServeHTTP)

	assert.Equal(t, int64(http.StatusInternalServerError), fields["status"])
}

func TestRecoverWithoutPanic(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	w := httptest.NewRecorder()

	Recover(zap.New(core))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/user/orders", nil))

	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Zero(t, logs.Len())
}
