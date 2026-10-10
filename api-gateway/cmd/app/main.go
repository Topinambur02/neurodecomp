package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/rs/cors"
	"github.com/sirupsen/logrus"
	"github.com/topinambur02/apigateway/internal/config"
	"github.com/topinambur02/apigateway/internal/handler"
	"github.com/topinambur02/apigateway/internal/logging"
	"github.com/topinambur02/apigateway/pkg/shutdown"
)

func main() {
	logger, logFile, err := logging.New("logs/all.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logging: %v\n", err)
		os.Exit(1)
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
		os.Exit(1)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})

	logger.Info("create and register handlers")
	rootMux, err := handler.NewHandler(cfg, nil)

	if err != nil {
		logger.WithError(err).Error("failed to register handlers")
		os.Exit(1)
	}

	handlerWithCors := logging.Requests(logger, c.Handler(rootMux))

	server := &http.Server{
		Addr:         addr,
		Handler:      handlerWithCors,
		ErrorLog:     log.New(standardLogWriter, "", 0),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.WithField("address", addr).Info("gateway starting")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Error("gateway server failed")
			os.Exit(1)
		}
	}()

	if err := shutdown.Graceful(logger, []os.Signal{syscall.SIGABRT, syscall.SIGQUIT, syscall.SIGHUP, os.Interrupt, syscall.SIGTERM}, server); err != nil {
		logger.WithError(err).Error("shutdown failed")
		os.Exit(1)
	}
	logger.Info("server exiting")
}
