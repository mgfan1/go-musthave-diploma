package server

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

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

func TestRunFailsOnBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	err = New(ln.Addr().String(), http.NotFoundHandler(), zap.NewNop()).Run(t.Context())
	assert.Error(t, err)
}
