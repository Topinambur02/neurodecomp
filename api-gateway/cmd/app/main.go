package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/topinambur02/apigateway/internal/auth"
	"github.com/topinambur02/apigateway/internal/config"
	"github.com/topinambur02/apigateway/internal/handler"
	"github.com/topinambur02/apigateway/internal/logging"
	"github.com/topinambur02/apigateway/internal/observability"
	"github.com/topinambur02/apigateway/internal/ratelimit"
	"github.com/topinambur02/apigateway/pkg/shutdown"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	logger, logFile, err := logging.New("logs/all.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logging: %v\n", err)
		return err
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close log file: %v\n", err)
		}
	}()
	standardLogWriter := logger.WriterLevel(logrus.ErrorLevel)
	defer func() {
		if err := standardLogWriter.Close(); err != nil {
			logger.WithError(err).Warn("failed to close standard log writer")
		}
	}()
	log.SetOutput(standardLogWriter)
	log.SetFlags(0)

	logger.Info("config initializing")
	configPath := os.Getenv("GATEWAY_CONFIG")
	if configPath == "" {
		configPath = "config.json"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.WithError(err).Error("failed to load config")
		return err
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGABRT, syscall.SIGQUIT, syscall.SIGHUP, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("create and register handlers")
	metrics := observability.NewMetrics()
	transport, err := handler.NewTransport(cfg.Upstream)
	if err != nil {
		logger.WithError(err).Error("invalid upstream configuration")
		return err
	}
	defer transport.CloseIdleConnections()

	rootMux, err := handler.NewHandlerWithMetrics(cfg, transport, metrics)

	if err != nil {
		logger.WithError(err).Error("failed to register handlers")
		return err
	}
	for service := range cfg.Services {
		metrics.InitService(service)
	}

	authenticator, err := auth.New(ctx, cfg.Auth)
	if err != nil {
		logger.WithError(err).Error("failed to initialize authentication")
		return err
	}
	applicationMux := http.NewServeMux()
	applicationMux.Handle("/api/", authenticator.Require(rootMux))
	applicationMux.Handle("/", rootMux)
	limitedHandler, err := ratelimit.New(cfg.RateLimit, applicationMux)
	if err != nil {
		logger.WithError(err).Error("invalid rate limit configuration")
		return err
	}

	operationalMux := http.NewServeMux()
	operationalMux.Handle("/ready", readinessHandler(ctx))
	operationalMux.Handle("/metrics", authenticator.Require(metrics.Handler(), "admin"))
	operationalMux.Handle("/", limitedHandler)
	handlerWithCors := logging.Requests(logger, cfg.CORS.Handler(operationalMux), metrics)

	server := &http.Server{
		Addr:         addr,
		Handler:      handlerWithCors,
		ErrorLog:     log.New(standardLogWriter, "", 0),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.WithError(err).Error("failed to start gateway server")
		return err
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	logger.WithField("address", listener.Addr().String()).Info("gateway starting")

	if err := shutdown.Graceful(ctx, logger, server, serveErr, 15*time.Second); err != nil {
		logger.WithError(err).Error("gateway server stopped")
		return err
	}
	logger.Info("server exiting")
	return nil
}

func readinessHandler(ctx context.Context) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if ctx.Err() != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ready\n"))
	})
}
