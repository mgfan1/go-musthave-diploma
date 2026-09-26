package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// gzipWriter сжимает тело ответа, если к моменту записи заголовков
// обработчик выставил Content-Type: application/json.
type gzipWriter struct {
	http.ResponseWriter
	zw      *gzip.Writer
	decided bool
}

// WriteHeader при первом вызове решает, сжимать ли ответ, и отправляет
// заголовки с кодом status.
func (g *gzipWriter) WriteHeader(status int) {
	if !g.decided {
		g.decided = true
		if status != http.StatusNoContent && strings.Contains(g.Header().Get("Content-Type"), "application/json") {
			g.Header().Del("Content-Length")
			g.Header().Set("Content-Encoding", "gzip")
			g.zw = gzip.NewWriter(g.ResponseWriter)
		}
	}
	g.ResponseWriter.WriteHeader(status)
}

// Write пишет b в ответ, сжимая его, если так решил WriteHeader.
func (g *gzipWriter) Write(b []byte) (int, error) {
	if !g.decided {
		g.WriteHeader(http.StatusOK)
	}
	if g.zw != nil {
		return g.zw.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// Close дописывает хвост gzip-потока. Для несжатого ответа ничего не делает.
func (g *gzipWriter) Close() error {
	if g.zw == nil {
		return nil
	}
	return g.zw.Close()
}

// Gzip распаковывает тело запроса с Content-Encoding: gzip и сжимает
// JSON-ответы для клиентов, приславших Accept-Encoding: gzip.
// Ответы без тела и не в JSON уходят как есть.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "не удалось распаковать тело запроса", http.StatusBadRequest)
				return
			}
			defer func() { _ = zr.Close() }()
			r.Body = zr
		}

		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Vary", "Accept-Encoding")

		gw := &gzipWriter{ResponseWriter: w}
		defer func() { _ = gw.Close() }()

		next.ServeHTTP(gw, r)
	})
}
