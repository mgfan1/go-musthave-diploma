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

func logged(t *testing.T, h http.HandlerFunc) map[string]any {
	t.Helper()

	core, logs := observer.New(zap.InfoLevel)
	Logging(zap.New(core))(h).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/user/orders", nil))

	entries := logs.All()
	require.Len(t, entries, 1)
	return entries[0].ContextMap()
}

func TestLoggingStatusWithoutWriteHeader(t *testing.T) {
	body := `[{"number":"12345678903"}]`

	fields := logged(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})

	assert.Equal(t, int64(http.StatusOK), fields["status"])
	assert.Equal(t, int64(len(body)), fields["size"])
}

func TestLoggingStatusForEmptyResponse(t *testing.T) {
	fields := logged(t, func(http.ResponseWriter, *http.Request) {})

	assert.Equal(t, int64(http.StatusOK), fields["status"])
	assert.Equal(t, int64(0), fields["size"])
}

func TestLoggingExplicitStatus(t *testing.T) {
	fields := logged(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	assert.Equal(t, int64(http.StatusNoContent), fields["status"])
	assert.Equal(t, "/api/user/orders", fields["uri"])
	assert.Equal(t, http.MethodGet, fields["method"])
}
