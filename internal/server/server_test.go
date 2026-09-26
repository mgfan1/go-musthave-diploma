package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type response struct {
	body string
	err  error
}

func observed(handler http.Handler) (*Server, *observer.ObservedLogs) {
	core, logs := observer.New(zap.InfoLevel)
	return New("127.0.0.1:0", handler, zap.New(core)), logs
}

func start(t *testing.T, srv *Server, logs *observer.ObservedLogs) (string, context.CancelFunc, <-chan error) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	started := func() bool { return logs.FilterMessage("сервер запущен").Len() == 1 }
	require.Eventually(t, started, time.Second, 10*time.Millisecond, "сервер не запустился")

	addr, ok := logs.FilterMessage("сервер запущен").All()[0].ContextMap()["addr"].(string)
	require.True(t, ok)

	return addr, cancel, done
}

func get(t *testing.T, addr string) <-chan response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr, nil)
	require.NoError(t, err)

	got := make(chan response, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			got <- response{err: err}
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		got <- response{body: string(body), err: err}
	}()

	return got
}

func waitEntered(t *testing.T, entered <-chan struct{}, got <-chan response) {
	t.Helper()

	select {
	case <-entered:
	case res := <-got:
		t.Fatalf("запрос завершился, не дойдя до обработчика: %v", res.err)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	srv := New("127.0.0.1:0", http.NotFoundHandler(), zap.NewNop())

	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("сервер не остановился после отмены контекста")
	}
}

func TestRunLogsActualAddr(t *testing.T) {
	srv, logs := observed(http.NotFoundHandler())

	addr, cancel, done := start(t, srv, logs)

	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", host)
	assert.NotEqual(t, "0", port)

	cancel()
	require.NoError(t, <-done)
	assert.Equal(t, 1, logs.FilterMessage("сервер остановлен").Len())
}

func TestRunFinishesStartedRequest(t *testing.T) {
	entered := make(chan struct{})
	srv, logs := observed(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "done")
	}))

	addr, cancel, done := start(t, srv, logs)
	got := get(t, addr)

	waitEntered(t, entered, got)
	cancel()

	res := <-got
	require.NoError(t, res.err)
	assert.Equal(t, "done", res.body)
	assert.NoError(t, <-done)
}

func TestRunClosesHangingConnections(t *testing.T) {
	entered := make(chan struct{})
	srv, logs := observed(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	srv.shutdownTimeout = 50 * time.Millisecond

	addr, cancel, done := start(t, srv, logs)
	got := get(t, addr)

	waitEntered(t, entered, got)
	cancel()

	require.ErrorIs(t, <-done, context.DeadlineExceeded)
	assert.Error(t, (<-got).err)
	assert.Zero(t, logs.FilterMessage("сервер остановлен").Len())
}

func TestRunFailsOnBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	core, logs := observer.New(zap.InfoLevel)

	err = New(ln.Addr().String(), http.NotFoundHandler(), zap.New(core)).Run(t.Context())
	require.Error(t, err)
	assert.Zero(t, logs.FilterMessage("сервер запущен").Len())
}

func TestNewSetsTimeouts(t *testing.T) {
	srv := New("127.0.0.1:0", http.NotFoundHandler(), zap.NewNop())

	assert.Equal(t, readHeaderTimeout, srv.http.ReadHeaderTimeout)
	assert.Equal(t, readTimeout, srv.http.ReadTimeout)
	assert.Equal(t, writeTimeout, srv.http.WriteTimeout)
	assert.Equal(t, idleTimeout, srv.http.IdleTimeout)
	assert.Equal(t, shutdownTimeout, srv.shutdownTimeout)
	assert.Less(t, handlerTimeout, writeTimeout)
	assert.NotNil(t, srv.http.ErrorLog)
}

func TestNewLimitsHandlerTime(t *testing.T) {
	var (
		deadline time.Time
		ok       bool
	)
	srv := New("127.0.0.1:0", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	}), zap.NewNop())

	srv.http.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	require.True(t, ok, "у контекста обработчика нет дедлайна")
	assert.WithinDuration(t, time.Now().Add(handlerTimeout), deadline, time.Second)
}

func TestNewKeepsHandlerResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header string
		value  string
		body   string
	}{
		{"токен после входа", http.StatusOK, "Authorization", "Bearer token", ""},
		{"пустой список", http.StatusNoContent, "Content-Type", "application/json", ""},
		{"тело ответа", http.StatusOK, "Content-Type", "application/json", `{"current":500.5,"withdrawn":42}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set(tt.header, tt.value)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}), zap.NewNop())

			w := httptest.NewRecorder()
			srv.http.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

			assert.Equal(t, tt.status, w.Code)
			assert.Equal(t, tt.value, w.Header().Get(tt.header))
			assert.Equal(t, tt.body, w.Body.String())
		})
	}
}
