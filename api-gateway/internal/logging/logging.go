package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

func New(path string) (*logrus.Entry, *os.File, error) {
	return newWithOutput(path, os.Stdout)
}

func newWithOutput(path string, console io.Writer) (*logrus.Entry, *os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %q: %w", path, err)
	}

	logger := logrus.New()
	logger.SetOutput(console)
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		PadLevelText:    true,
		TimestampFormat: "15:04:05.000",
	})
	logger.AddHook(&jsonFileHook{
		writer: file,
		formatter: &logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "timestamp",
				logrus.FieldKeyMsg:  "message",
			},
		},
	})
	return logger.WithField("service", "api-gateway"), file, nil
}

type jsonFileHook struct {
	mu        sync.Mutex
	writer    io.Writer
	formatter logrus.Formatter
}

func (h *jsonFileHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *jsonFileHook) Fire(entry *logrus.Entry) error {
	copy := *entry
	copy.Time = entry.Time.UTC()
	line, err := h.formatter.Format(&copy)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.writer.Write(line)
	return err
}
