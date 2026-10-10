package handler

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/topinambur02/apigateway/internal/config"
)

func NewHandler(cfg config.Config, transport http.RoundTripper) (http.Handler, error) {
	routes := make(map[string]http.Handler, len(cfg.Services))

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
			Transport: transport,
			Rewrite: func(request *httputil.ProxyRequest) {
				request.SetURL(target)
				request.SetXForwarded()
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
