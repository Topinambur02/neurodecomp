package observability

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry         *prometheus.Registry
	started          prometheus.Gauge
	httpRequests     *prometheus.CounterVec
	httpDuration     *prometheus.HistogramVec
	upstreamRequests *prometheus.CounterVec
	upstreamErrors   *prometheus.CounterVec
	upstreamDuration *prometheus.HistogramVec
}

func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		started: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_process_start_time_seconds",
			Help: "Unix timestamp when the gateway process started.",
		}),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Incoming HTTP requests by route group and response status.",
		}, []string{"route", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_http_request_duration_seconds",
			Help:    "Time spent handling incoming HTTP requests by route group.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route"}),
		upstreamRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_upstream_requests_total",
			Help: "Requests sent to upstream services, including transport failures.",
		}, []string{"service", "result"}),
		upstreamErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_upstream_errors_total",
			Help: "Upstream transport failures, timeouts, and HTTP 5xx responses.",
		}, []string{"service", "kind"}),
		upstreamDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_upstream_response_header_duration_seconds",
			Help:    "Time spent by the upstream transport until response headers or a transport error.",
			Buckets: prometheus.DefBuckets,
		}, []string{"service"}),
	}
	m.started.SetToCurrentTime()
	m.registry.MustRegister(m.started, m.httpRequests, m.httpDuration, m.upstreamRequests, m.upstreamErrors, m.upstreamDuration)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) InitService(service string) {
	m.upstreamDuration.WithLabelValues(service)
	m.upstreamRequests.WithLabelValues(service, "transport_error").Add(0)
	for _, kind := range []string{"transport", "timeout", "http_5xx"} {
		m.upstreamErrors.WithLabelValues(service, kind).Add(0)
	}
}

func (m *Metrics) ObserveRequest(route string, status int, duration time.Duration) {
	m.httpRequests.WithLabelValues(route, strconv.Itoa(status)).Inc()
	m.httpDuration.WithLabelValues(route).Observe(duration.Seconds())
}

func (m *Metrics) Observe(service string, duration time.Duration, status int, err error) {
	m.upstreamDuration.WithLabelValues(service).Observe(duration.Seconds())
	if err != nil {
		m.upstreamRequests.WithLabelValues(service, "transport_error").Inc()
		kind := "transport"
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			kind = "timeout"
		}
		m.upstreamErrors.WithLabelValues(service, kind).Inc()
		return
	}

	m.upstreamRequests.WithLabelValues(service, strconv.Itoa(status)).Inc()
	if status >= http.StatusInternalServerError {
		m.upstreamErrors.WithLabelValues(service, "http_5xx").Inc()
	}
}
