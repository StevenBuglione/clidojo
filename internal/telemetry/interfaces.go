package telemetry

type Logger interface {
	Info(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
	Close() error
}
