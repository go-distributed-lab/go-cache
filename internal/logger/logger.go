// Package logger provides a minimal structured key=value logger that writes to
// any io.Writer. It intentionally has no external dependencies.
package logger

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Logger writes structured key=value log lines.
type Logger struct {
	w io.Writer
}

// New returns a Logger that writes to w.
func New(w io.Writer) *Logger {
	return &Logger{w: w}
}

// Default returns a Logger that writes to os.Stdout.
func Default() *Logger {
	return New(os.Stdout)
}

// Info logs a message at INFO level with optional key=value pairs.
// Pairs must be supplied as alternating key, value arguments.
func (l *Logger) Info(msg string, pairs ...any) {
	l.log("INFO", msg, pairs...)
}

// Error logs a message at ERROR level with optional key=value pairs.
func (l *Logger) Error(msg string, pairs ...any) {
	l.log("ERROR", msg, pairs...)
}

func (l *Logger) log(level, msg string, pairs ...any) {
	line := fmt.Sprintf("time=%s level=%s msg=%q", time.Now().Format(time.RFC3339), level, msg)
	for i := 0; i+1 < len(pairs); i += 2 {
		line += fmt.Sprintf(" %v=%v", pairs[i], pairs[i+1])
	}
	fmt.Fprintln(l.w, line)
}
