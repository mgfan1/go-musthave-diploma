// Package server запускает HTTP-сервер и штатно останавливает его
// по отмене контекста.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	shutdownTimeout   = 5 * time.Second
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = time.Minute
)

// Server оборачивает http.Server и связывает его жизнь с контекстом.
type Server struct {
	http            *http.Server
	log             *zap.Logger
	shutdownTimeout time.Duration
}

// New создаёт сервер на addr для handler с таймаутами чтения, записи
// и простоя. Собственные сообщения net/http пишутся в log на уровне Error.
func New(addr string, handler http.Handler, log *zap.Logger) *Server {
	errorLog, _ := zap.NewStdLogAt(log, zap.ErrorLevel)

	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			ErrorLog:          errorLog,
		},
		log:             log,
		shutdownTimeout: shutdownTimeout,
	}
}

// Run принимает запросы, пока не отменён ctx, затем даёт начатым запросам
// до пяти секунд. Если они не успели, закрывает соединения и возвращает
// ошибку. Ошибку возвращает и тогда, когда адрес занять не удалось.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("не открыл сокет: %w", err)
	}

	stopped := make(chan error, 1)

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()

		err := s.http.Shutdown(shutdownCtx)
		if err != nil {
			_ = s.http.Close()
			err = fmt.Errorf("сервер не остановился за %s: %w", s.shutdownTimeout, err)
		}
		stopped <- err
	}()

	s.log.Info("сервер запущен", zap.Stringer("addr", ln.Addr()))

	if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("сервер упал: %w", err)
	}
	if err := <-stopped; err != nil {
		return err
	}

	s.log.Info("сервер остановлен")

	return nil
}
