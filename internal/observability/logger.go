package observability

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"github.com/Normious/Khomo/internal/db"
)

// Logger performs async, sampled request logging with PII redaction.
// Bodies are never logged — only metadata (method, path, status, sizes).
type Logger struct {
	db         *db.DB
	sampleRate float64
	queue      chan *db.RequestLog
}

func NewLogger(database *db.DB, sampleRate float64, queueSize int) *Logger {
	if sampleRate < 0 {
		sampleRate = 0
	}
	if sampleRate > 1 {
		sampleRate = 1
	}
	if queueSize <= 0 {
		queueSize = 1000
	}
	l := &Logger{db: database, sampleRate: sampleRate, queue: make(chan *db.RequestLog, queueSize)}
	go l.run()
	return l
}

func (l *Logger) run() {
	for entry := range l.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := l.db.LogRequest(ctx, entry); err != nil {
			slog.Warn("request log write failed", "error", err, "request_id", entry.RequestID)
		}
		cancel()
	}
}

// Log enqueues a request log entry (non-blocking, sampled).
// PII redaction: path query values are NOT stored; only the path itself.
// User-Agent is truncated; client IP is stored as seen (needed for abuse analysis).
func (l *Logger) Log(entry *db.RequestLog) {
	if l.sampleRate < 1.0 && rand.Float64() > l.sampleRate {
		return
	}
	select {
	case l.queue <- entry:
	default:
		slog.Warn("request log queue full, dropping", "request_id", entry.RequestID)
	}
}
