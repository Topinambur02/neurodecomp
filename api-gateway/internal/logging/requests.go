package logging

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type requestIDKey struct{}
type upstreamKey struct{}

type upstreamObservation struct {
	service  string
	duration time.Duration
	status   int
	failed   bool
}

type upstreamState struct {
	mu          sync.Mutex
	observation *upstreamObservation
}

type RequestObserver interface {
	ObserveRequest(route string, status int, duration time.Duration)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func RecordUpstream(ctx context.Context, service string, duration time.Duration, status int, failed bool) {
	state, _ := ctx.Value(upstreamKey{}).(*upstreamState)
	if state == nil {
		return
	}
	state.mu.Lock()
	state.observation = &upstreamObservation{service: service, duration: duration, status: status, failed: failed}
	state.mu.Unlock()
}

func Requests(logger *logrus.Entry, next http.Handler, observers ...RequestObserver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		writer := &responseWriter{ResponseWriter: w}
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			logger.WithError(err).Error("failed to generate request ID")
			http.Error(writer, "internal server error", http.StatusInternalServerError)
			return
		}
		id := hex.EncodeToString(random[:])
		writer.requestID = id
		writer.Header().Set("X-Request-ID", id)
		state := &upstreamState{}
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		r = r.WithContext(context.WithValue(ctx, upstreamKey{}, state))

		defer func() {
			panicValue := recover()
			duration := time.Since(start)
			status := writer.status
			if status == 0 {
				status = http.StatusOK
				if panicValue != nil {
					status = http.StatusInternalServerError
				}
			}
			for _, observer := range observers {
				observer.ObserveRequest(routeGroup(r.URL.Path), status, duration)
			}

			fields := logrus.Fields{
				"method":         r.Method,
				"path":           r.URL.Path,
				"status":         status,
				"duration_ms":    float64(duration.Microseconds()) / 1000,
				"response_bytes": writer.bytes,
				"remote_addr":    r.RemoteAddr,
				"request_id":     id,
			}
			state.mu.Lock()
			if observation := state.observation; observation != nil {
				fields["upstream_service"] = observation.service
				fields["upstream_response_header_duration_ms"] = float64(observation.duration.Microseconds()) / 1000
				fields["upstream_failed"] = observation.failed
				if observation.status != 0 {
					fields["upstream_status"] = observation.status
				}
			}
			state.mu.Unlock()
			logger.WithFields(fields).Info("http request")
			if panicValue != nil {
				panic(panicValue)
			}
		}()

		next.ServeHTTP(writer, r)
	})
}

func routeGroup(path string) string {
	switch path {
	case "/health":
		return "health"
	case "/ready":
		return "ready"
	case "/metrics":
		return "metrics"
	default:
		if strings.HasPrefix(path, "/api/") {
			return "api"
		}
		return "other"
	}
}

type responseWriter struct {
	http.ResponseWriter
	requestID string
	status    int
	bytes     int
}

func (w *responseWriter) WriteHeader(status int) {
	if w.requestID != "" {
		w.ResponseWriter.Header().Set("X-Request-ID", w.requestID)
	}
	if (status == http.StatusSwitchingProtocols || status >= 200) && w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.requestID != "" {
		w.ResponseWriter.Header().Set("X-Request-ID", w.requestID)
	}
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
