package logging

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

func Requests(logger *logrus.Entry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		writer := &responseWriter{ResponseWriter: w}
		defer func() {
			panicValue := recover()
			status := writer.status
			if status == 0 {
				status = http.StatusOK
				if panicValue != nil {
					status = http.StatusInternalServerError
				}
			}

			logger.WithFields(logrus.Fields{
				"method":         r.Method,
				"path":           r.URL.Path,
				"status":         status,
				"duration_ms":    float64(time.Since(start).Microseconds()) / 1000,
				"response_bytes": writer.bytes,
				"remote_addr":    r.RemoteAddr,
			}).Info("http request")
			if panicValue != nil {
				panic(panicValue)
			}
		}()

		next.ServeHTTP(writer, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *responseWriter) WriteHeader(status int) {
	if (status == http.StatusSwitchingProtocols || status >= 200) && w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += n
	return n, err
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, buffered, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil && w.status == 0 {
		w.status = http.StatusSwitchingProtocols
	}
	return conn, buffered, err
}
