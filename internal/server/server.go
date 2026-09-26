// Package server запускает HTTP-сервер и штатно останавливает его
// по отмене контекста.
package server

import (
	"context"
	"errors"
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
	http *http.Server
	log  *zap.Logger
}

// New создаёт сервер, который будет слушать addr и передавать запросы handler.
// Чтение запроса, запись ответа и простой соединения ограничены таймаутами,
// чтобы медленный клиент не держал соединение бесконечно.
func New(addr string, handler http.Handler, log *zap.Logger) *Server {
	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
		log: log,
	}
}

// Run принимает запросы, пока не отменён ctx, а затем даёт начатым запросам
// до пяти секунд на завершение. Возвращает ошибку, только если сервер
// не смог запуститься или упал сам.
func (s *Server) Run(ctx context.Context) error {
	stopped := make(chan struct{})

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.http.Shutdown(shutdownCtx); err != nil {
			s.log.Warn("сервер не остановился штатно", zap.Error(err))
		}
		close(stopped)
	}()

	s.log.Info("сервер запущен", zap.String("addr", s.http.Addr))

	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-stopped

	s.log.Info("сервер остановлен")

	return nil
}
