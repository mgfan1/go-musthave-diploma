package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gzipBody(t *testing.T, body string) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return &buf
}

func respond(contentType string, status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestGzipCompressesJSON(t *testing.T) {
	body := `{"current":500.5,"withdrawn":42}`

	req := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	Gzip(respond("application/json", http.StatusOK, body)).ServeHTTP(w, req)

	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

	zr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	got, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

func TestGzipSkips(t *testing.T) {
	cases := []struct {
		name           string
		acceptEncoding string
		contentType    string
		status         int
		body           string
	}{
		{"клиент не просил сжатия", "", "application/json", http.StatusOK, `{"current":0}`},
		{"ответ не в JSON", "gzip", "text/plain; charset=utf-8", http.StatusUnauthorized, "Unauthorized\n"},
		{"ответ без тела", "gzip", "application/json", http.StatusNoContent, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
			if c.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", c.acceptEncoding)
			}
			w := httptest.NewRecorder()

			Gzip(respond(c.contentType, c.status, c.body)).ServeHTTP(w, req)

			assert.Equal(t, c.status, w.Code)
			assert.Empty(t, w.Header().Get("Content-Encoding"))
			assert.Equal(t, c.contentType, w.Header().Get("Content-Type"))
			assert.Equal(t, c.body, w.Body.String())
		})
	}
}

func TestGzipDecompressesRequest(t *testing.T) {
	body := `{"login":"gopher","password":"secret"}`

	req := httptest.NewRequest(http.MethodPost, "/api/user/login", gzipBody(t, body))
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()

	var got []byte
	Gzip(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
	})).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, body, string(got))
}

func TestGzipBrokenRequestBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader("это не gzip"))
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()

	called := false
	Gzip(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, called, "с битым телом хендлер вызываться не должен")
}

type duplexWriter struct {
	http.ResponseWriter
	enabled bool
}

func (d *duplexWriter) EnableFullDuplex() error {
	d.enabled = true
	return nil
}

func flushViaController(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	require.NoError(t, http.NewResponseController(w).Flush())
}

func flushViaFlusher(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	f, ok := w.(http.Flusher)
	require.True(t, ok, "обёртка не реализует http.Flusher")
	f.Flush()
}

func unpack(t *testing.T, body []byte, encoding string) (string, error) {
	t.Helper()

	if encoding != "gzip" {
		return string(body), nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	require.NoError(t, err)
	got, err := io.ReadAll(zr)
	return string(got), err
}

func TestGzipFlush(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		encoding    string
		parts       []string
		flush       func(*testing.T, http.ResponseWriter)
	}{
		{"JSON по частям", "application/json", "gzip", []string{`[{"number":"1"}`, `,{"number":"2"}]`}, flushViaController},
		{"JSON по частям через http.Flusher", "application/json", "gzip", []string{`[{"number":"1"}`, `,{"number":"2"}]`}, flushViaFlusher},
		{"поток событий без сжатия", "text/event-stream", "", []string{"data: 1\n\n", "data: 2\n\n"}, flushViaController},
		{"сброс до первой записи", "application/json", "gzip", nil, flushViaController},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			rec := httptest.NewRecorder()

			Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", c.contentType)
				c.flush(t, w)
				require.True(t, rec.Flushed, "ответ не сброшен клиенту")
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.Equal(t, c.encoding, rec.Header().Get("Content-Encoding"))

				var written string
				for _, p := range c.parts {
					_, err := io.WriteString(w, p)
					require.NoError(t, err)
					written += p
					c.flush(t, w)

					got, _ := unpack(t, rec.Body.Bytes(), c.encoding)
					assert.Equal(t, written, got, "клиент не получил записанное до сброса")
				}
			})).ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, c.encoding, rec.Header().Get("Content-Encoding"))

			got, err := unpack(t, rec.Body.Bytes(), c.encoding)
			require.NoError(t, err)
			assert.Equal(t, strings.Join(c.parts, ""), got)
		})
	}
}

func TestGzipFlushBehindTimeoutHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	var err error
	http.TimeoutHandler(Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err = http.NewResponseController(w).Flush()
	})), time.Second, "").ServeHTTP(httptest.NewRecorder(), req)

	assert.ErrorIs(t, err, http.ErrNotSupported)
}

func TestGzipUnwrap(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	dw := &duplexWriter{ResponseWriter: httptest.NewRecorder()}

	Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, http.NewResponseController(w).EnableFullDuplex())
	})).ServeHTTP(dw, req)

	assert.True(t, dw.enabled, "ResponseController не добрался до исходного писателя")
}
