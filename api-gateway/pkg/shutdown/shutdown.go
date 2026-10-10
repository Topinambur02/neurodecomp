package shutdown

import (
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/sirupsen/logrus"
)

func Graceful(logger *logrus.Entry, signals []os.Signal, closeItems ...io.Closer) error {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, signals...)
	defer signal.Stop(sigc)
	sig := <-sigc
	logger.WithField("signal", sig.String()).Info("shutdown signal received")

	for _, closer := range closeItems {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close %T: %w", closer, err)
		}
	}
	return nil
}
