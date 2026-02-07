package telemetry

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type JSONLogger struct {
	mu sync.Mutex
	w  io.WriteCloser
}

func NewJSONLogger(path string) (*JSONLogger, error) {
	if path == "" {
		return &JSONLogger{w: nopCloser{Writer: io.Discard}}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &JSONLogger{w: f}, nil
}

func (l *JSONLogger) Info(msg string, fields map[string]any) {
	l.log("info", msg, fields)
}

func (l *JSONLogger) Error(msg string, fields map[string]any) {
	l.log("error", msg, fields)
}

func (l *JSONLogger) log(level, msg string, fields map[string]any) {
	if l == nil || l.w == nil {
		return
	}
	entry := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": level,
		"msg":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}
	b, _ := json.Marshal(entry)
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(append(b, '\n'))
}

func (l *JSONLogger) Close() error {
	if l == nil || l.w == nil {
		return nil
	}
	return l.w.Close()
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
