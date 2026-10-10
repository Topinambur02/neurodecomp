package handler

import (
	"net"
	"net/http"
	"time"

	"github.com/topinambur02/apigateway/internal/config"
)

func NewTransport(cfg config.Upstream) (*http.Transport, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   time.Duration(cfg.DialTimeoutSeconds) * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.ResponseHeaderTimeout = time.Duration(cfg.ResponseHeaderTimeoutSeconds) * time.Second
	transport.MaxConnsPerHost = cfg.MaxConnectionsPerHost
	transport.MaxIdleConnsPerHost = cfg.MaxIdleConnectionsPerHost
	return transport, nil
}
