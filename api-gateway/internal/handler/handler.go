package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/topinambur02/apigateway/internal/config"
	"github.com/topinambur02/apigateway/internal/logging"
	"github.com/topinambur02/apigateway/internal/observability"
)

func NewHandler(cfg config.Config, transport http.RoundTripper) (http.Handler, error) {
	return newHandler(cfg, transport, nil)
}

func NewHandlerWithMetrics(cfg config.Config, transport http.RoundTripper, metrics *observability.Metrics) (http.Handler, error) {
	return newHandler(cfg, transport, metrics)
}

func newHandler(cfg config.Config, transport http.RoundTripper, metrics *observability.Metrics) (http.Handler, error) {
	routes := make(map[string]http.Handler, len(cfg.Services))
	if transport == nil {
		transport = http.DefaultTransport
	}

	for name, service := range cfg.Services {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/%") {
			return nil, fmt.Errorf("invalid service name %q", name)
		}

		if service.Host == "" || service.Port < 1 || service.Port > 65535 {
			return nil, fmt.Errorf("invalid address for service %q", name)
		}

		target := &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(service.Host, strconv.Itoa(service.Port)),
		}

		proxy := &httputil.ReverseProxy{
			Transport: &observedTransport{service: name, next: transport, metrics: metrics},
			Rewrite: func(request *httputil.ProxyRequest) {
				request.SetURL(target)
				request.SetXForwarded()
				request.Out.Header.Del("X-Request-ID")
				if id := logging.RequestID(request.In.Context()); id != "" {
					request.Out.Header.Set("X-Request-ID", id)
				}
			},
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
				status := http.StatusBadGateway
				var netErr net.Error
				if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
					status = http.StatusGatewayTimeout
				}
				log.Printf("upstream %q request failed: %v", name, err)
				http.Error(w, http.StatusText(status), status)
			},
		}
		routes[name] = http.StripPrefix("/api/"+name, proxy)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		name, _, _ := strings.Cut(path, "/")
		route, ok := routes[name]

		if !ok {
			http.NotFound(w, r)
			return
		}

		route.ServeHTTP(w, r)
	})

	return mux, nil
}

type observedTransport struct {
	service string
	next    http.RoundTripper
	metrics *observability.Metrics
}

func (t *observedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	start := time.Now()
	response, err := t.next.RoundTrip(request)
	duration := time.Since(start)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	logging.RecordUpstream(request.Context(), t.service, duration, status, err != nil)
	if t.metrics != nil {
		t.metrics.Observe(t.service, duration, status, err)
	}
	return response, err
}
