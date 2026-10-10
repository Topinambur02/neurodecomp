package shutdown

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

func Graceful(ctx context.Context, logger *logrus.Entry, server *http.Server, serveErr <-chan error, timeout time.Duration) error {
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			_ = server.Close()
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}

	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP after shutdown: %w", err)
	}
	return nil
}
