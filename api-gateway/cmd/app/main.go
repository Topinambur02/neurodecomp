package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/rs/cors"
	"github.com/topinambur02/apigateway/internal/config"
	"github.com/topinambur02/apigateway/internal/handler"
	"github.com/topinambur02/apigateway/pkg/shutdown"
)

func main() {
	log.Println("config initializing")
	config := config.LoadConfig("config.json")
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})

	log.Println("create and register handlers")
	rootMux, err := handler.NewHandler(config, nil)

	if err != nil {
		log.Fatal(err)
	}

	handlerWithCors := c.Handler(rootMux)

	server := &http.Server{
		Addr:         addr,
		Handler:      handlerWithCors,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Println("Gateway launched on", config.Port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	shutdown.Graceful([]os.Signal{syscall.SIGABRT, syscall.SIGQUIT, syscall.SIGHUP, os.Interrupt, syscall.SIGTERM}, server)
	log.Println("Server exiting")
}
