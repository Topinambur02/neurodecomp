package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/cors"
	"github.com/sirupsen/logrus"
	"github.com/topinambur02/apigateway/internal/config"
	"github.com/topinambur02/apigateway/internal/handler"
	"github.com/topinambur02/apigateway/internal/logging"
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
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		logger.WithError(err).Error("failed to load config")
		return err
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})

	logger.Info("create and register handlers")
	transport, err := handler.NewTransport(cfg.Upstream)
	if err != nil {
		logger.WithError(err).Error("invalid upstream configuration")
		return err
	}
	defer transport.CloseIdleConnections()

	rootMux, err := handler.NewHandler(cfg, transport)

	if err != nil {
		logger.WithError(err).Error("failed to register handlers")
		return err
	}

	limitedHandler, err := ratelimit.New(cfg.RateLimit, rootMux)
	if err != nil {
		logger.WithError(err).Error("invalid rate limit configuration")
		return err
	}

	handlerWithCors := logging.Requests(logger, c.Handler(limitedHandler))

	server := &http.Server{
		Addr:         addr,
		Handler:      handlerWithCors,
		ErrorLog:     log.New(standardLogWriter, "", 0),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGABRT, syscall.SIGQUIT, syscall.SIGHUP, os.Interrupt, syscall.SIGTERM)
	defer stop()

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
